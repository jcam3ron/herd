// Package herd is the backend-agnostic core: classifying windows as
// zmx-backed or plain, and driving save/restore/list/rm against whatever
// backend.Backend is configured.
package herd

import (
	"context"
	"fmt"
	"io"
	"os"
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

// classify tags each raw window as zmx-backed (by its last_window label)
// or plain. herd never reads or sets the window title: it's the sole
// source of truth for which window belongs to which zmx session, set by
// itself for windows it spawns (see RestoreInPlace) and by the shell
// `zmx` wrapper for windows attached manually (see shell/herd.fish).
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
	Confirm     func(prompt string) bool
	SpawnWindow func(cmd []string) error
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

// Restore relaunches itself in a new window to perform the actual
// restore there (RestoreInPlace), rather than doing the work in the
// window it was invoked from. It checks upfront that name is actually
// restorable -- a bad name, or one saved for a different backend, should
// fail immediately in the calling window, not spawn a window only to
// fail inside it.
func (a *App) Restore(ctx context.Context, name string, force bool) error {
	snap, err := a.Store.Load(name)
	if err != nil {
		return err
	}
	if snap.Backend != a.Backend.Name() {
		return fmt.Errorf("snapshot %q was saved with backend %q, active backend is %q", name, snap.Backend, a.Backend.Name())
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("couldn't find herd's own binary to relaunch it: %w", err)
	}
	restoreCmd := []string{exe, "restore-in-place"}
	if force {
		restoreCmd = append(restoreCmd, "--force")
	}
	restoreCmd = append(restoreCmd, name)
	// Run through a shell that falls back to an interactive login shell
	// once the restore command exits, no matter why -- success on a
	// plain reused slot (nothing further to run), a failure, or later
	// once an exec'd `zmx attach` itself exits. Without this, the window
	// would run the restore command directly as its foreground process,
	// and ghostty closes a window the instant its foreground process
	// exits.
	cmd := append([]string{"sh", "-c", `"$@"; exec "${SHELL:-/bin/sh}" -l`, "sh"}, restoreCmd...)
	if err := a.SpawnWindow(cmd); err != nil {
		return fmt.Errorf("couldn't open a new window to restore in: %w", err)
	}
	fmt.Fprintf(a.Stdout, "restoring %q in a new window\n", name)
	return nil
}

// RestoreInPlace does the actual work: closing what's open and
// respawning the saved layout, treating the window it runs in as the
// snapshot's first window (see backend.Reuse). It's meant to be invoked
// only via the internal "restore-in-place" CLI command that Restore
// relaunches into, not called directly.
func (a *App) RestoreInPlace(ctx context.Context, name string, force bool) error {
	snap, err := a.Store.Load(name)
	if err != nil {
		return err
	}
	if snap.Backend != a.Backend.Name() {
		return fmt.Errorf("snapshot %q was saved with backend %q, active backend is %q", name, snap.Backend, a.Backend.Name())
	}

	raws, err := a.Backend.ListWindows(ctx)
	if err != nil {
		return err
	}

	// Never touch the window herd itself is running in -- closing it
	// would kill this process mid-restore.
	var ownID string
	if i := slices.IndexFunc(raws, func(w backend.RawWindow) bool { return w.Focused }); i >= 0 {
		ownID = raws[i].ID
		raws = slices.DeleteFunc(raws, func(w backend.RawWindow) bool { return w.ID == ownID })
	}

	current, err := classify(ctx, a.Zmx, raws)
	if err != nil {
		return err
	}

	var plain []classified
	for _, c := range current {
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
		fmt.Fscanln(in, &ans)
		return ans == "y" || ans == "Y"
	}
}
