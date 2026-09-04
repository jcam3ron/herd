package snapshot

import (
	"testing"
	"time"

	"github.com/jcam3ron/herd/internal/backend"
)

func TestSaveLoadListRemove(t *testing.T) {
	s := &Store{Dir: t.TempDir()}

	snap := Snapshot{
		Backend: "niri",
		Name:    "myproject",
		SavedAt: time.Now().Truncate(time.Second),
		Windows: []backend.PlannedWindow{
			{Kind: "zmx", Session: "nixos-config", Layout: []byte(`{"col":1,"row":1}`)},
			{Kind: "plain", Title: "scratch", Layout: []byte(`{"col":1,"row":2}`)},
		},
	}

	if err := s.Save(snap); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := s.Load("myproject")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Backend != snap.Backend || len(got.Windows) != len(snap.Windows) {
		t.Fatalf("Load round-trip mismatch: got %+v, want %+v", got, snap)
	}
	if !got.SavedAt.Equal(snap.SavedAt) {
		t.Errorf("SavedAt: got %v, want %v", got.SavedAt, snap.SavedAt)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].Name != "myproject" {
		t.Fatalf("List = %+v, want one snapshot named myproject", list)
	}

	if err := s.Remove("myproject"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := s.Load("myproject"); err == nil {
		t.Fatal("Load after Remove: expected an error, got nil")
	}
}

func TestLoadMissing(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if _, err := s.Load("nope"); err == nil {
		t.Fatal("Load of a missing snapshot: expected an error, got nil")
	}
}
