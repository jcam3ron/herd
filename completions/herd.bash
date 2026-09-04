# Bash completion for herd.
#
# Requires bash-completion (for _init_completion). Install via the Nix
# package (see flake.nix) or source directly and register with:
#   complete -F _herd herd

_herd_snapshot_names() {
    local dir="${XDG_STATE_HOME:-$HOME/.local/state}/herd" f base
    for f in "$dir"/*.json; do
        [[ -e "$f" ]] || continue
        base="${f##*/}"
        printf '%s\n' "${base%.json}"
    done
}

_herd() {
    # shellcheck disable=SC2034  # prev is unused here but _init_completion expects it declared local
    local cur prev words cword
    _init_completion || return

    if [[ $cword -eq 1 ]]; then
        # shellcheck disable=SC2207  # standard bash-completion compgen idiom
        COMPREPLY=($(compgen -W "save s restore r show sh list l remove rm" -- "$cur"))
        return
    fi

    case "${words[1]}" in
        save|s)
            if [[ "$cur" == -* ]]; then
                # shellcheck disable=SC2207  # standard bash-completion compgen idiom
                COMPREPLY=($(compgen -W "-f --force" -- "$cur"))
            fi
            ;;
        restore|r|remove|rm)
            if [[ "$cur" == -* ]]; then
                # shellcheck disable=SC2207  # standard bash-completion compgen idiom
                COMPREPLY=($(compgen -W "-f --force" -- "$cur"))
            else
                # shellcheck disable=SC2207  # standard bash-completion compgen idiom
                COMPREPLY=($(compgen -W "$(_herd_snapshot_names)" -- "$cur"))
            fi
            ;;
        show|sh)
            # shellcheck disable=SC2207  # standard bash-completion compgen idiom
            COMPREPLY=($(compgen -W "$(_herd_snapshot_names)" -- "$cur"))
            ;;
    esac
}
complete -F _herd herd
