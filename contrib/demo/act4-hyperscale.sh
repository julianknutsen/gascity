#!/usr/bin/env bash
# act4-hyperscale.sh — Act 4: 100-Agent Hyperscale
#
# Demonstrates: 100 agents, one controller, zero per-agent overhead.
# Spawns 100 worker pods on K8s. Visual: a wall of pods materializing,
# events streaming, work completing.
#
# Usage:
#   ./act4-hyperscale.sh [count]
#
# Args:
#   count — number of workers to spawn (default: 100)
#
# Env vars:
#   GC_HYPERSCALE_MOCK  — if "true", uses shell mock instead of Claude
#                         sessions (avoids API costs)
#   ACT4_TIMEOUT        — auto-teardown seconds (default: 300)
#   GC_K8S_NAMESPACE    — K8s namespace (default: gc)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GC_SRC="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=narrate.sh
source "$SCRIPT_DIR/narrate.sh"

COUNT="${1:-100}"
DEMO_CITY="${DEMO_CITY:-$HOME/hyperscale-demo}"
DEMO_SESSION="gc-hyperscale"
ACT4_TIMEOUT="${ACT4_TIMEOUT:-300}"
GC_K8S_NAMESPACE="${GC_K8S_NAMESPACE:-gc}"
GC_HYPERSCALE_MOCK="${GC_HYPERSCALE_MOCK:-false}"

# ── Preflight ─────────────────────────────────────────────────────────────

command -v gc >/dev/null 2>&1 || { echo "ERROR: gc not found in PATH" >&2; exit 1; }
command -v kubectl >/dev/null 2>&1 || { echo "ERROR: kubectl not found in PATH" >&2; exit 1; }
command -v tmux >/dev/null 2>&1 || { echo "ERROR: tmux not found in PATH" >&2; exit 1; }

kubectl get ns "$GC_K8S_NAMESPACE" >/dev/null 2>&1 || {
    echo "ERROR: K8s namespace '$GC_K8S_NAMESPACE' not found" >&2
    exit 1
}

# ── Act 4 ─────────────────────────────────────────────────────────────────

narrate "Act 4: 100-Agent Hyperscale" --sub "$COUNT agents, one controller, zero per-agent overhead"
pause

# ── Clean previous demo ──────────────────────────────────────────────────

if [ -d "$DEMO_CITY" ]; then
    (cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
    rm -rf "$DEMO_CITY"
fi
tmux kill-session -t "$DEMO_SESSION" 2>/dev/null || true

# ── Build prebaked image ──────────────────────────────────────────────────

step "Building prebaked image: gc-hyperscale:latest"
gc build-image "$GC_SRC/examples/hyperscale" --tag gc-hyperscale:latest
echo ""

# ── Initialize city ──────────────────────────────────────────────────────

step "Initializing hyperscale city..."
gc init --from "$GC_SRC/examples/hyperscale" "$DEMO_CITY"
step "City initialized at $DEMO_CITY"
echo ""

# ── Seed beads ────────────────────────────────────────────────────────────

step "Seeding $COUNT work beads..."
"$SCRIPT_DIR/seed-hyperscale.sh" "$COUNT" "$DEMO_CITY"
echo ""

# ── Mock mode ─────────────────────────────────────────────────────────────

if [ "$GC_HYPERSCALE_MOCK" = "true" ]; then
    step "Mock mode: workers will use shell commands instead of Claude sessions"
    step "  Each pod runs: bd ready -> bd close -> exit"
    echo ""
fi

pause "Press Enter to start the hyperscale demo..."

# ── Create 3-pane tmux layout ────────────────────────────────────────────
#
# ┌──────────────────────┬──────────────────────┐
# │ Controller logs      │ Pod watch             │
# │                      │ kubectl get pods -w   │
# ├──────────────────────┴──────────────────────┤
# │ Progress: ./progress.sh worker 100           │
# └──────────────────────────────────────────────┘

step "Creating 3-pane tmux layout..."

PANE_CTRL=$(tmux new-session -d -s "$DEMO_SESSION" -x 200 -y 50 -P -F "#{pane_id}")
PANE_PODS=$(tmux split-window -h -t "$PANE_CTRL" -P -F "#{pane_id}")
PANE_PROGRESS=$(tmux split-window -v -t "$PANE_CTRL" -l 5 -P -F "#{pane_id}")

tmux select-pane -t "$PANE_CTRL" -T "Controller"
tmux select-pane -t "$PANE_PODS" -T "Pods"
tmux select-pane -t "$PANE_PROGRESS" -T "Progress"

tmux set-option -t "$DEMO_SESSION" pane-border-status top
tmux set-option -t "$DEMO_SESSION" pane-border-format "#{pane_title}"

cd "$DEMO_CITY"

# Pane 2 (top-right): Pod watch.
tmux send-keys -t "$PANE_PODS" \
    "kubectl -n $GC_K8S_NAMESPACE get pods -w -l gc/component=agent 2>/dev/null || kubectl -n $GC_K8S_NAMESPACE get pods -w" C-m

# Pane 3 (bottom): Progress counter.
tmux send-keys -t "$PANE_PROGRESS" \
    "cd $DEMO_CITY && $SCRIPT_DIR/progress.sh worker $COUNT" C-m

# Pane 1 (top-left): Controller foreground.
tmux send-keys -t "$PANE_CTRL" \
    "cd $DEMO_CITY && GC_K8S_IMAGE=gc-hyperscale:latest GC_K8S_NAMESPACE=$GC_K8S_NAMESPACE gc start --foreground" C-m

step "Controller starting — watch pods materialize!"

# ── Auto-teardown timer ──────────────────────────────────────────────────

(
    sleep "$ACT4_TIMEOUT"
    (cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
    tmux kill-session -t "$DEMO_SESSION" 2>/dev/null || true
) &
TEARDOWN_PID=$!
trap 'kill $TEARDOWN_PID 2>/dev/null || true' EXIT

# ── Attach ────────────────────────────────────────────────────────────────

echo ""
step "Layout:"
echo "  Top-left:  Controller logs"
echo "  Top-right: kubectl get pods -w ($COUNT pods materializing)"
echo "  Bottom:    Progress counter (0/$COUNT -> $COUNT/$COUNT)"
echo ""
echo "  Auto-teardown in ${ACT4_TIMEOUT}s"
echo ""

tmux attach-session -t "$DEMO_SESSION"

# ── Post-run stats ────────────────────────────────────────────────────────

echo ""
step "Post-run stats:"
echo ""
echo "  K8s event ConfigMaps:"
event_count=$(kubectl -n "$GC_K8S_NAMESPACE" get configmaps -l gc/component=event --no-headers 2>/dev/null | wc -l || echo 0)
echo "  $event_count event ConfigMaps generated"
echo ""

# ── Done ──────────────────────────────────────────────────────────────────

narrate "Act 4 Complete" --sub "$COUNT pods spawned, work completed, pool drained"
pause "Press Enter to finish..."
