// Package ghostty spawns ghostty terminal windows. It is shared across
// window-manager backends since the terminal emulator itself isn't
// backend-specific.
package ghostty

import (
	"os"
	"os/exec"
	"slices"
	"strings"
)

// AppID is ghostty's app_id/bundle id, used by backends to filter windows
// belonging to it.
const AppID = "com.mitchellh.ghostty"

// Spawn starts a detached ghostty window and returns immediately; it does
// not wait for the window to appear or close. If cmd is non-empty,
// ghostty runs it (e.g. ["zmx", "attach", "myproject"]) instead of the
// default shell.
//
// Spawn is intentionally not tied to a context: ghostty runs single-
// instance, so this process just hands the new-window request to the
// existing instance and exits almost immediately: cancelling the caller's
// context must not tear that down mid-handoff.
func Spawn(workDir string, cmd []string) error {
	var args []string
	if workDir != "" {
		args = append(args, "--working-directory="+workDir)
	}
	if len(cmd) > 0 {
		args = append(args, "-e")
		args = append(args, cmd...)
	}
	c := exec.Command("ghostty", args...)
	c.Env = cleanEnv(os.Environ())
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait() //nolint:errcheck // fire-and-forget reap; the exit status of a detached window's launcher process isn't actionable
	return nil
}

// SpawnFallingBackToShell is Spawn, but wraps cmd so the window falls back
// to an interactive login shell once cmd exits, no matter why -- success,
// failure, or later once an exec'd process (e.g. `zmx attach`) itself
// exits. Without this, ghostty closes the window the instant cmd's
// foreground process exits.
func SpawnFallingBackToShell(workDir string, cmd []string) error {
	return Spawn(workDir, wrapWithShellFallback(cmd))
}

func wrapWithShellFallback(cmd []string) []string {
	return append([]string{"sh", "-c", `"$@"; exec "${SHELL:-/bin/sh}" -l`, "sh"}, cmd...)
}

// cleanEnv is env with ZMX_SESSION stripped. A spawned window is a
// genuinely new terminal, not a continuation of whatever spawned it, so
// it should never inherit which zmx session (if any) its spawner
// happened to be attached to.
func cleanEnv(env []string) []string {
	return slices.DeleteFunc(env, func(kv string) bool {
		return strings.HasPrefix(kv, "ZMX_SESSION=")
	})
}
