// Package backend defines the interface herd uses to talk to a window
// manager. A concrete implementation (niri today; others later, e.g. an
// omniwm backend for Mac) owns everything backend-specific: enumerating
// windows, closing them, spawning replacements, and reconstructing
// layout. Shared code (package herd) never looks inside a Layout value —
// it is opaque outside the backend that produced it.
package backend

import (
	"context"
	"encoding/json"
)

// RawWindow is one window as reported by a backend, before herd has
// classified it as zmx-backed or plain.
type RawWindow struct {
	ID      string
	Title   string
	Layout  Layout
	Focused bool
}

// Layout is a backend-specific, opaque position/arrangement descriptor.
// herd's shared code stores and replays it without interpreting it. It is
// a raw JSON value (not base64-encoded bytes) so snapshots stay readable.
type Layout = json.RawMessage

// PlannedWindow is one window as it should exist after a restore: either
// re-attached to a zmx session, or (if it had none) reopened blank.
type PlannedWindow struct {
	Kind    string `json:"kind"` // "zmx" or "plain"
	Session string `json:"session,omitempty"`
	Title   string `json:"title,omitempty"`
	Layout  Layout `json:"layout"`
}

// Reuse identifies an already-open window that should stand in for a
// snapshot's first window instead of Apply spawning a new one -- the
// window herd itself was invoked from. Window is that entry's saved
// content, carried along only so Apply can seed layout reconstruction
// for the windows spawned after it.
type Reuse struct {
	ID     string
	Window PlannedWindow
}

// Backend talks to one window manager. A snapshot records the name it was
// captured with (Name), and restore refuses to replay it on a different
// backend.
type Backend interface {
	Name() string

	// ListWindows returns the windows in the current focused
	// workspace/space, in the backend's natural order. Exactly one
	// entry has Focused set if any window in that workspace is
	// currently focused; Restore uses that to identify (and never
	// close) the window it was invoked from.
	ListWindows(ctx context.Context) ([]RawWindow, error)

	// Close asks the backend to close one window; Wait blocks until it is
	// confirmed gone, or returns an error (e.g. on timeout).
	Close(ctx context.Context, id string) error
	Wait(ctx context.Context, id string) error

	// Apply spawns and arranges spawn's windows in order. If reuse is
	// non-nil, its window is already open as reuse.ID (the one herd
	// itself was invoked from) and must not be spawned again -- it's
	// only used to seed layout reconstruction for spawn's entries.
	Apply(ctx context.Context, spawn []PlannedWindow, reuse *Reuse) error
}
