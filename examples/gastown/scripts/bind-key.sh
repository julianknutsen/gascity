#!/bin/sh
# bind-key.sh — idempotent tmux keybinding with fallback preservation.
# Usage: bind-key.sh <key> <gc-command> [guard-pattern]
#
# If the key already has a GC binding (if-shell + gc), does nothing.
# Otherwise captures the existing binding as fallback, then installs
# an if-shell binding that runs <gc-command> in GC sessions and the
# original binding in non-GC sessions.
set -e

key="$1"
gc_command="$2"
guard_pattern="${3:-^gc-}"

[ -z "$key" ] || [ -z "$gc_command" ] && exit 1

# Check if already a GC binding (idempotent).
existing=$(tmux list-keys -T prefix "$key" 2>/dev/null || true)
if printf '%s' "$existing" | grep -q 'if-shell' && printf '%s' "$existing" | grep -q 'gc '; then
    exit 0
fi

# Parse existing binding command as fallback.
# tmux list-keys format: bind-key [-r] -T <table> <key> <command> [args...]
fallback=""
if [ -n "$existing" ]; then
    # Skip past "-T prefix <key>" to get the command portion.
    # Handle optional -r flag.
    fallback=$(printf '%s' "$existing" | head -1 | awk '
    {
        i = 1
        # skip "bind-key"
        if ($i == "bind-key") i++
        # skip optional -r
        if ($i == "-r") i++
        # skip -T <table> <key>
        if ($i == "-T") i += 3
        # rest is the command
        cmd = ""
        for (; i <= NF; i++) cmd = cmd (cmd ? " " : "") $i
        print cmd
    }')
fi

# Default fallbacks for common keys.
if [ -z "$fallback" ]; then
    case "$key" in
        n) fallback="next-window" ;;
        p) fallback="previous-window" ;;
        *) fallback="command-prompt" ;;
    esac
fi

# Install the if-shell binding.
guard="echo '#{session_name}' | grep -Eq '${guard_pattern}'"
tmux bind-key -T prefix "$key" if-shell "$guard" "$gc_command" "$fallback"
