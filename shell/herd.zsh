# herd shell integration for zsh.
#
# Source this file from your zshrc:
#   source /path/to/herd/shell/herd.zsh
#
# It does two things herd relies on to tell a zmx-backed ghostty window
# apart from a plain one, without any process inspection (ghostty runs
# one process for all windows, so pid alone can't distinguish them):
#
#  1. Tags the ghostty window title "zmx:$ZMX_SESSION" while attached, via
#     the OSC 0 escape sequence. Zsh has no fish_title equivalent that
#     fires automatically, so this hooks precmd_functions to run before
#     each prompt instead.
#  2. Labels the zmx session with the niri window id it was attached
#     from, right before the blocking attach call -- that window is
#     focused at the moment you type the command, so this is reliable.
#     herd falls back to this label when something else (claude, vim,
#     ssh, ...) has overwritten the title for as long as it runs.

__herd_set_title() {
    if [[ -n "$ZMX_SESSION" ]]; then
        printf '\033]0;zmx:%s\007' "$ZMX_SESSION"
    fi
}
precmd_functions+=(__herd_set_title)

zmx() {
    if [[ $# -ge 2 && ("$1" == a || "$1" == attach) ]]; then
        local winid
        winid=$(niri msg -j focused-window 2>/dev/null | jq -r '.id // empty')
        if [[ -n "$winid" ]]; then
            command zmx set "$2" "last_window=$winid" >/dev/null 2>&1
        fi
    fi
    command zmx "$@"
}
