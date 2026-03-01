#!/usr/bin/env bash
# mock-polecat.sh — Deterministic polecat for lifecycle demo.
#
# Claims a bead, creates a git worktree for branch isolation, creates a
# file, commits on the branch, then hands off to the refinery for merge.
#
# Required env vars (set by gc start):
#   GC_AGENT — this agent's name (e.g., "demo-repo/polecat-1")
#   GC_CITY  — path to the city directory
#   GC_DIR   — working directory (rig repo path)

set -euo pipefail

cd "$GC_DIR"

# Disable git credential prompts (K8s pods have no TTY for interactive input).
export GIT_TERMINAL_PROMPT=0

# Pull latest from origin (K8s: repo is baked in, pull gets updates).
git pull --ff-only origin main 2>/dev/null || true

# K8s: initialize beads client pointing at dolt server (if not already done).
# Uses K8s service discovery env vars injected by the dolt Service.
# Derive prefix from rig name same as gc-controller-k8s: split on hyphens,
# first letter of each part (e.g., "demo-repo" → "dr").
DOLT_HOST="${GC_K8S_DOLT_HOST:-${DOLT_SERVICE_HOST:-}}"
if [ ! -d .beads ] && [ -n "$DOLT_HOST" ]; then
    RIG_NAME=$(basename "$GC_DIR")
    BD_PREFIX=$(echo "$RIG_NAME" | tr '-' '\n' | sed 's/^\(.\).*/\1/' | tr -d '\n')
    yes | bd init --server --server-host "$DOLT_HOST" \
      --server-port "${GC_K8S_DOLT_PORT:-${DOLT_SERVICE_PORT:-3307}}" \
      -p "$BD_PREFIX" --skip-hooks 2>/dev/null || true
fi

AGENT_SHORT=$(basename "$GC_AGENT")

echo "[$AGENT_SHORT] Starting up in rig dir: $GC_DIR"
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
    ready=$(bd ready --label=pool:polecat 2>/dev/null || true)
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

# ── Step 2: Notify ────────────────────────────────────────────────────────

gc mail send --all "CLAIMED: $BEAD_TITLE ($BEAD_ID)" 2>/dev/null || true
echo "[$AGENT_SHORT] Sent mail: CLAIMED $BEAD_TITLE"

# ── Step 3: Create worktree + branch ─────────────────────────────────────

BRANCH="polecat/$BEAD_ID"
WT_DIR="${GC_WT_DIR:-/tmp/gc-wt}/$AGENT_SHORT"

echo "[$AGENT_SHORT] Creating worktree at $WT_DIR with branch $BRANCH"

mkdir -p "$(dirname "$WT_DIR")"
# Remove stale worktree dir if present (not a git worktree, just leftover dir).
if [ -d "$WT_DIR" ] && [ ! -f "$WT_DIR/.git" ]; then
    rm -rf "$WT_DIR"
fi

if ! git worktree add "$WT_DIR" -b "$BRANCH" HEAD 2>/dev/null; then
    # Branch might already exist from a previous run — try using it.
    git branch -D "$BRANCH" 2>/dev/null || true
    # Remove stale worktree entry.
    git worktree prune 2>/dev/null || true
    rm -rf "$WT_DIR" 2>/dev/null || true
    git worktree add "$WT_DIR" -b "$BRANCH" HEAD 2>/dev/null || {
        echo "[$AGENT_SHORT] Failed to create worktree. Exiting."
        exit 1
    }
fi

cd "$WT_DIR"

# ── Step 4: Create file + commit ─────────────────────────────────────────

FILENAME="${BEAD_TITLE//[^a-zA-Z0-9_-]/_}.txt"
FILENAME=$(echo "$FILENAME" | tr '[:upper:]' '[:lower:]')

echo "[$AGENT_SHORT] Creating file: $FILENAME"
cat > "$FILENAME" <<EOF
# $BEAD_TITLE
#
# Created by $AGENT_SHORT on branch $BRANCH
# Bead: $BEAD_ID
# Date: $(date -u +%Y-%m-%dT%H:%M:%SZ)

Implementation of $BEAD_TITLE.
EOF

sleep 2  # Simulate work time

git add "$FILENAME"
git commit -m "feat: $BEAD_TITLE ($BEAD_ID)" 2>/dev/null || true
echo "[$AGENT_SHORT] Committed on $BRANCH"

# ── Step 5: Push branch (if remote exists) ────────────────────────────────

if git remote | grep -q origin 2>/dev/null; then
    echo "[$AGENT_SHORT] Pushing $BRANCH to origin..."
    git push origin "$BRANCH" 2>/dev/null || true
fi

# ── Step 6: Clean up worktree (branch ref persists) ──────────────────────

cd "$GC_DIR"
git worktree remove "$WT_DIR" --force 2>/dev/null || true
echo "[$AGENT_SHORT] Worktree cleaned up. Branch $BRANCH persists."

# ── Step 7: Hand off to refinery ──────────────────────────────────────────

gc mail send --all "READY FOR MERGE: $BRANCH ($BEAD_TITLE)" 2>/dev/null || true

# Set metadata on the bead so the refinery knows which branch to merge.
bd update "$BEAD_ID" --metadata "{\"branch\":\"$BRANCH\"}" 2>/dev/null || true

echo "[$AGENT_SHORT] Handed off to refinery. Done."

# Kill the "sleep infinity" keepalive so the K8s pod exits cleanly.
# Use -P 1 to only target sleep processes that are children of PID 1
# (the container entrypoint). pkill -f would also match PID 1 itself
# since its cmdline contains "sleep infinity".
kill $(pgrep -P 1 -x sleep 2>/dev/null) 2>/dev/null || true
