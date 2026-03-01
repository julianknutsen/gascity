#!/usr/bin/env bash
# wasteland-worker.sh — Claim a bead, run the wasteland inference, close it.
#
# Env vars (inherited from controller):
#   WL_BIN  — path to wl CLI (default: "wl")
set -euo pipefail

WL_BIN="${WL_BIN:-wl}"
POOL="polecat"

# 1. Claim a bead from the pool.
BEAD_JSON=$(bd list --label="pool:${POOL}" --status=open --limit=1 --json 2>/dev/null) || BEAD_JSON="[]"
BEAD_ID=$(echo "$BEAD_JSON" | jq -r '.[0].id // empty' 2>/dev/null) || true

if [[ -z "$BEAD_ID" ]]; then
    echo "wasteland-worker: no open beads in pool:${POOL}"
    exit 0
fi

BEAD_TITLE=$(echo "$BEAD_JSON" | jq -r '.[0].title // "untitled"' 2>/dev/null)
WL_ID=$(echo "$BEAD_JSON" | jq -r '.[0].metadata.wasteland_id // empty' 2>/dev/null) || true

echo "wasteland-worker: claiming bead ${BEAD_ID} — ${BEAD_TITLE}"
bd update "$BEAD_ID" --status=in_progress 2>/dev/null || true

# 2. Run the inference (if wl is available and we have a wasteland ID).
if [[ -n "$WL_ID" ]] && command -v "$WL_BIN" &>/dev/null; then
    echo "wasteland-worker: running inference for ${WL_ID}"
    if "$WL_BIN" infer run "$WL_ID" --skip-claim 2>/dev/null; then
        echo "wasteland-worker: inference completed for ${WL_ID}"
    else
        echo "wasteland-worker: inference failed for ${WL_ID}" >&2
    fi
else
    echo "wasteland-worker: processed bead ${BEAD_ID} (no wl available — simulated)"
    sleep 2
fi

# 3. Close the bead.
bd close "$BEAD_ID" 2>/dev/null || true
echo "wasteland-worker: closed bead ${BEAD_ID}"
