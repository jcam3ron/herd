package herd

import (
	"context"
	"io"
	"testing"

	"github.com/jcam3ron/herd/internal/backend"
	"github.com/jcam3ron/herd/internal/snapshot"
)

// fakeBackend is a minimal backend.Backend for exercising Restore without
// shelling out to niri.
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

func (b *fakeBackend) Apply(_ context.Context, spawn []backend.PlannedWindow, reuse *backend.Reuse) error {
	b.spawn = spawn
	b.reuse = reuse
	return nil
}

func TestRestoreExcludesOwnWindow(t *testing.T) {
	t.Setenv("ZMX_SESSION", "") // this test isn't exercising the zmx-session guard

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

	app := &App{
		Backend: fb,
		Zmx:     zmx,
		Store:   store,
		Stdout:  io.Discard,
		Confirm: func(string) bool { return true },
	}

	if err := app.Restore(context.Background(), "proj", false); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	for _, id := range fb.closed {
		if id == ownWindowID {
			t.Fatalf("Restore closed its own window (id %q): closed = %v", ownWindowID, fb.closed)
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

func TestRestoreReusedSlotPlain(t *testing.T) {
	t.Setenv("ZMX_SESSION", "")

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

	app := &App{
		Backend: fb,
		Zmx:     zmx,
		Store:   store,
		Stdout:  io.Discard,
		Confirm: func(string) bool { return true },
	}

	// The reused slot is plain (id "1"), so it's excluded from raws
	// before classification -- there's nothing left to warn about or
	// confirm, and Confirm should never be consulted.
	if err := app.Restore(context.Background(), "proj", false); err != nil {
		t.Fatalf("Restore: %v", err)
	}

	if attachCalled {
		t.Error("Zmx.Attach was called for a plain reused slot")
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

func TestRestoreRefusesInsideZmxSession(t *testing.T) {
	t.Setenv("ZMX_SESSION", "some-session")

	app := &App{
		Backend: &fakeBackend{name: "niri"},
		Zmx:     fakeZmx(nil),
		Store:   &snapshot.Store{Dir: t.TempDir()},
		Stdout:  io.Discard,
		Confirm: func(string) bool { return true },
	}

	if err := app.Restore(context.Background(), "anything", false); err == nil {
		t.Fatal("Restore from inside a zmx session: expected an error, got nil")
	}
}

func TestRestoreForceSkipsConfirm(t *testing.T) {
	t.Setenv("ZMX_SESSION", "")

	fb := &fakeBackend{
		name: "niri",
		windows: []backend.RawWindow{
			{ID: "2", Title: "no zmx session here"},
		},
	}
	store := &snapshot.Store{Dir: t.TempDir()}
	snap := snapshot.Snapshot{
		Backend: "niri",
		Name:    "proj",
		Windows: []backend.PlannedWindow{
			{Kind: "plain", Title: "no zmx session here", Layout: []byte(`{"col":1,"row":1}`)},
		},
	}
	if err := store.Save(snap); err != nil {
		t.Fatalf("Save: %v", err)
	}

	app := &App{
		Backend: fb,
		Zmx:     fakeZmx(nil),
		Store:   store,
		Stdout:  io.Discard,
		// Confirm always declines: force must bypass it entirely, not
		// just default it to "yes".
		Confirm: func(string) bool { return false },
	}

	if err := app.Restore(context.Background(), "proj", true); err != nil {
		t.Fatalf("Restore with force=true: %v", err)
	}
}
