#!/bin/sh
# cycle.sh — cycle between running GC agent sessions in tmux.
# Usage: cycle.sh next|prev <current-session> <client-tty>
# Called via tmux run-shell from a keybinding.

direction="$1"
current="$2"
client="$3"

[ -z "$direction" ] || [ -z "$current" ] || [ -z "$client" ] && exit 0

# Get target session via awk: find current, pick next/prev with wraparound.
target=$(tmux list-sessions -F '#{session_name}' 2>/dev/null \
    | grep '^gc-' \
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
