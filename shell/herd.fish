# herd shell integration for fish.
#
# Source this file from your fish config (or copy its function into
# functions/zmx.fish in nixos-config, since fish only autoloads functions
# from files named after them).
#
# Labels the zmx session with the niri window id it was attached from,
# right before the blocking attach call -- that window is focused at the
# moment you type the command, so this is reliable. This is how herd
# tells a zmx-backed ghostty window apart from a plain one.

function zmx --wraps zmx
    if test (count $argv) -ge 2
        and contains -- $argv[1] a attach
        set -l winid (niri msg -j focused-window 2>/dev/null | jq -r '.id // empty')
        if test -n "$winid"
            command zmx set $argv[2] last_window=$winid >/dev/null 2>&1
        end
    end
    command zmx $argv
end
