// Package zmxclient wraps the parts of the zmx CLI herd needs: listing
// sessions and reading the last_window label the fish `zmx` wrapper sets
// at attach time (see fish/zmx-wrapper.fish).
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

// LastWindowLabels returns every session's last_window label as a map
// from window id to session name. This is the fallback path for windows
// whose title no longer says "zmx:<name>" because something else
// (claude, vim, ssh) overwrote it: the fish `zmx` wrapper sets this label
// at attach time (see fish/zmx-wrapper.fish).
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
