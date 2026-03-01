#!/bin/sh
# agent-menu.sh — tmux popup menu for switching between GC agent sessions.
# Usage: agent-menu.sh <client-tty>
# Called via tmux run-shell from a keybinding (typically prefix-g).
# Always exits 0 — tmux must never see errors from run-shell.

client="$1"
[ -z "$client" ] && exit 0

# Collect GC sessions (sorted, one per line).
sessions=$(tmux list-sessions -F '#{session_name}' 2>/dev/null | grep '^gc-' | sort)
[ -z "$sessions" ] && exit 0

# Build tmux display-menu arguments.
# Each session gets a numbered shortcut (1-9, then a-z).
set -- "display-menu" "-T" "#[fg=cyan,bold]Gas City Agents" "-x" "C" "-y" "C"

i=0
for s in $sessions; do
    # Shortcut key: 1-9, then a-z.
    if [ "$i" -lt 9 ]; then
        key=$((i + 1))
    elif [ "$i" -lt 35 ]; then
        key=$(printf "\\$(printf '%03o' $((i - 9 + 97)))")
    else
        key=""
    fi

    # Display label: strip "gc-" prefix for readability.
    label=$(printf '%s' "$s" | sed 's/^gc-//')

    set -- "$@" "$label" "$key" "switch-client -c '$client' -t '$s'"
    i=$((i + 1))
done

tmux "$@"
