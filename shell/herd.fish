# herd shell integration for fish.
#
# Source this file from your fish config (or copy its two functions into
# functions/fish_title.fish and functions/zmx.fish in nixos-config, since
# fish only autoloads functions from files named after them).
#
# It does two things herd relies on to tell a zmx-backed ghostty window
# apart from a plain one, without any process inspection (ghostty runs
# one process for all windows, so pid alone can't distinguish them):
#
#  1. Tags the ghostty window title "zmx:$ZMX_SESSION" while attached.
#     fish calls fish_title automatically before each prompt/title
#     update, so this is herd's fast path.
#  2. Labels the zmx session with the niri window id it was attached
#     from, right before the blocking attach call -- that window is
#     focused at the moment you type the command, so this is reliable.
#     herd falls back to this label when something else (claude, vim,
#     ssh, ...) has overwritten the title for as long as it runs.

function fish_title
    if set -q ZMX_SESSION
        echo "zmx:$ZMX_SESSION"
    else
        echo (status current-command) (prompt_pwd)
    end
end

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
