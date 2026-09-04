package herd

import (
	"context"
	"io"
	"testing"

	"github.com/jcam3ron/herd/internal/backend"
	"github.com/jcam3ron/herd/internal/snapshot"
)

func TestSaveWarnsOnOverwrite(t *testing.T) {
	fb := &fakeBackend{
		name:    "niri",
		windows: []backend.RawWindow{{ID: "1", Title: "irrelevant"}},
	}
	store := &snapshot.Store{Dir: t.TempDir()}
	original := snapshot.Snapshot{
		Backend: "niri",
		Name:    "proj",
		Windows: []backend.PlannedWindow{{Kind: "zmx", Session: "old-content", Layout: []byte(`{}`)}},
	}
	if err := store.Save(original); err != nil {
		t.Fatalf("Save (seed): %v", err)
	}

	app := &App{
		Backend: fb,
		Zmx:     fakeZmx(nil),
		Store:   store,
		Stdout:  io.Discard,
		Confirm: func(string) bool { return false },
	}

	if err := app.Save(context.Background(), "proj", false); err == nil {
		t.Fatal("Save over an existing snapshot with Confirm declining: expected an error, got nil")
	}

	got, err := store.Load("proj")
	if err != nil {
		t.Fatalf("Load after aborted overwrite: %v", err)
	}
	if len(got.Windows) != 1 || got.Windows[0].Session != "old-content" {
		t.Errorf("snapshot after aborted overwrite = %+v, want the original (old-content) untouched", got.Windows)
	}
}

func TestSaveForceSkipsOverwriteConfirm(t *testing.T) {
	fb := &fakeBackend{
		name:    "niri",
		windows: []backend.RawWindow{{ID: "1", Title: "irrelevant"}},
	}
	store := &snapshot.Store{Dir: t.TempDir()}
	original := snapshot.Snapshot{
		Backend: "niri",
		Name:    "proj",
		Windows: []backend.PlannedWindow{{Kind: "zmx", Session: "old-content", Layout: []byte(`{}`)}},
	}
	if err := store.Save(original); err != nil {
		t.Fatalf("Save (seed): %v", err)
	}

	app := &App{
		Backend: fb,
		Zmx:     fakeZmx(map[string]string{"new-content": "1"}),
		Store:   store,
		Stdout:  io.Discard,
		// Confirm always declines: force must bypass it entirely.
		Confirm: func(string) bool { return false },
	}

	if err := app.Save(context.Background(), "proj", true); err != nil {
		t.Fatalf("Save with force=true: %v", err)
	}

	got, err := store.Load("proj")
	if err != nil {
		t.Fatalf("Load after forced overwrite: %v", err)
	}
	if len(got.Windows) != 1 || got.Windows[0].Session != "new-content" {
		t.Errorf("snapshot after forced overwrite = %+v, want new-content", got.Windows)
	}
}

func TestSaveNewNameSkipsOverwriteConfirm(t *testing.T) {
	fb := &fakeBackend{
		name:    "niri",
		windows: []backend.RawWindow{{ID: "1", Title: "irrelevant"}},
	}
	store := &snapshot.Store{Dir: t.TempDir()}

	confirmCalled := false
	app := &App{
		Backend: fb,
		Zmx:     fakeZmx(nil),
		Store:   store,
		Stdout:  io.Discard,
		Confirm: func(string) bool {
			confirmCalled = true
			return true
		},
	}

	if err := app.Save(context.Background(), "brand-new", false); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if confirmCalled {
		t.Error("Confirm was consulted for a name with no existing snapshot")
	}
}
