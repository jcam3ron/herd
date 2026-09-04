// Package run executes external commands on behalf of herd's backends and
// clients. It exists as a single indirection point (Runner) so tests can
// substitute a fake instead of shelling out to niri/zmx/ghostty.
package run

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Runner runs name with args and returns its stdout.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Exec is the real Runner, backed by os/exec.
func Exec(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out.Bytes(), nil
}
