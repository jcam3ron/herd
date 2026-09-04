// Package niri implements backend.Backend for niri, driving it entirely
// through `niri msg`.
package niri

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/jcam3ron/herd/internal/backend"
	"github.com/jcam3ron/herd/internal/ghostty"
	"github.com/jcam3ron/herd/internal/run"
)

const pollInterval = 100 * time.Millisecond
const pollTimeout = 5 * time.Second

var errPollTimeout = errors.New("poll timeout")

// pollUntil calls check every pollInterval until it reports done, returns
// an error, or pollTimeout elapses (in which case it returns
// errPollTimeout, for the caller to turn into a descriptive error).
func pollUntil[T any](check func() (result T, done bool, err error)) (T, error) {
	deadline := time.Now().Add(pollTimeout)
	for time.Now().Before(deadline) {
		result, done, err := check()
		if err != nil || done {
			return result, err
		}
		time.Sleep(pollInterval)
	}
	var zero T
	return zero, errPollTimeout
}

// window mirrors the fields herd needs from `niri msg -j windows`.
type window struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	AppID       string `json:"app_id"`
	WorkspaceID int    `json:"workspace_id"`
	IsFocused   bool   `json:"is_focused"`
	Layout      struct {
		PosInScrollingLayout [2]int `json:"pos_in_scrolling_layout"`
	} `json:"layout"`
}

type workspace struct {
	ID        int  `json:"id"`
	IsFocused bool `json:"is_focused"`
}

// layout is niri's Layout payload: a scrolling-layout column/row pair.
type layout struct {
	Col int `json:"col"`
	Row int `json:"row"`
}

type Backend struct {
	Run run.Runner
	// WorkDir is the working directory for spawned ghostty windows that
	// have no zmx session to attach to. "" uses ghostty's default.
	WorkDir string
	// Out receives niri-specific warnings (e.g. a column that could not
	// be stacked as saved). Defaults to os.Stdout.
	Out io.Writer
}

func New() *Backend {
	return &Backend{Run: run.Exec}
}

func (b *Backend) Name() string { return "niri" }

func (b *Backend) out() io.Writer {
	if b.Out != nil {
		return b.Out
	}
	return os.Stdout
}

func (b *Backend) focusedWorkspaceID(ctx context.Context) (int, error) {
	out, err := b.Run(ctx, "niri", "msg", "-j", "workspaces")
	if err != nil {
		return 0, err
	}
	var wss []workspace
	if err := json.Unmarshal(out, &wss); err != nil {
		return 0, fmt.Errorf("decoding niri workspaces: %w", err)
	}
	for _, ws := range wss {
		if ws.IsFocused {
			return ws.ID, nil
		}
	}
	return 0, fmt.Errorf("no focused niri workspace")
}

// allWindows returns every window niri knows about, regardless of
// workspace or app.
func (b *Backend) allWindows(ctx context.Context) ([]window, error) {
	out, err := b.Run(ctx, "niri", "msg", "-j", "windows")
	if err != nil {
		return nil, err
	}
	var all []window
	if err := json.Unmarshal(out, &all); err != nil {
		return nil, fmt.Errorf("decoding niri windows: %w", err)
	}
	return all, nil
}

// windowsInWorkspace returns ghostty windows in ws, sorted by column then
// row (matching the scrolling layout's visual order).
func (b *Backend) windowsInWorkspace(ctx context.Context, ws int) ([]window, error) {
	all, err := b.allWindows(ctx)
	if err != nil {
		return nil, err
	}

	var in []window
	for _, w := range all {
		if w.WorkspaceID == ws && w.AppID == ghostty.AppID {
			in = append(in, w)
		}
	}
	sort.Slice(in, func(i, j int) bool {
		pi, pj := in[i].Layout.PosInScrollingLayout, in[j].Layout.PosInScrollingLayout
		if pi[0] != pj[0] {
			return pi[0] < pj[0]
		}
		return pi[1] < pj[1]
	})
	return in, nil
}

func (b *Backend) ListWindows(ctx context.Context) ([]backend.RawWindow, error) {
	ws, err := b.focusedWorkspaceID(ctx)
	if err != nil {
		return nil, err
	}
	wins, err := b.windowsInWorkspace(ctx, ws)
	if err != nil {
		return nil, err
	}

	raws := make([]backend.RawWindow, len(wins))
	for i, w := range wins {
		l, err := json.Marshal(layout{Col: w.Layout.PosInScrollingLayout[0], Row: w.Layout.PosInScrollingLayout[1]})
		if err != nil {
			return nil, err
		}
		raws[i] = backend.RawWindow{ID: strconv.Itoa(w.ID), Title: w.Title, Layout: l, Focused: w.IsFocused}
	}
	return raws, nil
}

func (b *Backend) Close(ctx context.Context, id string) error {
	_, err := b.Run(ctx, "niri", "msg", "action", "close-window", "--id", id)
	return err
}

func (b *Backend) Wait(ctx context.Context, id string) error {
	_, err := pollUntil(func() (struct{}, bool, error) {
		all, err := b.allWindows(ctx)
		if err != nil {
			return struct{}{}, false, err
		}
		for _, w := range all {
			if strconv.Itoa(w.ID) == id {
				return struct{}{}, false, nil
			}
		}
		return struct{}{}, true, nil
	})
	if errors.Is(err, errPollTimeout) {
		return fmt.Errorf("window %s did not close (is confirm-close-surface disabled in ghostty config?)", id)
	}
	return err
}

// waitForNewWindow polls until a window not in known appears, and returns
// its id along with the full window list from that successful poll --
// Apply needs that list right after to look up columns, and reusing it
// here avoids fetching it all over again.
func (b *Backend) waitForNewWindow(ctx context.Context, ws int, known map[int]bool) (int, []window, error) {
	type found struct {
		id   int
		wins []window
	}
	f, err := pollUntil(func() (found, bool, error) {
		wins, err := b.windowsInWorkspace(ctx, ws)
		if err != nil {
			return found{}, false, err
		}
		for _, w := range wins {
			if !known[w.ID] {
				return found{id: w.ID, wins: wins}, true, nil
			}
		}
		return found{}, false, nil
	})
	if errors.Is(err, errPollTimeout) {
		return 0, nil, fmt.Errorf("new ghostty window did not appear within %s", pollTimeout)
	}
	if err != nil {
		return 0, nil, err
	}
	return f.id, f.wins, nil
}

func columnOf(wins []window, id int) (int, bool) {
	for _, w := range wins {
		if w.ID == id {
			return w.Layout.PosInScrollingLayout[0], true
		}
	}
	return 0, false
}

// Apply spawns spawn's windows in order and, for consecutive entries that
// share a saved column, folds each new window into the previous one's
// column via consume-window-into-column -- but only when the new window
// actually landed in the very next column, since that's the only way to
// be sure it's the right window to consume. If reuse is non-nil, its
// window is already open as reuse.ID and is used only to seed that
// stacking state, never spawned or repositioned itself.
func (b *Backend) Apply(ctx context.Context, spawn []backend.PlannedWindow, reuse *backend.Reuse) error {
	ws, err := b.focusedWorkspaceID(ctx)
	if err != nil {
		return err
	}

	var prevID, prevCol int
	havePrev := false
	known := map[int]bool{}

	if reuse != nil {
		var l layout
		if err := json.Unmarshal(reuse.Window.Layout, &l); err != nil {
			return fmt.Errorf("decoding saved layout: %w", err)
		}
		id, err := strconv.Atoi(reuse.ID)
		if err != nil {
			return fmt.Errorf("invalid reuse window id %q: %w", reuse.ID, err)
		}
		prevID, prevCol, havePrev = id, l.Col, true
		known[id] = true
	}

	for _, p := range spawn {
		var l layout
		if err := json.Unmarshal(p.Layout, &l); err != nil {
			return fmt.Errorf("decoding saved layout: %w", err)
		}

		switch p.Kind {
		case "zmx":
			if err := ghostty.Spawn(b.WorkDir, []string{"zmx", "attach", p.Session}); err != nil {
				return err
			}
		default:
			if err := ghostty.Spawn(b.WorkDir, nil); err != nil {
				return err
			}
		}

		newID, wins, err := b.waitForNewWindow(ctx, ws, known)
		if err != nil {
			return err
		}

		if havePrev && l.Col == prevCol {
			newCol, newOK := columnOf(wins, newID)
			prevWinCol, prevOK := columnOf(wins, prevID)
			if newOK && prevOK && newCol == prevWinCol+1 {
				if _, err := b.Run(ctx, "niri", "msg", "action", "focus-window", "--id", strconv.Itoa(prevID)); err != nil {
					return err
				}
				if _, err := b.Run(ctx, "niri", "msg", "action", "consume-window-into-column"); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(b.out(), "warning: could not stack window under column %d; leaving it as its own column\n", l.Col)
			}
		}

		// wins already reflects every window present after this spawn
		// (including newID); reuse it as the next iteration's known set
		// instead of re-fetching it from niri.
		known = make(map[int]bool, len(wins))
		for _, w := range wins {
			known[w.ID] = true
		}

		prevID, prevCol, havePrev = newID, l.Col, true
	}
	return nil
}
