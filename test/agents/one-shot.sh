#!/bin/bash
# Bash agent: one-shot worker.
# Implements the same flow as prompts/one-shot.md using gc CLI commands.
# Polls hook until work appears, processes one bead, then exits.
#
# Required env vars (set by gc start):
#   GC_AGENT — this agent's name
#   GC_CITY  — path to the city directory
#   PATH     — must include gc binary

set -euo pipefail
cd "$GC_CITY"

while true; do
    # Step 1: Check hook for assigned work
    hooked=$(gc bead hooked "$GC_AGENT" 2>/dev/null || true)

    if echo "$hooked" | grep -q "^ID:"; then
        # Step 2: Extract bead ID and close it (simulates executing the work)
        id=$(echo "$hooked" | grep "^ID:" | awk '{print $2}')
        gc bead close "$id"
        # Step 3: Done — one-shot agent exits after processing one bead
        exit 0
    fi

    sleep 0.5
done
