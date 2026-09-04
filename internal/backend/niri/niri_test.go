package niri

import (
	"context"
	"encoding/json"
	"testing"
)

// sampleWindows mirrors real `niri msg -j windows` output captured from a
// live session (three ghostty windows, two zmx-tagged, in one workspace).
const sampleWindows = `[
  {"id":138,"title":"zmx:nixos-config","app_id":"com.mitchellh.ghostty","workspace_id":4,"is_focused":false,"layout":{"pos_in_scrolling_layout":[1,1]}},
  {"id":139,"title":"unrelated window","app_id":"com.mitchellh.ghostty","workspace_id":4,"is_focused":true,"layout":{"pos_in_scrolling_layout":[1,2]}},
  {"id":140,"title":"zmx:homelab-docs","app_id":"com.mitchellh.ghostty","workspace_id":4,"is_focused":false,"layout":{"pos_in_scrolling_layout":[2,1]}},
  {"id":999,"title":"zmx:other-workspace","app_id":"com.mitchellh.ghostty","workspace_id":7,"is_focused":false,"layout":{"pos_in_scrolling_layout":[1,1]}},
  {"id":1000,"title":"zmx:other-app","app_id":"org.example.other","workspace_id":4,"is_focused":false,"layout":{"pos_in_scrolling_layout":[3,1]}}
]`

const sampleWorkspaces = `[
  {"id":4,"is_focused":true},
  {"id":7,"is_focused":false}
]`

func TestListWindows(t *testing.T) {
	b := &Backend{Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "niri" || args[0] != "msg" {
			t.Fatalf("unexpected command: %s %v", name, args)
		}
		switch args[2] {
		case "workspaces":
			return []byte(sampleWorkspaces), nil
		case "windows":
			return []byte(sampleWindows), nil
		}
		t.Fatalf("unexpected niri msg target: %v", args)
		return nil, nil
	}}

	got, err := b.ListWindows(context.Background())
	if err != nil {
		t.Fatalf("ListWindows: %v", err)
	}

	// Only workspace 4's ghostty windows, in column/row order: 138 (1,1),
	// 139 (1,2), 140 (2,1). Window 999 is in the wrong workspace and 1000
	// is the wrong app_id -- both must be filtered out.
	wantIDs := []string{"138", "139", "140"}
	if len(got) != len(wantIDs) {
		t.Fatalf("got %d windows, want %d: %+v", len(got), len(wantIDs), got)
	}
	for i, id := range wantIDs {
		if got[i].ID != id {
			t.Errorf("window %d: got id=%s, want %s", i, got[i].ID, id)
		}
	}

	var l layout
	if err := json.Unmarshal(got[2].Layout, &l); err != nil {
		t.Fatalf("decoding layout: %v", err)
	}
	if l.Col != 2 || l.Row != 1 {
		t.Errorf("window 140 layout = %+v, want col=2 row=1", l)
	}

	for _, w := range got {
		want := w.ID == "139"
		if w.Focused != want {
			t.Errorf("window %s: Focused = %v, want %v", w.ID, w.Focused, want)
		}
	}
}
