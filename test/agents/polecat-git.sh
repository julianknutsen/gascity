#!/bin/bash
# Bash agent: polecat git work lifecycle.
# Deterministic polecat that exercises the full git pipeline:
# poll for work via bd ready → create branch → commit fix → push → hand off
# to refinery via bd update --assignee → exit.
#
# Required env vars (set by gc start):
#   GC_AGENT — this agent's name
#   GC_CITY  — path to the city directory
#   GC_DIR   — path to the rig's repo (working copy)
#   GIT_WORK_DIR — override for git repo path (optional, defaults to GC_DIR)
#   GC_HANDOFF_TO — agent name to hand off to (optional, defaults to "refinery")
#   PATH     — must include gc, bd, and jq binaries

set -euo pipefail
cd "$GC_CITY"

REPO_DIR="${GIT_WORK_DIR:-$GC_DIR}"

while true; do
    # Check for work assigned to this agent
    work_id=$(bd ready --assignee="$GC_AGENT" --json 2>/dev/null \
        | jq -r '.[0].id // empty' 2>/dev/null || true)

    if [ -n "$work_id" ]; then
        # Step 1: Create feature branch in working copy
        cd "$REPO_DIR"
        branch="gc/${GC_AGENT}/${work_id}"
        git checkout -b "$branch" 2>/dev/null || git checkout "$branch" 2>/dev/null || true

        # Step 2: Make a change and commit
        echo "fix for $work_id" > "fix-${work_id}.txt"
        git add -A
        git commit -m "fix: $work_id" 2>/dev/null || true

        # Step 3: Push branch to origin
        git push origin "$branch" 2>/dev/null || true

        cd "$GC_CITY"

        # Step 4: Hand off to refinery by reassigning the bead
        bd update "$work_id" --assignee="${GC_HANDOFF_TO:-refinery}" 2>/dev/null || true

        exit 0
    fi

    sleep 0.2
done
