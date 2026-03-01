#!/usr/bin/env bash
# mock-worker.sh — Deterministic worker for hyperscale demo.
#
# Claims one bead, closes it, exits. That's it.
# 100 of these run in parallel on K8s to demonstrate pool scaling.
#
# Required env vars (set by gc start):
#   GC_AGENT — this agent's name (e.g., "worker-42")
#   GC_DIR   — working directory

set -euo pipefail
cd "$GC_DIR"

# K8s: initialize beads client pointing at dolt server (if not already done).
DOLT_HOST="${GC_K8S_DOLT_HOST:-${DOLT_SERVICE_HOST:-}}"
if [ ! -d .beads ] && [ -n "$DOLT_HOST" ]; then
    BD_PREFIX=$(basename "$GC_DIR" | tr '-' '\n' | sed 's/^\(.\).*/\1/' | tr -d '\n')
    yes | bd init --server --server-host "$DOLT_HOST" \
      --server-port "${GC_K8S_DOLT_PORT:-${DOLT_SERVICE_PORT:-3307}}" \
      -p "$BD_PREFIX" --skip-hooks 2>/dev/null || true
fi

AGENT_SHORT=$(basename "$GC_AGENT")
echo "[$AGENT_SHORT] Starting up"

# Jitter to avoid 100 workers racing on the same bead.
JITTER=$(( RANDOM % 3 ))
sleep "$JITTER"

# ── Claim work ──────────────────────────────────────────────────────────

BEAD_ID=""
for attempt in $(seq 1 10); do
    ready=$(bd ready --label=pool:worker 2>/dev/null || true)
    if echo "$ready" | grep -qE '[a-z]{2}-[a-z0-9]'; then
        BEAD_ID=$(echo "$ready" | head -1 | awk '{print $2}')
        if bd update "$BEAD_ID" --claim --actor="$GC_AGENT" 2>/dev/null; then
            echo "[$AGENT_SHORT] Claimed: $BEAD_ID"
            break
        fi
        BEAD_ID=""
    fi
    sleep 1
done

if [ -z "$BEAD_ID" ]; then
    echo "[$AGENT_SHORT] No work found. Exiting."
    pkill -f "sleep infinity" 2>/dev/null || true
    exit 0
fi

# ── Do work (simulate) ─────────────────────────────────────────────────

sleep 1

# ── Close bead ──────────────────────────────────────────────────────────

bd close "$BEAD_ID" --reason "Hyperscale demo: completed by $AGENT_SHORT" 2>/dev/null || true
echo "[$AGENT_SHORT] Closed: $BEAD_ID. Done."

# Kill the "sleep infinity" keepalive so the K8s pod exits cleanly.
pkill -f "sleep infinity" 2>/dev/null || true
