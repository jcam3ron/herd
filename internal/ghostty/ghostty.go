// Package ghostty spawns ghostty terminal windows. It is shared across
// window-manager backends since the terminal emulator itself isn't
// backend-specific.
package ghostty

import "os/exec"

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
	if err := c.Start(); err != nil {
		return err
	}
	go c.Wait()
	return nil
}
