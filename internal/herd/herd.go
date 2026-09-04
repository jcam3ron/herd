// Package herd is the backend-agnostic core: classifying windows as
// zmx-backed or plain, and driving save/restore/list/rm against whatever
// backend.Backend is configured.
package herd

import (
	"context"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/jcam3ron/herd/internal/backend"
	"github.com/jcam3ron/herd/internal/snapshot"
	"github.com/jcam3ron/herd/internal/zmxclient"
)

// windowLabel is the human-readable identifier for a planned window: its
// zmx session if it has one, else its plain title.
func windowLabel(w backend.PlannedWindow) string {
	if w.Session != "" {
		return w.Session
	}
	return w.Title
}

// classified pairs a backend.PlannedWindow with the raw window id it came
// from, needed for Close/Wait during restore.
type classified struct {
	backend.PlannedWindow
	ID string
}

// classify tags each raw window as zmx-backed (by its last_window label,
// see the zmxclient package doc) or plain.
func classify(ctx context.Context, zmx *zmxclient.Client, raws []backend.RawWindow) ([]classified, error) {
	labels, err := zmx.LastWindowLabels(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]classified, len(raws))
	for i, w := range raws {
		if session, ok := labels[w.ID]; ok {
			out[i] = classified{backend.PlannedWindow{Kind: "zmx", Session: session, Layout: w.Layout}, w.ID}
		} else {
			out[i] = classified{backend.PlannedWindow{Kind: "plain", Title: w.Title, Layout: w.Layout}, w.ID}
		}
	}
	return out, nil
}

// excludeFocused removes the focused window (if any) from raws, returning
// its id -- the window herd itself is running in, whether that's Restore
// checking what it may close, or RestoreInPlace protecting itself from
// closing its own window.
func excludeFocused(raws []backend.RawWindow) (rest []backend.RawWindow, focusedID string) {
	if i := slices.IndexFunc(raws, func(w backend.RawWindow) bool { return w.Focused }); i >= 0 {
		focusedID = raws[i].ID
	}
	return slices.DeleteFunc(raws, func(w backend.RawWindow) bool { return w.Focused }), focusedID
}

// App wires a Backend, a zmx client and a snapshot Store into the
// save/restore/list/rm operations.
type App struct {
	Backend backend.Backend
	Zmx     *zmxclient.Client
	Store   *snapshot.Store
	Stdout  io.Writer
	// Confirm asks the user a yes/no question before a destructive
	// action (closing a window with no zmx session). It returns true to
	// proceed.
	Confirm func(prompt string) bool
	// Relaunch re-invokes herd's own "restore-in-place" command for name
	// in a new window (see cmd/herd/main.go). Restore calls it once it's
	// established the restore may proceed.
	Relaunch func(ctx context.Context, name string) error
}

func (a *App) Save(ctx context.Context, name string, force bool) error {
	if !force && a.Store.Exists(name) {
		fmt.Fprintf(a.Stdout, "Snapshot %q already exists and will be overwritten.\n", name)
		if !a.Confirm("Continue?") {
			return fmt.Errorf("save aborted")
		}
	}

	raws, err := a.Backend.ListWindows(ctx)
	if err != nil {
		return err
	}
	if len(raws) == 0 {
		return fmt.Errorf("no windows in the focused workspace to save")
	}

	windows, err := classify(ctx, a.Zmx, raws)
	if err != nil {
		return err
	}

	planned := make([]backend.PlannedWindow, len(windows))
	for i, w := range windows {
		planned[i] = w.PlannedWindow
	}

	snap := snapshot.Snapshot{Backend: a.Backend.Name(), Name: name, SavedAt: time.Now(), Windows: planned}
	if err := a.Store.Save(snap); err != nil {
		return err
	}

	fmt.Fprintf(a.Stdout, "saved %q: %d window(s) -> %s\n", name, len(planned), a.Store.Path(name))
	for _, w := range planned {
		fmt.Fprintf(a.Stdout, "  %s: %s\n", w.Kind, windowLabel(w))
	}
	return nil
}

// restorePlan is the preamble both Restore and RestoreInPlace need: the
// loaded snapshot, the workspace's other windows (everything but the one
// herd itself is running in) classified as zmx-backed or plain, and that
// window's id, if any. Restore uses it to say what closing would lose;
// RestoreInPlace uses it to actually close and replace those windows.
type restorePlan struct {
	snap    snapshot.Snapshot
	current []classified
	ownID   string
}

// planRestore loads name and validates it's restorable on the active
// backend -- a bad name, or one saved for a different backend, must be
// reported before anything is closed or spawned, not discovered partway
// through.
func (a *App) planRestore(ctx context.Context, name string) (restorePlan, error) {
	snap, err := a.Store.Load(name)
	if err != nil {
		return restorePlan{}, err
	}
	if snap.Backend != a.Backend.Name() {
		return restorePlan{}, fmt.Errorf("snapshot %q was saved with backend %q, active backend is %q", name, snap.Backend, a.Backend.Name())
	}

	raws, err := a.Backend.ListWindows(ctx)
	if err != nil {
		return restorePlan{}, err
	}
	raws, ownID := excludeFocused(raws)

	current, err := classify(ctx, a.Zmx, raws)
	if err != nil {
		return restorePlan{}, err
	}

	return restorePlan{snap: snap, current: current, ownID: ownID}, nil
}

// Restore relaunches itself in a new window to perform the actual
// restore there (RestoreInPlace), rather than doing the work in the
// window it was invoked from. It runs the "will lose content" check
// here, before spawning: doing it from RestoreInPlace (after the new
// window exists) would count the window Restore was invoked from as
// content to be lost, when losing it is implicit in having just run
// this command -- planRestore already excludes it for that reason.
func (a *App) Restore(ctx context.Context, name string, force bool) error {
	plan, err := a.planRestore(ctx, name)
	if err != nil {
		return err
	}

	var plain []classified
	for _, c := range plan.current {
		if c.Kind == "plain" {
			plain = append(plain, c)
		}
	}
	if len(plain) > 0 {
		fmt.Fprintln(a.Stdout, "The following terminal(s) in this workspace have no zmx session and will lose their content:")
		for _, c := range plain {
			fmt.Fprintf(a.Stdout, "  - %s\n", c.Title)
		}
		if !force && !a.Confirm("Close them anyway?") {
			return fmt.Errorf("restore aborted")
		}
	}

	if err := a.Relaunch(ctx, name); err != nil {
		return fmt.Errorf("couldn't open a new window to restore in: %w", err)
	}
	fmt.Fprintf(a.Stdout, "restoring %q in a new window\n", name)
	return nil
}

// RestoreInPlace does the actual work: closing what's open and
// respawning the saved layout, treating the window it runs in as the
// snapshot's first window (see backend.Reuse). It's meant to be invoked
// only via the internal "restore-in-place" CLI command that Restore
// relaunches into, not called directly -- Restore has already run (and
// gotten the user past) the "will lose content" check, so this doesn't
// repeat it.
func (a *App) RestoreInPlace(ctx context.Context, name string) error {
	plan, err := a.planRestore(ctx, name)
	if err != nil {
		return err
	}
	snap, current, ownID := plan.snap, plan.current, plan.ownID

	// Closing a zmx-backed window just hangs up the client; the session
	// keeps running in the zmx daemon (zmx has no remote detach-by-name,
	// so "close the window" *is* the detach here).
	for _, c := range current {
		if err := a.Backend.Close(ctx, c.ID); err != nil {
			return err
		}
		if err := a.Backend.Wait(ctx, c.ID); err != nil {
			return err
		}
	}

	// The window herd was invoked from always takes the snapshot's first
	// window: if that's zmx-backed, this process execs into `zmx attach`
	// for it below instead of a new window being spawned; if it's plain,
	// it's already sitting at a blank shell prompt, which is exactly
	// what a plain slot means, so there's nothing further to do for it.
	spawn := snap.Windows
	var reuse *backend.Reuse
	if ownID != "" {
		reuse = &backend.Reuse{ID: ownID, Window: snap.Windows[0]}
		spawn = snap.Windows[1:]
	}

	for _, w := range spawn {
		if w.Kind != "zmx" {
			fmt.Fprintf(a.Stdout, "note: %q had no zmx session, opening a blank terminal in its place\n", w.Title)
		}
	}

	newIDs, err := a.Backend.Apply(ctx, spawn, reuse)
	if err != nil {
		return err
	}

	// Label each spawned zmx window ourselves, rather than depending on
	// the shell `zmx` wrapper: these windows are spawned via `ghostty -e
	// zmx attach ...` directly, never through an interactive shell, so
	// the wrapper never runs for them.
	for i, w := range spawn {
		if w.Kind == "zmx" {
			if err := a.Zmx.SetLastWindow(ctx, w.Session, newIDs[i]); err != nil {
				fmt.Fprintf(a.Stdout, "warning: could not label zmx session %q for future saves: %v\n", w.Session, err)
			}
		}
	}

	fmt.Fprintf(a.Stdout, "restored %q\n", name)

	if reuse != nil && reuse.Window.Kind == "zmx" {
		if err := a.Zmx.SetLastWindow(ctx, reuse.Window.Session, reuse.ID); err != nil {
			fmt.Fprintf(a.Stdout, "warning: could not label zmx session %q for future saves: %v\n", reuse.Window.Session, err)
		}
		if err := a.Zmx.Attach(reuse.Window.Session); err != nil {
			return fmt.Errorf("restored, but could not attach this terminal to zmx session %q: %w", reuse.Window.Session, err)
		}
		// unreachable on success: Attach replaces this process
	}
	return nil
}

func (a *App) Show(name string) error {
	snap, err := a.Store.Load(name)
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "name:    %s\n", snap.Name)
	fmt.Fprintf(a.Stdout, "backend: %s\n", snap.Backend)
	fmt.Fprintf(a.Stdout, "saved:   %s\n", snap.SavedAt.Format(time.RFC3339))
	fmt.Fprintf(a.Stdout, "windows: %d\n", len(snap.Windows))
	for i, w := range snap.Windows {
		fmt.Fprintf(a.Stdout, "  [%d] %s: %s (layout: %s)\n", i, w.Kind, windowLabel(w), w.Layout)
	}
	return nil
}

func (a *App) List() error {
	snaps, err := a.Store.List()
	if err != nil {
		return err
	}
	for _, s := range snaps {
		fmt.Fprintf(a.Stdout, "%-24s %d window(s)\n", s.Name, len(s.Windows))
	}
	return nil
}

func (a *App) Remove(name string, force bool) error {
	if !force && a.Store.Exists(name) {
		fmt.Fprintf(a.Stdout, "Snapshot %q will be permanently deleted.\n", name)
		if !a.Confirm("Continue?") {
			return fmt.Errorf("remove aborted")
		}
	}
	if err := a.Store.Remove(name); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "removed %q\n", name)
	return nil
}

// ConfirmPrompt builds a Confirm function that asks the user on out and
// reads a y/N answer from in.
func ConfirmPrompt(in io.Reader, out io.Writer) func(string) bool {
	return func(prompt string) bool {
		fmt.Fprintf(out, "%s [y/N] ", prompt)
		var ans string
		_, _ = fmt.Fscanln(in, &ans) // EOF or no input leaves ans empty, correctly treated as "no" below
		return ans == "y" || ans == "Y"
	}
}
