#!/usr/bin/env bash
# mock-merger.sh — Deterministic git janitor for swarm-lifecycle demo.
#
# Polls git status in a loop. When dirty, stages all changes, commits,
# and pushes. Notifies coders via mail. Runs until killed by controller.
#
# Required env vars (set by gc start):
#   GC_AGENT — this agent's name (e.g., "demo-repo/merger")
#   GC_CITY  — path to the city directory
#   GC_DIR   — working directory (rig path)

set -euo pipefail
cd "$GC_DIR"

AGENT_SHORT=$(basename "$GC_AGENT")
POLL_INTERVAL="${GC_MERGER_POLL:-5}"

echo "[$AGENT_SHORT] Starting git janitor (poll every ${POLL_INTERVAL}s)..."

while true; do
    # Check for dirty state.
    DIRTY=$(git status --porcelain 2>/dev/null || true)

    if [ -n "$DIRTY" ]; then
        # Count new/modified files.
        FILE_COUNT=$(echo "$DIRTY" | wc -l | tr -d ' ')
        FILE_LIST=$(echo "$DIRTY" | awk '{print $2}' | head -5 | tr '\n' ', ' | sed 's/,$//')

        echo "[$AGENT_SHORT] Dirty state detected: $FILE_COUNT file(s)"

        # Notify coders.
        gc mail send --all "COMMITTING: $FILE_COUNT file(s): $FILE_LIST" 2>/dev/null || true

        # Stage all changes.
        git add -A

        # Commit.
        COMMIT_MSG="swarm: batch commit $FILE_COUNT file(s)"
        echo "[$AGENT_SHORT] Committing: $COMMIT_MSG"
        git commit -m "$COMMIT_MSG" 2>/dev/null || true

        # Push (best-effort — may not have remote).
        if git remote | grep -q origin 2>/dev/null; then
            BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null || echo "main")
            echo "[$AGENT_SHORT] Pushing to origin/$BRANCH..."
            git push origin "$BRANCH" 2>/dev/null || true
            gc mail send --all "PUSHED: batch on $BRANCH ($FILE_COUNT files)" 2>/dev/null || true
        else
            gc mail send --all "COMMITTED: $FILE_COUNT file(s) (no remote)" 2>/dev/null || true
        fi
    else
        echo "[$AGENT_SHORT] Clean. Sleeping ${POLL_INTERVAL}s..."
    fi

    sleep "$POLL_INTERVAL"
done
