#!/bin/bash
# Bash agent: loop worker.
# Implements the same flow as prompts/loop.md using gc CLI commands.
# Continuously drains the backlog: check claim → claim from ready → close → repeat.
#
# Required env vars (set by gc start):
#   GC_AGENT — this agent's name
#   GC_CITY  — path to the city directory
#   PATH     — must include gc binary

set -euo pipefail
cd "$GC_CITY"

while true; do
    # Step 1: Check for already-claimed work
    hooked=$(gc agent claimed "$GC_AGENT" 2>/dev/null || true)

    if echo "$hooked" | grep -q "^ID:"; then
        # Step 5-6: Execute work and close the bead
        id=$(echo "$hooked" | grep "^ID:" | awk '{print $2}')
        gc bd close "$id"
        continue
    fi

    # Step 3: Check for available work in ready queue
    ready=$(gc bd ready 2>/dev/null || true)

    if echo "$ready" | grep -q "^gc-"; then
        # Step 4: Claim the first available bead
        id=$(echo "$ready" | grep "^gc-" | head -1 | awk '{print $1}')
        gc agent claim "$GC_AGENT" "$id" 2>/dev/null || true
        # Will process on next iteration (now claimed)
        continue
    fi

    # No work available — keep polling
    sleep 0.5
done
