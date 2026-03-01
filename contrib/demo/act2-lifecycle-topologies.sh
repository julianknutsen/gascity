#!/usr/bin/env bash
# act2-lifecycle-topologies.sh — Act 2: Topology Comparison (Deterministic)
#
# Demonstrates: Same SDK, different orchestration shapes.
# Runs lifecycle (hierarchical: polecat branches + refinery merges) then
# swarm-lifecycle (flat: coders + merger) on local tmux, showing how the
# topology layer changes the orchestration pattern while the SDK stays same.
#
# All agents are bash scripts — no Claude API calls.
#
# Usage:
#   ./act2-lifecycle-topologies.sh
#
# Env vars:
#   ACT2_TIMEOUT   — auto-teardown seconds per topology (default: 60)
#   DEMO_CITY      — city directory (default: ~/demo-city)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GC_SRC="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=narrate.sh
source "$SCRIPT_DIR/narrate.sh"

QUICK_TIMEOUT="${ACT2_TIMEOUT:-60}"
DEMO_CITY="${DEMO_CITY:-$HOME/demo-city}"
DEMO_SESSION="gc-lifecycle"

# ── Helper: run one topology ───────────────────────────────────────────

run_topology_demo() {
    local EXAMPLE="$1"        # e.g., "examples/lifecycle"
    local TOPO_NAME="$2"      # e.g., "lifecycle" or "swarm-lifecycle"
    local POOL_LABEL="$3"     # e.g., "pool:polecat" or "pool:coder"

    # Discover topology dir name from the example.
    local RIG_TOPOLOGY
    RIG_TOPOLOGY=$(cd "$GC_SRC/$EXAMPLE" && ls -d topologies/*/ 2>/dev/null | head -1 | sed 's|/$||')
    RIG_TOPOLOGY="${RIG_TOPOLOGY:-topologies/$TOPO_NAME}"

    # ── Clean previous demo ────────────────────────────────────────────
    if [ -d "$DEMO_CITY" ]; then
        (cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
        rm -rf "$DEMO_CITY"
    fi
    tmux kill-session -t "$DEMO_SESSION" 2>/dev/null || true

    # ── Initialize city ────────────────────────────────────────────────
    step "Initializing city from $EXAMPLE..."
    gc init --from "$GC_SRC/$EXAMPLE" "$DEMO_CITY"

    # Create demo repo inside the city.
    DEMO_REPO="$DEMO_CITY/demo-repo"
    mkdir -p "$DEMO_REPO"
    (cd "$DEMO_REPO" && git init -q && echo "# Demo" > README.md && git add . && git commit -q -m "init")

    # Register rig.
    (cd "$DEMO_CITY" && gc rig add "$DEMO_REPO" --topology "$RIG_TOPOLOGY")
    step "City ready: demo-repo -> $RIG_TOPOLOGY"

    # ── Create 4-pane tmux layout ──────────────────────────────────────
    step "Creating demo tmux layout..."

    PANE_CTRL=$(tmux new-session -d -s "$DEMO_SESSION" -x 200 -y 50 -P -F "#{pane_id}")
    PANE_EVENTS=$(tmux split-window -h -t "$PANE_CTRL" -P -F "#{pane_id}")
    PANE_MAIL=$(tmux split-window -v -t "$PANE_CTRL" -P -F "#{pane_id}")
    PANE_PEEK=$(tmux split-window -v -t "$PANE_EVENTS" -P -F "#{pane_id}")

    tmux select-pane -t "$PANE_CTRL" -T "Controller ($TOPO_NAME)"
    tmux select-pane -t "$PANE_EVENTS" -T "Events"
    tmux select-pane -t "$PANE_MAIL" -T "Mail"
    tmux select-pane -t "$PANE_PEEK" -T "Peek"

    tmux set-option -t "$DEMO_SESSION" pane-border-status top
    tmux set-option -t "$DEMO_SESSION" pane-border-format "#{pane_title}"

    # Pane 2: Events stream.
    tmux send-keys -t "$PANE_EVENTS" \
        "cd $DEMO_CITY && gc events --follow" C-m

    # Pane 3: Mail traffic.
    tmux send-keys -t "$PANE_MAIL" \
        "cd $DEMO_CITY && watch -n2 'gc mail inbox 2>/dev/null || echo \"No mail yet\"'" C-m

    # Pane 4: Agent peek cycling.
    tmux send-keys -t "$PANE_PEEK" \
        "cd $DEMO_CITY && $SCRIPT_DIR/peek-cycle.sh" C-m

    # Pane 1: Controller (foreground).
    tmux send-keys -t "$PANE_CTRL" \
        "cd $DEMO_CITY && gc start --foreground" C-m

    step "Controller starting ($TOPO_NAME)..."

    # Wait for dolt/controller to stabilize.
    sleep 8

    # ── Seed beads ─────────────────────────────────────────────────────
    step "Seeding work beads (label: $POOL_LABEL)..."
    (cd "$DEMO_REPO" && bd create "Add authentication module" --labels "$POOL_LABEL" 2>/dev/null) || true
    (cd "$DEMO_REPO" && bd create "Fix parser edge case" --labels "$POOL_LABEL" 2>/dev/null) || true
    (cd "$DEMO_REPO" && bd create "Update API documentation" --labels "$POOL_LABEL" 2>/dev/null) || true
    step "3 beads seeded. Patrol will pick them up."

    # ── Auto-teardown timer ────────────────────────────────────────────
    (
        sleep "$QUICK_TIMEOUT"
        (cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
        tmux kill-session -t "$DEMO_SESSION" 2>/dev/null || true
    ) &
    local TEARDOWN_PID=$!
    trap 'kill $TEARDOWN_PID 2>/dev/null || true' EXIT

    # Wait for teardown.
    wait "$TEARDOWN_PID" 2>/dev/null || true
}

# ── Act 2 ─────────────────────────────────────────────────────────────────

narrate "Act 2: Topology Comparison" --sub "Same SDK, different orchestration shapes"
pause

# ── Lifecycle (Hierarchical) ────────────────────────────────────────────

narrate "Lifecycle Topology" --sub "Hierarchical: polecat pool + refinery (branch + merge)"

step "Topology shape:"
echo "  polecat pool (max 5, self-managed worktrees)"
echo "    -> each polecat: claim bead -> create branch -> commit"
echo "    -> hand off to refinery via bead metadata"
echo "  refinery (singleton, on main)"
echo "    -> poll for beads with branch metadata"
echo "    -> merge branch to main"
echo ""
step "Key feature: Branch isolation via worktrees, formal handoff to merge agent"
echo ""
pause "Press Enter to start lifecycle..."

run_topology_demo "examples/lifecycle" "lifecycle" "pool:polecat"

step "Lifecycle events:"
echo ""
countdown 5 "Showing events for"
(cd "$DEMO_CITY" && gc events --last 10 2>/dev/null) || true
echo ""

step "Lifecycle git log:"
echo ""
(cd "$DEMO_CITY/demo-repo" && git log --oneline --graph --all 2>/dev/null | head -15) || true
echo ""

narrate "Switching to swarm topology..." --sub "Flat peers, shared directory, no branches"
pause

# ── Swarm-Lifecycle (Flat Peer) ─────────────────────────────────────────

narrate "Swarm-Lifecycle Topology" --sub "Flat: coder pool (shared dir) + merger (git janitor)"

step "Topology shape:"
echo "  coder pool (max 5, shared directory)"
echo "    -> each coder: claim bead -> create file (NO git)"
echo "    -> mail merger when done"
echo "  merger (singleton, same directory)"
echo "    -> poll git status -> batch commit dirty files"
echo ""
step "Key feature: No branches, no worktrees — peers share one directory"
echo ""
pause "Press Enter to start swarm-lifecycle..."

run_topology_demo "examples/swarm-lifecycle" "swarm-lifecycle" "pool:coder"

step "Swarm events:"
echo ""
countdown 5 "Showing events for"
(cd "$DEMO_CITY" && gc events --last 10 2>/dev/null) || true
echo ""

step "Swarm git log (flat commits, no branches):"
echo ""
(cd "$DEMO_CITY/demo-repo" && git log --oneline 2>/dev/null | head -10) || true
echo ""

# ── Done ──────────────────────────────────────────────────────────────────

narrate "Act 2 Complete" --sub "Same primitives, different shapes — topology is configuration"
pause "Press Enter to continue to next act..."
