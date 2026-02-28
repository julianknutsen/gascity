#!/bin/sh
# worktree-setup.sh — idempotent git worktree creation for Gas City agents.
#
# Usage: worktree-setup.sh <repo-dir> <agent-name> <city-root> [--sync]
#
# Creates a git worktree at <city-root>/.gc/worktrees/<rig>/<agent-name>
# from the given repo directory. Idempotent: skips if worktree already exists.
# Optional --sync flag runs git fetch + pull --rebase after creation.
#
# Called from pre_start in topology configs. Runs before the session is created
# so the agent starts IN the worktree directory.

set -eu

REPO="${1:?usage: worktree-setup.sh <repo-dir> <agent-name> <city-root> [--sync]}"
AGENT="${2:?missing agent-name}"
CITY="${3:?missing city-root}"
RIG=$(basename "$REPO")
WT="$CITY/.gc/worktrees/$RIG/$AGENT"

# Idempotent: skip if worktree already exists.
if [ -d "$WT/.git" ] || [ -f "$WT/.git" ]; then
    [ "${4:-}" = "--sync" ] && { git -C "$WT" fetch origin 2>/dev/null; git -C "$WT" pull --rebase 2>/dev/null || true; }
    exit 0
fi

# MkdirAll may have created an empty dir — remove it for git worktree.
rmdir "$WT" 2>/dev/null || true
mkdir -p "$(dirname "$WT")"
git -C "$REPO" worktree add "$WT" -b "gc-$AGENT" || exit 0

# Bead redirect for filesystem beads.
mkdir -p "$WT/.beads"
echo "$REPO/.beads" > "$WT/.beads/redirect"

# Optional sync.
[ "${4:-}" = "--sync" ] && { git -C "$WT" fetch origin 2>/dev/null; git -C "$WT" pull --rebase 2>/dev/null || true; }

exit 0
