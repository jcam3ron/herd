package niri

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/jcam3ron/herd/internal/backend"
	"github.com/jcam3ron/herd/internal/ghostty"
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

// mkWindow builds a ghostty window in workspace ws at (col, row) --
// enough of niri's `windows` payload for the fields Apply's stacking
// heuristic reads.
func mkWindow(id, ws, col, row int) window {
	w := window{ID: id, AppID: ghostty.AppID, WorkspaceID: ws}
	w.Layout.PosInScrollingLayout = [2]int{col, row}
	return w
}

// runFake answers the "niri msg" subcommands Apply issues against a
// single focused workspace (id 4) and a live windows list, recording
// action calls so the stacking decision can be asserted on.
type runFake struct {
	windows      []window
	focusCalls   []string
	consumeCalls int
}

func (f *runFake) run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name != "niri" || args[0] != "msg" {
		return nil, nil
	}
	switch {
	case args[1] == "-j" && args[2] == "workspaces":
		return []byte(`[{"id":4,"is_focused":true}]`), nil
	case args[1] == "-j" && args[2] == "windows":
		return json.Marshal(f.windows)
	case args[1] == "action" && args[2] == "focus-window":
		f.focusCalls = append(f.focusCalls, args[4])
		return nil, nil
	case args[1] == "action" && args[2] == "consume-window-into-column":
		f.consumeCalls++
		return nil, nil
	}
	return nil, nil
}

func mustLayout(t *testing.T, col, row int) []byte {
	t.Helper()
	b, err := json.Marshal(layout{Col: col, Row: row})
	if err != nil {
		t.Fatalf("marshal layout: %v", err)
	}
	return b
}

func TestApplyFoldsAdjacentColumn(t *testing.T) {
	f := &runFake{windows: []window{mkWindow(1, 4, 1, 1)}} // reused window: id 1, col 1
	b := &Backend{
		Run: f.run,
		Spawn: func(string, []string) error {
			// Lands one column right of the reused window -- adjacent,
			// so Apply should fold it into that column.
			f.windows = append(f.windows, mkWindow(2, 4, 2, 1))
			return nil
		},
	}

	reuse := &backend.Reuse{ID: "1", Window: backend.PlannedWindow{Layout: mustLayout(t, 1, 1)}}
	spawn := []backend.PlannedWindow{{Kind: "plain", Layout: mustLayout(t, 1, 1)}}

	ids, err := b.Apply(context.Background(), spawn, reuse)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(ids) != 1 || ids[0] != "2" {
		t.Fatalf("ids = %v, want [\"2\"]", ids)
	}
	if !slices.Equal(f.focusCalls, []string{"1"}) {
		t.Errorf("focus-window calls = %v, want [\"1\"]", f.focusCalls)
	}
	if f.consumeCalls != 1 {
		t.Errorf("consume-window-into-column calls = %d, want 1", f.consumeCalls)
	}
}

func TestApplyWarnsOnNonAdjacentColumn(t *testing.T) {
	f := &runFake{windows: []window{mkWindow(1, 4, 1, 1)}} // reused window: id 1, col 1
	var out bytes.Buffer
	b := &Backend{
		Run: f.run,
		Spawn: func(string, []string) error {
			// Lands far from the reused window -- not adjacent, so Apply
			// must not fold it and should warn instead.
			f.windows = append(f.windows, mkWindow(2, 4, 5, 1))
			return nil
		},
		Out: &out,
	}

	reuse := &backend.Reuse{ID: "1", Window: backend.PlannedWindow{Layout: mustLayout(t, 1, 1)}}
	spawn := []backend.PlannedWindow{{Kind: "plain", Layout: mustLayout(t, 1, 1)}}

	ids, err := b.Apply(context.Background(), spawn, reuse)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(ids) != 1 || ids[0] != "2" {
		t.Fatalf("ids = %v, want [\"2\"]", ids)
	}
	if len(f.focusCalls) != 0 || f.consumeCalls != 0 {
		t.Errorf("focus-window/consume-window-into-column calls = %v/%d, want none", f.focusCalls, f.consumeCalls)
	}
	if !strings.Contains(out.String(), "could not stack") {
		t.Errorf("Out = %q, want a stacking warning", out.String())
	}
}
