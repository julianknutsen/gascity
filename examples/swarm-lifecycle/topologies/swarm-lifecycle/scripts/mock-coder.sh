#!/usr/bin/env bash
# mock-coder.sh — Deterministic coder for swarm-lifecycle demo.
#
# Creates files in a shared directory (no git). Claims one bead, creates
# the file, notifies the merger via mail, closes the bead, and exits.
#
# Required env vars (set by gc start):
#   GC_AGENT — this agent's name (e.g., "demo-repo/coder-1")
#   GC_CITY  — path to the city directory
#   GC_DIR   — working directory (rig path)

set -euo pipefail
cd "$GC_DIR"

AGENT_SHORT=$(basename "$GC_AGENT")

echo "[$AGENT_SHORT] Starting up..."
# Jitter startup to avoid pool members racing on the same bead.
JITTER=$(( RANDOM % 3 ))
sleep "$JITTER"

# ── Step 1: Find + claim work ───────────────────────────────────────────

echo "[$AGENT_SHORT] Looking for work..."

BEAD_ID=""
BEAD_TITLE=""

for attempt in $(seq 1 30); do
    # Check if we already have claimed work (assigned to us, in_progress).
    claimed=$(bd list --assignee="$GC_AGENT" --status=in_progress --json 2>/dev/null || echo "[]")
    if echo "$claimed" | jq -e 'length > 0' >/dev/null 2>&1; then
        BEAD_ID=$(echo "$claimed" | jq -r '.[0].id')
        BEAD_TITLE=$(echo "$claimed" | jq -r '.[0].title')
        echo "[$AGENT_SHORT] Already have work: $BEAD_ID ($BEAD_TITLE)"
        break
    fi

    # Try to claim from the ready queue.
    # bd ready output: ○ dr-5bd ● P2 Title...  (bead ID is field 2)
    # Match on bead ID pattern (locale-independent, works in Docker).
    ready=$(bd ready --label=pool:coder 2>/dev/null || true)
    if echo "$ready" | grep -qE '[a-z]{2}-[a-z0-9]'; then
        BEAD_ID=$(echo "$ready" | head -1 | awk '{print $2}')
        # Atomic claim: sets assignee + status=in_progress, fails if taken.
        if bd update "$BEAD_ID" --claim --actor="$GC_AGENT" 2>/dev/null; then
            BEAD_TITLE=$(bd show "$BEAD_ID" --json 2>/dev/null | jq -r '.[0].title // "task"' || echo "task")
            echo "[$AGENT_SHORT] Claimed: $BEAD_ID ($BEAD_TITLE)"
            break
        fi
        BEAD_ID=""
    fi

    sleep 1
done

if [ -z "$BEAD_ID" ]; then
    echo "[$AGENT_SHORT] No work found after 30 attempts. Exiting."
    exit 0
fi

# ── Step 2: Notify merger ────────────────────────────────────────────────

gc mail send --all "WORKING: $BEAD_TITLE ($BEAD_ID)" 2>/dev/null || true
echo "[$AGENT_SHORT] Sent mail: WORKING on $BEAD_TITLE"

# ── Step 3: Create file (NO git operations) ──────────────────────────────

FILENAME="${BEAD_TITLE//[^a-zA-Z0-9_-]/_}.txt"
FILENAME=$(echo "$FILENAME" | tr '[:upper:]' '[:lower:]')

echo "[$AGENT_SHORT] Creating file: $FILENAME"
cat > "$FILENAME" <<EOF
# $BEAD_TITLE
#
# Created by $AGENT_SHORT
# Bead: $BEAD_ID
# Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)

Implementation of $BEAD_TITLE.
EOF

sleep 2  # Simulate work time

# ── Step 4: Close bead ───────────────────────────────────────────────────

echo "[$AGENT_SHORT] Closing bead: $BEAD_ID"
bd close "$BEAD_ID" 2>/dev/null || true

# ── Step 5: Notify done ─────────────────────────────────────────────────

gc mail send --all "DONE: $BEAD_TITLE — file $FILENAME ready" 2>/dev/null || true
echo "[$AGENT_SHORT] Done. File $FILENAME created. Exiting."
exit 0
