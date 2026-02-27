#!/bin/bash
# Bash agent: loop worker with drain awareness.
# Like loop.sh but checks gc agent drain-check before each iteration.
# If drain-check returns 0 (draining), sends drain-ack and exits cleanly.
#
# Required env vars (set by gc start):
#   GC_AGENT — this agent's name
#   GC_CITY  — path to the city directory
#   PATH     — must include gc binary

set -euo pipefail
cd "$GC_CITY"

while true; do
    # Check if we're being drained
    if gc agent drain-check 2>/dev/null; then
        gc agent drain-ack 2>/dev/null || true
        exit 0
    fi

    # Check for already-claimed work
    hooked=$(gc agent claimed "$GC_AGENT" 2>/dev/null || true)

    if echo "$hooked" | grep -q "^ID:"; then
        id=$(echo "$hooked" | grep "^ID:" | awk '{print $2}')
        bd close "$id"
        continue
    fi

    # Check for available work in ready queue
    ready=$(bd ready 2>/dev/null || true)

    if echo "$ready" | grep -q "^gc-"; then
        id=$(echo "$ready" | grep "^gc-" | head -1 | awk '{print $1}')
        gc agent claim "$GC_AGENT" "$id" 2>/dev/null || true
        continue
    fi

    sleep 0.2
done
