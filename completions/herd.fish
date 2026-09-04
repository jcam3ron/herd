# Fish completion for herd. Install via the Nix package (see flake.nix),
# or place this under a fish completions directory as herd.fish.

function __herd_snapshot_names
    set -l dir (set -q XDG_STATE_HOME; and echo $XDG_STATE_HOME; or echo $HOME/.local/state)/herd
    for f in $dir/*.json
        path basename (path change-extension '' $f)
    end
end

complete -c herd -f
complete -c herd -n __fish_use_subcommand -a save -d "snapshot the focused workspace's ghostty windows"
complete -c herd -n __fish_use_subcommand -a s -d "snapshot the focused workspace's ghostty windows"
complete -c herd -n __fish_use_subcommand -a restore -d "close what's open, reopen the saved layout"
complete -c herd -n __fish_use_subcommand -a r -d "close what's open, reopen the saved layout"
complete -c herd -n __fish_use_subcommand -a show -d "print a saved snapshot's contents"
complete -c herd -n __fish_use_subcommand -a sh -d "print a saved snapshot's contents"
complete -c herd -n __fish_use_subcommand -a list -d 'list saved snapshots'
complete -c herd -n __fish_use_subcommand -a l -d 'list saved snapshots'
complete -c herd -n __fish_use_subcommand -a remove -d 'delete a snapshot'
complete -c herd -n __fish_use_subcommand -a rm -d 'delete a snapshot'
complete -c herd -n '__fish_seen_subcommand_from restore r show sh remove rm' -a '(__herd_snapshot_names)'
complete -c herd -n '__fish_seen_subcommand_from save s restore r remove rm' -s f -l force -d 'skip the confirmation prompt'
