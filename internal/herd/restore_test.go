package herd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"testing"

	"github.com/jcam3ron/herd/internal/backend"
	"github.com/jcam3ron/herd/internal/snapshot"
	"github.com/jcam3ron/herd/internal/zmxclient"
)

// fakeBackend is a minimal backend.Backend for exercising RestoreInPlace
// without shelling out to niri.
type fakeBackend struct {
	name    string
	windows []backend.RawWindow
	closed  []string
	spawn   []backend.PlannedWindow
	reuse   *backend.Reuse
}

func (b *fakeBackend) Name() string { return b.name }

func (b *fakeBackend) ListWindows(context.Context) ([]backend.RawWindow, error) {
	return b.windows, nil
}

func (b *fakeBackend) Close(_ context.Context, id string) error {
	b.closed = append(b.closed, id)
	return nil
}

func (b *fakeBackend) Wait(context.Context, string) error { return nil }

func (b *fakeBackend) Apply(_ context.Context, spawn []backend.PlannedWindow, reuse *backend.Reuse) ([]string, error) {
	b.spawn = spawn
	b.reuse = reuse
	ids := make([]string, len(spawn))
	for i := range spawn {
		ids[i] = fmt.Sprintf("spawned-%d", i)
	}
	return ids, nil
}

// zmxSetCalls wraps zmx's Run to additionally record "zmx set ..." calls
// (SetLastWindow), which fakeZmx's own Run otherwise ignores.
func zmxSetCalls(zmx *zmxclient.Client) *[][]string {
	var calls [][]string
	inner := zmx.Run
	zmx.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if name == "zmx" && len(args) > 0 && args[0] == "set" {
			calls = append(calls, args)
		}
		return inner(ctx, name, args...)
	}
	return &calls
}

func TestRestoreInPlaceExcludesOwnWindow(t *testing.T) {
	const ownWindowID = "1"
	fb := &fakeBackend{
		name: "niri",
		windows: []backend.RawWindow{
			{ID: ownWindowID, Title: "zmx:running-herd", Layout: []byte(`{"col":1,"row":1}`), Focused: true},
			{ID: "2", Title: "zmx:other", Layout: []byte(`{"col":2,"row":1}`)},
		},
	}
	store := &snapshot.Store{Dir: t.TempDir()}
	snap := snapshot.Snapshot{
		Backend: "niri",
		Name:    "proj",
		Windows: []backend.PlannedWindow{
			// slot 0: reused into herd's own window (id "1")
			{Kind: "zmx", Session: "reattach-here", Layout: []byte(`{"col":1,"row":1}`)},
			// slot 1: a genuinely new window to spawn
			{Kind: "zmx", Session: "other", Layout: []byte(`{"col":2,"row":1}`)},
		},
	}
	if err := store.Save(snap); err != nil {
		t.Fatalf("Save: %v", err)
	}

	var attached string
	zmx := fakeZmx(nil)
	zmx.Attach = func(session string) error {
		attached = session
		return nil
	}
	setCalls := zmxSetCalls(zmx)

	app := &App{
		Backend: fb,
		Zmx:     zmx,
		Store:   store,
		Stdout:  io.Discard,
		Confirm: func(string) bool { return true },
	}

	if err := app.RestoreInPlace(context.Background(), "proj"); err != nil {
		t.Fatalf("RestoreInPlace: %v", err)
	}

	// Both the reused window (id "1") and the newly spawned one must be
	// labeled by herd itself -- neither goes through an interactive
	// shell, so the shell `zmx` wrapper never runs for either.
	wantSetCalls := [][]string{
		{"set", "other", "last_window=spawned-0"},
		{"set", "reattach-here", "last_window=1"},
	}
	if len(*setCalls) != len(wantSetCalls) {
		t.Fatalf("zmx set calls = %v, want %v", *setCalls, wantSetCalls)
	}
	for i, want := range wantSetCalls {
		if !slices.Equal((*setCalls)[i], want) {
			t.Errorf("zmx set call %d = %v, want %v", i, (*setCalls)[i], want)
		}
	}

	for _, id := range fb.closed {
		if id == ownWindowID {
			t.Fatalf("RestoreInPlace closed its own window (id %q): closed = %v", ownWindowID, fb.closed)
		}
	}
	if len(fb.closed) != 1 || fb.closed[0] != "2" {
		t.Errorf("closed = %v, want [\"2\"]", fb.closed)
	}
	if fb.reuse == nil || fb.reuse.ID != ownWindowID {
		t.Fatalf("Apply reuse = %+v, want ID %q", fb.reuse, ownWindowID)
	}
	if fb.reuse.Window.Session != "reattach-here" {
		t.Errorf("Apply reuse.Window.Session = %q, want %q (slot 0)", fb.reuse.Window.Session, "reattach-here")
	}
	if len(fb.spawn) != 1 || fb.spawn[0].Session != "other" {
		t.Errorf("Apply spawn = %+v, want just slot 1 (\"other\")", fb.spawn)
	}
	if attached != "reattach-here" {
		t.Errorf("Zmx.Attach session = %q, want %q (slot 0's session)", attached, "reattach-here")
	}
}

func TestRestoreInPlaceReusedSlotPlain(t *testing.T) {
	const ownWindowID = "1"
	fb := &fakeBackend{
		name: "niri",
		windows: []backend.RawWindow{
			{ID: ownWindowID, Title: "plain shell", Layout: []byte(`{"col":1,"row":1}`), Focused: true},
		},
	}
	store := &snapshot.Store{Dir: t.TempDir()}
	snap := snapshot.Snapshot{
		Backend: "niri",
		Name:    "proj",
		Windows: []backend.PlannedWindow{
			{Kind: "plain", Title: "plain shell", Layout: []byte(`{"col":1,"row":1}`)},
		},
	}
	if err := store.Save(snap); err != nil {
		t.Fatalf("Save: %v", err)
	}

	attachCalled := false
	zmx := fakeZmx(nil)
	zmx.Attach = func(string) error {
		attachCalled = true
		return nil
	}
	setCalls := zmxSetCalls(zmx)

	app := &App{
		Backend: fb,
		Zmx:     zmx,
		Store:   store,
		Stdout:  io.Discard,
	}

	// The reused slot is plain (id "1"), so it's excluded from raws
	// before classification -- there's nothing left to close or spawn
	// for it.
	if err := app.RestoreInPlace(context.Background(), "proj"); err != nil {
		t.Fatalf("RestoreInPlace: %v", err)
	}

	if attachCalled {
		t.Error("Zmx.Attach was called for a plain reused slot")
	}
	if len(*setCalls) != 0 {
		t.Errorf("zmx set calls = %v, want none for a plain reused slot", *setCalls)
	}
	if fb.reuse == nil || fb.reuse.ID != ownWindowID {
		t.Fatalf("Apply reuse = %+v, want ID %q", fb.reuse, ownWindowID)
	}
	if len(fb.spawn) != 0 {
		t.Errorf("Apply spawn = %+v, want none", fb.spawn)
	}
	if len(fb.closed) != 0 {
		t.Errorf("closed = %v, want none", fb.closed)
	}
}

// TestRestoreIgnoresOwnWindowForContentWarning is the fix for the bug
// where running `herd restore` from a plain, zmx-less terminal (the
// normal case) always triggered the "will lose content" warning: the
// window Restore was invoked from is not yet excluded from the window
// list the way RestoreInPlace excludes its own (new) window, so it was
// counted as content that would be lost. Confirm always declines here,
// so if it were consulted at all, Restore would abort.
func TestRestoreIgnoresOwnWindowForContentWarning(t *testing.T) {
	fb := &fakeBackend{
		name: "niri",
		windows: []backend.RawWindow{
			{ID: "1", Title: "invoking shell", Focused: true},
		},
	}
	store := &snapshot.Store{Dir: t.TempDir()}
	seedSnapshot(t, store, "proj")

	spawned := false
	app := &App{
		Backend:  fb,
		Zmx:      fakeZmx(nil),
		Store:    store,
		Stdout:   io.Discard,
		Confirm:  func(string) bool { return false },
		Relaunch: func(context.Context, string) error { spawned = true; return nil },
	}

	if err := app.Restore(context.Background(), "proj", false); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !spawned {
		t.Error("Relaunch was not called: Confirm must have been (wrongly) consulted about the invoking window")
	}
}

// TestRestoreWarnsAboutOtherPlainWindows confirms the content-loss check
// still fires for windows other than the one Restore was invoked from.
func TestRestoreWarnsAboutOtherPlainWindows(t *testing.T) {
	fb := &fakeBackend{
		name: "niri",
		windows: []backend.RawWindow{
			{ID: "1", Title: "invoking shell", Focused: true},
			{ID: "2", Title: "unrelated plain terminal"},
		},
	}
	store := &snapshot.Store{Dir: t.TempDir()}
	seedSnapshot(t, store, "proj")

	spawned := false
	app := &App{
		Backend:  fb,
		Zmx:      fakeZmx(nil),
		Store:    store,
		Stdout:   io.Discard,
		Confirm:  func(string) bool { return false },
		Relaunch: func(context.Context, string) error { spawned = true; return nil },
	}

	if err := app.Restore(context.Background(), "proj", false); err == nil {
		t.Fatal("Restore: expected an error from a declined confirm, got nil")
	}
	if spawned {
		t.Error("Relaunch was called despite the confirm being declined")
	}
}

// TestRestoreForceSkipsConfirm confirms --force bypasses the
// content-loss confirm entirely, not just defaults it to "yes".
func TestRestoreForceSkipsConfirm(t *testing.T) {
	fb := &fakeBackend{
		name: "niri",
		windows: []backend.RawWindow{
			{ID: "1", Title: "invoking shell", Focused: true},
			{ID: "2", Title: "unrelated plain terminal"},
		},
	}
	store := &snapshot.Store{Dir: t.TempDir()}
	seedSnapshot(t, store, "proj")

	spawned := false
	app := &App{
		Backend:  fb,
		Zmx:      fakeZmx(nil),
		Store:    store,
		Stdout:   io.Discard,
		Confirm:  func(string) bool { return false },
		Relaunch: func(context.Context, string) error { spawned = true; return nil },
	}

	if err := app.Restore(context.Background(), "proj", true); err != nil {
		t.Fatalf("Restore with force=true: %v", err)
	}
	if !spawned {
		t.Error("Relaunch was not called despite force=true")
	}
}

// Restore always relaunches into a new window running "restore-in-place"
// -- there's no special-casing based on how or where it was invoked from
// (in particular, no $ZMX_SESSION check), so these tests don't set it.
// Each seeds a real snapshot: Restore validates upfront (name exists,
// backend matches) before ever spawning anything, so an unseeded store
// would fail before reaching the behavior under test.

func seedSnapshot(t *testing.T, store *snapshot.Store, name string) {
	t.Helper()
	snap := snapshot.Snapshot{
		Backend: "niri",
		Name:    name,
		Windows: []backend.PlannedWindow{{Kind: "plain", Title: "x", Layout: []byte(`{}`)}},
	}
	if err := store.Save(snap); err != nil {
		t.Fatalf("seedSnapshot: %v", err)
	}
}

func TestRestoreValidatesBeforeSpawning(t *testing.T) {
	spawnCalled := false
	app := &App{
		Backend:  &fakeBackend{name: "niri"},
		Store:    &snapshot.Store{Dir: t.TempDir()},
		Stdout:   io.Discard,
		Relaunch: func(context.Context, string) error { spawnCalled = true; return nil },
	}

	if err := app.Restore(context.Background(), "nonexistent", false); err == nil {
		t.Fatal("Restore of a nonexistent snapshot: expected an error, got nil")
	}
	if spawnCalled {
		t.Error("Relaunch was called despite the snapshot not existing")
	}
}

func TestRestoreValidatesBackendBeforeSpawning(t *testing.T) {
	spawnCalled := false
	store := &snapshot.Store{Dir: t.TempDir()}
	snap := snapshot.Snapshot{
		Backend: "omniwm",
		Name:    "proj",
		Windows: []backend.PlannedWindow{{Kind: "plain", Title: "x", Layout: []byte(`{}`)}},
	}
	if err := store.Save(snap); err != nil {
		t.Fatalf("Save: %v", err)
	}

	app := &App{
		Backend:  &fakeBackend{name: "niri"},
		Store:    store,
		Stdout:   io.Discard,
		Relaunch: func(context.Context, string) error { spawnCalled = true; return nil },
	}

	if err := app.Restore(context.Background(), "proj", false); err == nil {
		t.Fatal("Restore of a snapshot saved for a different backend: expected an error, got nil")
	}
	if spawnCalled {
		t.Error("Relaunch was called despite the backend mismatch")
	}
}

func TestRestoreRelaunches(t *testing.T) {
	store := &snapshot.Store{Dir: t.TempDir()}
	seedSnapshot(t, store, "anything")

	var relaunchedName string
	app := &App{
		Backend: &fakeBackend{name: "niri"},
		Zmx:     fakeZmx(nil),
		Store:   store,
		Stdout:  io.Discard,
		Relaunch: func(_ context.Context, name string) error {
			relaunchedName = name
			return nil
		},
	}

	if err := app.Restore(context.Background(), "anything", true); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if relaunchedName != "anything" {
		t.Errorf("Relaunch called with name %q, want %q", relaunchedName, "anything")
	}
}

func TestRestoreSurfacesSpawnFailure(t *testing.T) {
	store := &snapshot.Store{Dir: t.TempDir()}
	seedSnapshot(t, store, "anything")

	app := &App{
		Backend:  &fakeBackend{name: "niri"},
		Zmx:      fakeZmx(nil),
		Store:    store,
		Stdout:   io.Discard,
		Relaunch: func(context.Context, string) error { return errors.New("no ghostty on PATH") },
	}

	if err := app.Restore(context.Background(), "anything", false); err == nil {
		t.Fatal("Restore when Relaunch fails: expected an error, got nil")
	}
}
