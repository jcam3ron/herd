# herd shell integration for zsh.
#
# Source this file from your zshrc:
#   source /path/to/herd/shell/herd.zsh
#
# Labels the zmx session with the niri window id it was attached from,
# right before the blocking attach call -- that window is focused at the
# moment you type the command, so this is reliable. This is how herd
# tells a zmx-backed ghostty window apart from a plain one.

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
