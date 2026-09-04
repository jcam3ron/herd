// Package zmxclient wraps the parts of the zmx CLI herd needs: listing
// sessions and reading and writing the last_window label that ties a
// window to a zmx session.
//
// This package is the sole source of truth for that label: herd never
// reads or sets a window's title to figure out which zmx session (if
// any) it belongs to. herd itself writes the label for windows it spawns
// (see herd.App.RestoreInPlace); the shell `zmx` wrapper writes it for
// sessions attached to manually (see shell/herd.fish).
package zmxclient

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/jcam3ron/herd/internal/run"
)

type Client struct {
	Run run.Runner
	// Attach replaces the calling process with `zmx attach <session>`.
	// On success it never returns.
	Attach func(session string) error
}

func New() *Client {
	return &Client{Run: run.Exec, Attach: execAttach}
}

// execAttach is Client's real Attach: it hands the calling terminal over
// to the zmx session by replacing the current process image (like a
// shell's `exec`), not by forking a child -- so nothing about this
// process (including the pty herd was invoked from) needs to survive.
func execAttach(session string) error {
	path, err := exec.LookPath("zmx")
	if err != nil {
		return err
	}
	return syscall.Exec(path, []string{"zmx", "attach", session}, os.Environ())
}

// nonEmptyLines splits s on newlines, trims each line, and drops blanks.
func nonEmptyLines(s string) []string {
	var lines []string
	for _, line := range strings.Split(s, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// Sessions lists known zmx session names.
func (c *Client) Sessions(ctx context.Context) ([]string, error) {
	out, err := c.Run(ctx, "zmx", "list", "--short")
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(string(out)), nil
}

// SetLastWindow records windowID as session's last_window label -- the
// same bookkeeping the shell `zmx` wrapper does for a manual attach (see
// shell/herd.fish).
func (c *Client) SetLastWindow(ctx context.Context, session, windowID string) error {
	_, err := c.Run(ctx, "zmx", "set", session, "last_window="+windowID)
	return err
}

// LastWindowLabels returns every session's last_window label as a map
// from window id to session name.
func (c *Client) LastWindowLabels(ctx context.Context) (map[string]string, error) {
	sessions, err := c.Sessions(ctx)
	if err != nil {
		return nil, err
	}
	labels := make(map[string]string, len(sessions))
	for _, session := range sessions {
		out, err := c.Run(ctx, "zmx", "get", session)
		if err != nil {
			continue // session may have vanished between list and get
		}
		for _, line := range nonEmptyLines(string(out)) {
			if id, ok := strings.CutPrefix(line, "last_window="); ok {
				labels[id] = session
			}
		}
	}
	return labels, nil
}
