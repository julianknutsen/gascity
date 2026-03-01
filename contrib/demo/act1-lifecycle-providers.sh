#!/usr/bin/env bash
# act1-lifecycle-providers.sh — Act 1: Provider Pluggability (Deterministic)
#
# Demonstrates: Same lifecycle topology, 3 infrastructure stacks.
# Runs the deterministic lifecycle topology (polecat pool + refinery)
# on local, docker, and k8s combos sequentially with narration.
#
# All agents are bash scripts — no Claude API calls.
#
# Usage:
#   ./act1-lifecycle-providers.sh
#
# Env vars:
#   ACT1_TIMEOUT   — auto-teardown seconds (default: 60)
#   DEMO_CITY      — city directory (default: ~/demo-city)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GC_SRC="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=narrate.sh
source "$SCRIPT_DIR/narrate.sh"

QUICK_TIMEOUT="${ACT1_TIMEOUT:-60}"
DEMO_CITY="${DEMO_CITY:-$HOME/demo-city}"
DEMO_SESSION="gc-lifecycle"

# ── Helper: run one provider combo ─────────────────────────────────────

run_lifecycle_combo() {
    local COMBO="$1"
    local ENV_EXPORT="${2:-}"

    # ── Clean previous demo ────────────────────────────────────────────
    if [ -d "$DEMO_CITY" ]; then
        (cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
        rm -rf "$DEMO_CITY"
    fi
    tmux kill-session -t "$DEMO_SESSION" 2>/dev/null || true

    # ── Initialize city ────────────────────────────────────────────────
    step "Initializing city from examples/lifecycle..."
    gc init --from "$GC_SRC/examples/lifecycle" "$DEMO_CITY"

    # Create demo repo inside the city (so Dir resolution works).
    DEMO_REPO="$DEMO_CITY/demo-repo"
    mkdir -p "$DEMO_REPO"
    (cd "$DEMO_REPO" && git init -q && echo "# Demo" > README.md && git add . && git commit -q -m "init")

    # Register rig with lifecycle topology.
    (cd "$DEMO_CITY" && gc rig add "$DEMO_REPO" --topology topologies/lifecycle)
    step "City ready: demo-repo -> topologies/lifecycle"

    # ── Create 4-pane tmux layout ──────────────────────────────────────
    step "Creating demo tmux layout..."

    PANE_CTRL=$(tmux new-session -d -s "$DEMO_SESSION" -x 200 -y 50 -P -F "#{pane_id}")
    PANE_EVENTS=$(tmux split-window -h -t "$PANE_CTRL" -P -F "#{pane_id}")
    PANE_MAIL=$(tmux split-window -v -t "$PANE_CTRL" -P -F "#{pane_id}")
    PANE_PEEK=$(tmux split-window -v -t "$PANE_EVENTS" -P -F "#{pane_id}")

    tmux select-pane -t "$PANE_CTRL" -T "Controller ($COMBO)"
    tmux select-pane -t "$PANE_EVENTS" -T "Events"
    tmux select-pane -t "$PANE_MAIL" -T "Mail"
    tmux select-pane -t "$PANE_PEEK" -T "Peek"

    tmux set-option -t "$DEMO_SESSION" pane-border-status top
    tmux set-option -t "$DEMO_SESSION" pane-border-format "#{pane_title}"

    # Pane 2: Events stream.
    tmux send-keys -t "$PANE_EVENTS" \
        "cd $DEMO_CITY && $ENV_EXPORT gc events --follow" C-m

    # Pane 3: Mail traffic.
    tmux send-keys -t "$PANE_MAIL" \
        "cd $DEMO_CITY && $ENV_EXPORT watch -n2 'gc mail inbox 2>/dev/null || echo \"No mail yet\"'" C-m

    # Pane 4: Agent peek cycling.
    tmux send-keys -t "$PANE_PEEK" \
        "cd $DEMO_CITY && $ENV_EXPORT $SCRIPT_DIR/peek-cycle.sh" C-m

    # Pane 1: Controller (foreground).
    tmux send-keys -t "$PANE_CTRL" \
        "cd $DEMO_CITY && $ENV_EXPORT gc start --foreground" C-m

    step "Controller starting ($COMBO)..."

    # Wait for dolt/controller to stabilize.
    sleep 8

    # ── Seed beads ─────────────────────────────────────────────────────
    step "Seeding work beads..."
    (cd "$DEMO_REPO" && bd create "Add authentication module" --labels pool:polecat 2>/dev/null) || true
    (cd "$DEMO_REPO" && bd create "Fix parser edge case" --labels pool:polecat 2>/dev/null) || true
    (cd "$DEMO_REPO" && bd create "Update API documentation" --labels pool:polecat 2>/dev/null) || true
    step "3 beads seeded (pool:polecat). Patrol will pick them up."

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
    step "$COMBO combo complete."
}

# ── Act 1 ─────────────────────────────────────────────────────────────────

narrate "Act 1: Provider Pluggability" --sub "Same lifecycle topology, three stacks"
pause

# ── Combo 1: Local ────────────────────────────────────────────────────────

narrate "Combo 1: Local" --sub "tmux sessions + bd (filesystem) + file JSONL events"

step "Provider env vars:"
echo "  GC_SESSION=tmux (default)"
echo "  GC_BEADS=bd (default)"
echo "  GC_EVENTS=file (default)"
echo ""
step "Topology: lifecycle (polecat pool + refinery, branch + merge)"
echo ""
pause "Press Enter to start local combo..."

run_lifecycle_combo "local" ""

# ── Show local event storage ──────────────────────────────────────────────

if [ -f "$DEMO_CITY/.gc/events.jsonl" ]; then
    step "Local event storage — .gc/events.jsonl:"
    echo ""
    tail -5 "$DEMO_CITY/.gc/events.jsonl" | jq . 2>/dev/null || tail -5 "$DEMO_CITY/.gc/events.jsonl"
    echo ""
fi

narrate "Switching from tmux to Docker..." --sub "Same lifecycle topology, different session provider"
pause

# ── Combo 2: Docker ───────────────────────────────────────────────────────

narrate "Combo 2: Docker" --sub "Docker containers + br (beads_rust) + file JSONL events"

step "Provider env vars:"
echo "  GC_SESSION=exec:scripts/gc-session-docker"
echo "  GC_BEADS=exec:contrib/beads-scripts/gc-beads-br"
echo "  GC_DOCKER_IMAGE=gc-lifecycle:latest"
echo ""
pause "Press Enter to start Docker combo..."

run_lifecycle_combo "docker" \
    "export GC_SESSION=exec:$GC_SRC/scripts/gc-session-docker; export GC_BEADS=exec:$GC_SRC/contrib/beads-scripts/gc-beads-br; export GC_DOCKER_IMAGE=gc-lifecycle:latest; "

step "Docker combo complete"
echo ""

narrate "Switching from Docker to Kubernetes..." --sub "Same lifecycle topology, native K8s provider"
pause

# ── Combo 3: Kubernetes ───────────────────────────────────────────────────

narrate "Combo 3: Kubernetes" --sub "K8s pods + dolt StatefulSet + ConfigMap events"

step "Provider env vars:"
echo "  GC_SESSION=k8s"
echo "  GC_BEADS=exec:contrib/beads-scripts/gc-beads-k8s"
echo "  GC_EVENTS=exec:contrib/events-scripts/gc-events-k8s"
echo "  GC_K8S_IMAGE=gc-lifecycle:latest"
echo "  GC_K8S_NAMESPACE=gc"
echo ""
pause "Press Enter to start K8s combo..."

# Ensure K8s namespace exists.
step "Creating namespace 'gc' (if not exists)..."
kubectl create namespace gc 2>/dev/null || true

run_lifecycle_combo "k8s" \
    "export GC_SESSION=k8s; export GC_BEADS=exec:$GC_SRC/contrib/beads-scripts/gc-beads-k8s; export GC_EVENTS=exec:$GC_SRC/contrib/events-scripts/gc-events-k8s; export GC_K8S_IMAGE=gc-lifecycle:latest; export GC_K8S_NAMESPACE=gc; "

# ── Show K8s event storage ────────────────────────────────────────────────

step "K8s event storage — ConfigMaps:"
echo ""
kubectl -n gc get configmaps -l gc/component=event --no-headers 2>/dev/null \
    | head -10 || echo "  (no event ConfigMaps found)"
echo ""

step "Same 'gc events --watch' command, different storage backends"
echo ""

# ── Done ──────────────────────────────────────────────────────────────────

narrate "Act 1 Complete" --sub "Three stacks, one SDK, identical lifecycle orchestration"
pause "Press Enter to continue to next act..."
