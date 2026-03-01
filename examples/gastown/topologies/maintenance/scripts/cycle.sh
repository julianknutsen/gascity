#!/bin/sh
# cycle.sh — cycle between Gas Town agent sessions in the same group.
# Usage: cycle.sh next|prev <current-session> <client-tty>
# Called via tmux run-shell from a keybinding.
#
# This is Gas Town-specific. It knows the role names and grouping rules:
#   Town group:    mayor ↔ deacon
#   Dog pool:      dog-1 ↔ dog-2 ↔ dog-3
#   Rig infra:     {rig}--witness ↔ {rig}--refinery  (per rig)
#   Polecat pool:  {rig}--polecat-1 ↔ {rig}--polecat-2  (per rig)
#   Crew:          {rig}--{name} members  (per rig, excluding witness/refinery/polecat)
#
# Session name format: gc-{city}-{agent}
#   Town-scoped:  gc-gastown-mayor, gc-gastown-deacon, gc-gastown-dog-1
#   Rig-scoped:   gc-gastown-myrig--witness, gc-gastown-myrig--polecat-1

direction="$1"
current="$2"
client="$3"

[ -z "$direction" ] || [ -z "$current" ] || [ -z "$client" ] && exit 0

# Determine the group filter pattern based on known Gas Town roles.
case "$current" in
    # Rig infra: witness ↔ refinery in same rig.
    *--witness|*--refinery)
        rig="${current%%--*}"
        pattern="^${rig}--\(witness\|refinery\)$"
        ;;
    # Polecat pool: cycle polecats in same rig.
    *--polecat-*)
        rig="${current%%--*}"
        pattern="^${rig}--polecat-"
        ;;
    # Other rig-scoped (crew, etc): cycle all same-rig non-infra agents.
    *--*)
        rig="${current%%--*}"
        pattern="^${rig}--"
        ;;
    # Town group: mayor ↔ deacon.
    *-mayor|*-deacon)
        city="${current%-mayor}"
        city="${city%-deacon}"
        pattern="^${city}-\(mayor\|deacon\)$"
        ;;
    # Dog pool: cycle between dog instances.
    *-dog-[0-9]*)
        prefix=$(printf '%s' "$current" | sed 's/dog-[0-9]*$/dog-/')
        pattern="^${prefix}[0-9]"
        ;;
    # Unknown — cycle all gc-* sessions as fallback.
    *)
        pattern="^gc-"
        ;;
esac

# Get target session: filter to same group, find current, pick next/prev.
target=$(tmux list-sessions -F '#{session_name}' 2>/dev/null \
    | grep "$pattern" \
    | sort \
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
