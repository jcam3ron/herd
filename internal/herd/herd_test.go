package herd

import (
	"context"
	"testing"

	"github.com/jcam3ron/herd/internal/backend"
	"github.com/jcam3ron/herd/internal/zmxclient"
)

// fakeZmx answers `zmx list --short` / `zmx get <name>` from an in-memory
// map of session -> last_window label, so classification can be tested
// without shelling out. Its Attach is a no-op: real Attach replaces the
// calling process, which would take out the test binary itself.
func fakeZmx(lastWindow map[string]string) *zmxclient.Client {
	return &zmxclient.Client{
		Attach: func(string) error { return nil },
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name != "zmx" {
				return nil, nil
			}
			switch args[0] {
			case "list":
				var out string
				for session := range lastWindow {
					out += session + "\n"
				}
				return []byte(out), nil
			case "get":
				session := args[1]
				if wid, ok := lastWindow[session]; ok {
					return []byte("last_window=" + wid + "\n"), nil
				}
				return []byte(""), nil
			}
			return nil, nil
		},
	}
}

func TestClassify(t *testing.T) {
	zmx := fakeZmx(map[string]string{
		"claude-session": "42", // title overwritten by claude, no "zmx:" prefix
	})

	raws := []backend.RawWindow{
		{ID: "1", Title: "zmx:nixos-config", Layout: []byte(`{"col":1,"row":1}`)},
		{ID: "2", Title: "no session here", Layout: []byte(`{"col":1,"row":2}`)},
		{ID: "42", Title: "claude", Layout: []byte(`{"col":2,"row":1}`)},
	}

	got, err := classify(context.Background(), zmx, raws)
	if err != nil {
		t.Fatalf("classify: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d windows, want 3", len(got))
	}

	if got[0].Kind != "zmx" || got[0].Session != "nixos-config" {
		t.Errorf("window 0: got kind=%q session=%q, want zmx/nixos-config", got[0].Kind, got[0].Session)
	}
	if got[1].Kind != "plain" || got[1].Title != "no session here" {
		t.Errorf("window 1: got kind=%q title=%q, want plain/%q", got[1].Kind, got[1].Title, "no session here")
	}
	if got[2].Kind != "zmx" || got[2].Session != "claude-session" {
		t.Errorf("window 2 (label fallback): got kind=%q session=%q, want zmx/claude-session", got[2].Kind, got[2].Session)
	}
}

func TestSessionFromTitle(t *testing.T) {
	cases := []struct {
		title   string
		session string
		ok      bool
	}{
		{"zmx:nixos-config", "nixos-config", true},
		{"zmx:homelab-docs", "homelab-docs", true},
		{"◑ Project environment switcher for niri and zmx", "", false},
		{"zmx: has a space", "", false},
	}
	for _, c := range cases {
		session, ok := sessionFromTitle(c.title)
		if session != c.session || ok != c.ok {
			t.Errorf("sessionFromTitle(%q) = (%q, %v), want (%q, %v)", c.title, session, ok, c.session, c.ok)
		}
	}
}
