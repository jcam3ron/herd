package herd

import (
	"io"
	"testing"

	"github.com/jcam3ron/herd/internal/backend"
	"github.com/jcam3ron/herd/internal/snapshot"
)

func TestRemoveWarnsBeforeDeleting(t *testing.T) {
	store := &snapshot.Store{Dir: t.TempDir()}
	snap := snapshot.Snapshot{
		Backend: "niri",
		Name:    "proj",
		Windows: []backend.PlannedWindow{{Kind: "plain", Title: "x", Layout: []byte(`{}`)}},
	}
	if err := store.Save(snap); err != nil {
		t.Fatalf("Save (seed): %v", err)
	}

	app := &App{
		Store:   store,
		Stdout:  io.Discard,
		Confirm: func(string) bool { return false },
	}

	if err := app.Remove("proj", false); err == nil {
		t.Fatal("Remove with Confirm declining: expected an error, got nil")
	}
	if !store.Exists("proj") {
		t.Error("snapshot was deleted despite Confirm declining")
	}
}

func TestRemoveForceSkipsConfirm(t *testing.T) {
	store := &snapshot.Store{Dir: t.TempDir()}
	snap := snapshot.Snapshot{
		Backend: "niri",
		Name:    "proj",
		Windows: []backend.PlannedWindow{{Kind: "plain", Title: "x", Layout: []byte(`{}`)}},
	}
	if err := store.Save(snap); err != nil {
		t.Fatalf("Save (seed): %v", err)
	}

	app := &App{
		Store:  store,
		Stdout: io.Discard,
		// Confirm always declines: force must bypass it entirely.
		Confirm: func(string) bool { return false },
	}

	if err := app.Remove("proj", true); err != nil {
		t.Fatalf("Remove with force=true: %v", err)
	}
	if store.Exists("proj") {
		t.Error("snapshot still exists after a forced remove")
	}
}

func TestRemoveMissingSkipsConfirm(t *testing.T) {
	store := &snapshot.Store{Dir: t.TempDir()}

	confirmCalled := false
	app := &App{
		Store:  store,
		Stdout: io.Discard,
		Confirm: func(string) bool {
			confirmCalled = true
			return true
		},
	}

	if err := app.Remove("nope", false); err == nil {
		t.Fatal("Remove of a missing snapshot: expected an error, got nil")
	}
	if confirmCalled {
		t.Error("Confirm was consulted for a name with no existing snapshot")
	}
}
