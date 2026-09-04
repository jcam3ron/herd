// Command herd snapshots and restores ghostty window layouts and zmx
// sessions for quick project switching.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/jcam3ron/herd/internal/backend/niri"
	"github.com/jcam3ron/herd/internal/ghostty"
	"github.com/jcam3ron/herd/internal/herd"
	"github.com/jcam3ron/herd/internal/snapshot"
	"github.com/jcam3ron/herd/internal/zmxclient"
)

const usageText = `herd snapshots and restores ghostty window layouts and zmx sessions.

Usage:
  herd <command> [arguments]

Commands:
  [s]ave <name>       snapshot the focused workspace's ghostty windows
  [r]estore <name>    close what's open, reopen the saved layout
  [sh]ow <name>       print a saved snapshot's contents
  [l]ist              list saved snapshots
  remove, rm <name>   delete a snapshot

Use "herd <command> -h" for details on a specific command.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(1)
	}
	if isHelp(os.Args[1]) {
		fmt.Fprint(os.Stdout, usageText)
		return
	}

	store, err := snapshot.NewStore()
	if err != nil {
		fatal(err)
	}
	app := &herd.App{
		Backend:     niri.New(),
		Zmx:         zmxclient.New(),
		Store:       store,
		Stdout:      os.Stdout,
		Confirm:     herd.ConfirmPrompt(os.Stdin, os.Stdout),
		SpawnWindow: func(cmd []string) error { return ghostty.Spawn("", cmd) },
	}
	ctx := context.Background()

	args := os.Args[2:]
	switch os.Args[1] {
	case "save", "s":
		name, force := parseNameForce("save", "snapshot the focused workspace's ghostty windows", args)
		fatalIf(app.Save(ctx, name, force))
	case "restore", "r":
		name, force := parseNameForce("restore", "close what's open, reopen the saved layout", args)
		fatalIf(app.Restore(ctx, name, force))
	case "restore-in-place":
		// Internal: what Restore relaunches into a new window to run.
		// Not listed in usageText or completions -- not meant to be
		// typed directly.
		name := parseName("restore-in-place", "perform a restore in the current window", args)
		fatalIf(app.RestoreInPlace(ctx, name))
	case "show", "sh":
		fatalIf(app.Show(parseName("show", "print a saved snapshot's contents", args)))
	case "list", "l":
		parseNoArgs("list", "list saved snapshots", args)
		fatalIf(app.List())
	case "remove", "rm":
		name, force := parseNameForce("remove", "delete a snapshot", args)
		fatalIf(app.Remove(name, force))
	default:
		fmt.Fprint(os.Stderr, usageText)
		os.Exit(1)
	}
}

func isHelp(s string) bool {
	return s == "-h" || s == "--help" || s == "help"
}

// newFlagSet builds a FlagSet whose -h/--help prints a one-line usage,
// the command's description, and (if given) additional flag help lines,
// instead of the flag package's default "Usage of save:" boilerplate.
func newFlagSet(cmd, argsUsage, description string, extraHelp ...string) *flag.FlagSet {
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	fs.Usage = func() {
		usage := cmd
		if argsUsage != "" {
			usage += " " + argsUsage
		}
		fmt.Fprintf(fs.Output(), "usage: herd %s\n\n  %s\n", usage, description)
		for _, line := range extraHelp {
			fmt.Fprintf(fs.Output(), "  %s\n", line)
		}
	}
	return fs
}

// parseName parses args for a subcommand that takes exactly one
// positional argument, exiting with usage on -h/--help or a wrong count.
func parseName(cmd, description string, args []string) string {
	fs := newFlagSet(cmd, "<name>", description)
	fs.Parse(args) //nolint:errcheck // ExitOnError already handles failures
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	return fs.Arg(0)
}

// parseNameForce is parseName plus a -f/--force flag that skips the
// command's confirmation prompt. Flags must precede the name (a stdlib
// flag.FlagSet limitation), hence "[-f|--force] <name>" in usage.
func parseNameForce(cmd, description string, args []string) (name string, force bool) {
	fs := newFlagSet(cmd, "[-f|--force] <name>", description, "-f, --force  skip the confirmation prompt")
	fs.BoolVar(&force, "force", false, "skip the confirmation prompt")
	fs.BoolVar(&force, "f", false, "skip the confirmation prompt (shorthand)")
	fs.Parse(args) //nolint:errcheck // ExitOnError already handles failures
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	return fs.Arg(0), force
}

// parseNoArgs parses args for a subcommand that takes no positional
// arguments, exiting with usage on -h/--help or an unexpected argument.
func parseNoArgs(cmd, description string, args []string) {
	fs := newFlagSet(cmd, "", description)
	fs.Parse(args) //nolint:errcheck // ExitOnError already handles failures
	if fs.NArg() != 0 {
		fs.Usage()
		os.Exit(1)
	}
}

func fatalIf(err error) {
	if err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
