#!/bin/sh
# cycle.sh — cycle between GC agent sessions in the same group.
# Usage: cycle.sh next|prev <current-session> <client-tty>
# Called via tmux run-shell from a keybinding.
#
# Grouping: sessions are grouped by their rig scope. Rig-scoped sessions
# (containing "--") cycle with others sharing the same rig prefix.
# Town-scoped sessions (no "--") cycle together.
#
# Examples (city "gastown"):
#   gc-gastown-mayor ↔ gc-gastown-deacon ↔ gc-gastown-dog-1  (town group)
#   gc-gastown-myrig--polecat-1 ↔ gc-gastown-myrig--polecat-2  (rig group)
#   gc-gastown-myrig--witness ↔ gc-gastown-myrig--refinery  (rig group)

direction="$1"
current="$2"
client="$3"

[ -z "$direction" ] || [ -z "$current" ] || [ -z "$client" ] && exit 0

# Build a grep pattern to select sessions in the same group.
case "$current" in
    *--*)
        # Rig-scoped: group = all sessions sharing the prefix before "--".
        prefix="${current%%--*}--"
        pattern="^${prefix}"
        ;;
    gc-*)
        # Town-scoped: group = all gc-* sessions without "--" (same city).
        pattern="^gc-"
        filter_rig="yes"
        ;;
    *)
        exit 0
        ;;
esac

# Get target session: filter to same group, find current, pick next/prev.
sessions=$(tmux list-sessions -F '#{session_name}' 2>/dev/null | grep "$pattern" | sort)

# For town-scoped: exclude rig-scoped sessions (those containing "--").
if [ "$filter_rig" = "yes" ]; then
    sessions=$(printf '%s\n' "$sessions" | grep -v -- '--')
fi

target=$(printf '%s\n' "$sessions" \
    | awk -v cur="$current" -v dir="$direction" '
        { a[NR] = $0; if ($0 == cur) idx = NR }
        END {
            if (NR <= 1 || idx == 0) exit
            if (dir == "next") t = (idx % NR) + 1
            else t = ((idx - 2 + NR) % NR) + 1
            print a[t]
        }')

[ -z "$target" ] && exit 0
tmux switch-client -c "$client" -t "$target"
