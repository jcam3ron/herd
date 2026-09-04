# herd

Snapshot and restore ghostty window layouts and zmx sessions for quick project
switching - no tmux required.

Written in Go, with window-manager logic behind a `Backend` interface
(`internal/backend`). Only niri (Linux) is implemented today; omniwm (Mac) is a
future goal, not yet designed.

This project was built with the assistance of Claude.

## Requires

- [niri](https://github.com/YaLTeR/niri), [ghostty](https://ghostty.org),
  [zmx](https://github.com/neurosnap/zmx) on `PATH`
- fish, bash, or zsh, with `shell/herd.fish`, `shell/herd.bash`, or
  `shell/herd.zsh` sourced from your config. Each defines one `zmx` wrapper
  function: when you run `zmx attach <session>`, it labels that session with the
  niri window id it was attached from (right before the blocking attach call,
  while that window is still focused) so herd can tell a zmx-backed window apart
  from a plain one later.
- for windows herd itself spawns during `restore` (not attached manually), it
  labels them itself, so the shell integration above is only needed for sessions
  you `zmx attach` by hand.
- ghostty config: `confirm-close-surface = false` - without it, closing a window
  during restore pops a confirmation dialog `herd` can't click through, and
  restore will hang

## Build / install

```
nix build .#herd          # x86_64-linux or aarch64-darwin
go build ./cmd/herd        # or plain go, for local iteration
```

## Usage

```
herd save <name>      # snapshot the focused workspace's ghostty windows (s)
herd restore <name>   # close what's open, reopen the saved layout (r)
herd show <name>      # print a saved snapshot's contents (sh)
herd list             # list saved snapshots (l)
herd remove <name>    # delete a snapshot (rm)
```

Snapshots are stored as JSON under `$XDG_STATE_HOME/herd` (default
`~/.local/state/herd`) and record which backend they were captured with;
`restore` refuses to replay a snapshot on a different backend.

`restore` never does its work in the window it was invoked from - it always
opens a new window and relaunches itself there, which becomes the saved layout's
first window: if that slot was zmx-backed, `herd` execs itself into `zmx attach`
for it (replacing the herd process in place, no extra wrapper left running); if
it was plain, the window is already sitting at a blank shell prompt, which is
exactly what a plain slot means.

`save` warns and asks for confirmation before overwriting an existing snapshot;
`restore` warns and asks before closing a plain (non-zmx) window that would lose
its content; `remove` warns and asks before permanently deleting a snapshot. All
three accept `-f`/`--force` to skip the prompt: `herd save -f <name>`,
`herd restore -f <name>`, `herd remove -f <name>`.

## How it works

A zmx-backed window is detected by a `last_window=<window-id>` label on the zmx
session - set by herd itself right after spawning or reattaching a window during
`restore` (see `zmxclient.Client.SetLastWindow`), or by the shell's `zmx`
wrapper for a session you attach to by hand. herd never reads or sets the window
title, so it works regardless of your prompt or title customization. This
classification is backend-agnostic (`internal/herd`); everything after "here are
the windows and where they go" - closing, spawning, and reconstructing layout -
is owned by the active `Backend` (`internal/backend/niri` today).

Restoring closes the current workspace's windows - for zmx-backed ones this just
detaches the client, since the session persists in the zmx daemon - then
respawns windows in the saved order and asks the backend to reconstruct their
arrangement (for niri: column/row stacking via `consume-window-into-column`).

## Development

```
nix develop            # go + gopls
go test ./...
```
