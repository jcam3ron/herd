#compdef herd
# Zsh completion for herd. Install via the Nix package (see flake.nix), or
# place this on your fpath as _herd.

_herd() {
    local -a commands
    commands=(
        "save:snapshot the focused workspace's ghostty windows"
        "s:snapshot the focused workspace's ghostty windows"
        "restore:close what's open, reopen the saved layout"
        "r:close what's open, reopen the saved layout"
        "show:print a saved snapshot's contents"
        "sh:print a saved snapshot's contents"
        'list:list saved snapshots'
        'l:list saved snapshots'
        'remove:delete a snapshot'
        'rm:delete a snapshot'
    )

    if (( CURRENT == 2 )); then
        _describe 'command' commands
        return
    fi

    local -a flags
    flags=('-f:skip the confirmation prompt' '--force:skip the confirmation prompt')

    case ${words[2]} in
        save|s)
            [[ ${words[CURRENT]} == -* ]] && _describe 'flag' flags
            ;;
        restore|r|remove|rm)
            if [[ ${words[CURRENT]} == -* ]]; then
                _describe 'flag' flags
            else
                local dir=${XDG_STATE_HOME:-$HOME/.local/state}/herd
                local -a names
                names=(${dir}/*.json(N:t:r))
                _describe 'snapshot' names
            fi
            ;;
        show|sh)
            local dir=${XDG_STATE_HOME:-$HOME/.local/state}/herd
            local -a names
            names=(${dir}/*.json(N:t:r))
            _describe 'snapshot' names
            ;;
    esac
}

_herd "$@"
