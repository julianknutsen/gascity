#!/usr/bin/env bash
# run-demo.sh — "Three Stacks, One SDK" demo orchestration.
#
# Demonstrates Gas City's pluggable provider architecture by running the
# full Gas Town topology (8 roles) across 3 radically different infra
# combos — same city.toml, different env vars.
#
# Usage:
#   ./run-demo.sh <local|docker|k8s> [demo-repo-path]
#
# The demo-repo-path is a git repo the polecats will work on. If omitted,
# a small temp repo is created.
#
# Prerequisites:
#   local:  tmux, gc, bd/br
#   docker: docker, gc-agent:latest image (see contrib/k8s/Dockerfile.agent)
#   k8s:    kubectl, Lens recommended, gc namespace + dolt StatefulSet deployed
#
# Layout (4 tmux panes):
#   ┌──────────────────────┬──────────────────────┐
#   │ 1: Controller        │ 2: Events Stream     │
#   │ gc start --foreground│ gc events --watch     │
#   ├──────────────────────┼──────────────────────┤
#   │ 3: Convoy Status     │ 4: Agent Peek        │
#   │ watch gc convoy list │ peek-cycle.sh         │
#   └──────────────────────┴──────────────────────┘

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
COMBO="${1:?usage: run-demo.sh <local|docker|k8s> [demo-repo-path]}"
DEMO_REPO="${2:-}"
DEMO_CITY="${DEMO_CITY:-$HOME/demo-city}"
DEMO_SESSION="gc-demo"

# ── Colors ──────────────────────────────────────────────────────────────

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

banner() {
    echo ""
    echo -e "${BLUE}${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}${BOLD}  $1${NC}"
    echo -e "${BLUE}${BOLD}═══════════════════════════════════════════════════════${NC}"
    echo ""
}

step() {
    echo -e "${GREEN}▶${NC} ${BOLD}$1${NC}"
}

warn() {
    echo -e "${YELLOW}⚠${NC} $1"
}

die() {
    echo -e "${RED}✗${NC} $1" >&2
    exit 1
}

# ── Validate combo ──────────────────────────────────────────────────────

case "$COMBO" in
    local|docker|k8s) ;;
    *) die "Unknown combo: $COMBO (expected: local, docker, k8s)" ;;
esac

# ── Preflight checks ───────────────────────────────────────────────────

banner "PREFLIGHT — $COMBO combo"

command -v gc >/dev/null 2>&1 || die "gc not found in PATH"
command -v tmux >/dev/null 2>&1 || die "tmux not found in PATH"

case "$COMBO" in
    docker)
        command -v docker >/dev/null 2>&1 || die "docker not found in PATH"
        docker image inspect gc-agent:latest >/dev/null 2>&1 \
            || die "gc-agent:latest image not found — build with: docker build -f contrib/k8s/Dockerfile.agent -t gc-agent:latest ."
        ;;
    k8s)
        command -v kubectl >/dev/null 2>&1 || die "kubectl not found in PATH"
        kubectl get ns gc >/dev/null 2>&1 \
            || die "K8s namespace 'gc' not found — apply contrib/k8s/namespace.yaml first"
        ;;
esac

step "Preflight checks passed for '$COMBO' combo"

# ── Clean up previous demo ──────────────────────────────────────────────

if [ -d "$DEMO_CITY" ]; then
    banner "CLEANUP — removing previous demo city"
    # Stop any running controller first.
    (cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
    rm -rf "$DEMO_CITY"
    step "Removed $DEMO_CITY"
fi

# Kill any previous demo tmux session.
tmux kill-session -t "$DEMO_SESSION" 2>/dev/null || true

# ── Initialize city from gastown example ────────────────────────────────

banner "INIT — gc init --from examples/gastown"

# Find the gascity source tree (where examples/ lives).
GC_SRC="$(cd "$SCRIPT_DIR/../.." && pwd)"

gc init --from "$GC_SRC/examples/gastown" "$DEMO_CITY"
step "City initialized at $DEMO_CITY"

# ── Set up demo rig ─────────────────────────────────────────────────────

banner "RIG — setting up demo repository"

if [ -n "$DEMO_REPO" ]; then
    step "Using provided repo: $DEMO_REPO"
else
    DEMO_REPO="$DEMO_CITY/demo-repo"
    mkdir -p "$DEMO_REPO"
    (cd "$DEMO_REPO" && git init -q && echo "# Demo" > README.md && git add . && git commit -q -m "init")
    step "Created temp demo repo at $DEMO_REPO"
fi

(cd "$DEMO_CITY" && gc rig add "$DEMO_REPO" --name demo-repo --topology topologies/gastown)
step "Rig registered: demo-repo → topologies/gastown"

# ── Set env vars for combo ──────────────────────────────────────────────

banner "COMBO — configuring $COMBO providers"

ENV_EXPORT=""
DASHBOARD_HINT=""

case "$COMBO" in
    local)
        step "Using defaults: tmux sessions, bd (dolt) beads, file JSONL events"
        DASHBOARD_HINT="Run 'tmux list-sessions' in another terminal"
        ;;
    docker)
        ENV_EXPORT="export GC_SESSION=exec:$GC_SRC/scripts/gc-session-docker"
        ENV_EXPORT="$ENV_EXPORT; export GC_BEADS=exec:$GC_SRC/contrib/beads-scripts/gc-beads-br"
        ENV_EXPORT="$ENV_EXPORT; export GC_DOCKER_IMAGE=gc-agent:latest"
        step "GC_SESSION=exec:scripts/gc-session-docker"
        step "GC_BEADS=exec:contrib/beads-scripts/gc-beads-br"
        step "GC_DOCKER_IMAGE=gc-agent:latest"
        DASHBOARD_HINT="Open lazydocker or Portainer (localhost:9000) on second monitor"
        ;;
    k8s)
        ENV_EXPORT="export GC_SESSION=exec:$GC_SRC/contrib/session-scripts/gc-session-k8s"
        ENV_EXPORT="$ENV_EXPORT; export GC_K8S_IMAGE=gc-agent:latest"
        ENV_EXPORT="$ENV_EXPORT; export GC_K8S_NAMESPACE=gc"
        ENV_EXPORT="$ENV_EXPORT; export GC_DOLT_HOST=dolt.gc.svc.cluster.local"
        ENV_EXPORT="$ENV_EXPORT; export GC_DOLT_PORT=3307"
        step "GC_SESSION=exec:contrib/session-scripts/gc-session-k8s"
        step "GC_K8S_IMAGE=gc-agent:latest"
        step "GC_K8S_NAMESPACE=gc"
        DASHBOARD_HINT="Open Lens and navigate to namespace 'gc' → Pods"
        ;;
esac

# ── Create tmux demo session with 4 panes ──────────────────────────────

banner "LAYOUT — building 4-pane tmux session"

cd "$DEMO_CITY"

# Create session with first pane (controller).
tmux new-session -d -s "$DEMO_SESSION" -x 200 -y 50

# Split into 4 panes (2x2 grid).
tmux split-window -h -t "$DEMO_SESSION"
tmux split-window -v -t "${DEMO_SESSION}:0.0"
tmux split-window -v -t "${DEMO_SESSION}:0.1"

# Label panes via pane titles.
tmux select-pane -t "${DEMO_SESSION}:0.0" -T "Controller"
tmux select-pane -t "${DEMO_SESSION}:0.1" -T "Events"
tmux select-pane -t "${DEMO_SESSION}:0.2" -T "Convoy"
tmux select-pane -t "${DEMO_SESSION}:0.3" -T "Peek"

# Enable pane border labels.
tmux set-option -t "$DEMO_SESSION" pane-border-status top
tmux set-option -t "$DEMO_SESSION" pane-border-format "#{pane_title}"

step "4-pane layout created in tmux session '$DEMO_SESSION'"

# ── Launch pane commands ────────────────────────────────────────────────

banner "LAUNCH — starting demo"

# Pane 2: Events stream (start first so it catches everything).
tmux send-keys -t "${DEMO_SESSION}:0.1" \
    "cd $DEMO_CITY && $ENV_EXPORT; gc events --watch --timeout 3600" C-m

# Pane 3: Convoy status (refreshes every 2s).
tmux send-keys -t "${DEMO_SESSION}:0.2" \
    "cd $DEMO_CITY && watch -n2 gc convoy list 2>/dev/null || echo 'No convoys yet'" C-m

# Pane 4: Agent peek cycling.
tmux send-keys -t "${DEMO_SESSION}:0.3" \
    "cd $DEMO_CITY && $SCRIPT_DIR/peek-cycle.sh" C-m

# ── Dashboard reminder ──────────────────────────────────────────────────

echo ""
echo -e "${YELLOW}${BOLD}  DASHBOARD: $DASHBOARD_HINT${NC}"
echo ""
echo -e "  Press ${BOLD}Enter${NC} when your dashboard is positioned, then we'll start the controller."
read -r

# Pane 1: Controller (foreground — the main event).
tmux send-keys -t "${DEMO_SESSION}:0.0" \
    "cd $DEMO_CITY && $ENV_EXPORT; gc start --foreground" C-m

step "Controller starting in pane 1"

# ── Attach to demo session ──────────────────────────────────────────────

banner "DEMO RUNNING — $COMBO combo"

echo -e "  Attaching to tmux session '${BOLD}$DEMO_SESSION${NC}'..."
echo ""
echo -e "  ${CYAN}Pane 1${NC}: Controller (gc start --foreground)"
echo -e "  ${CYAN}Pane 2${NC}: Events stream (gc events --watch)"
echo -e "  ${CYAN}Pane 3${NC}: Convoy status (watch gc convoy list)"
echo -e "  ${CYAN}Pane 4${NC}: Agent peek cycle"
echo ""
echo -e "  ${YELLOW}To dispatch work:${NC}"
echo -e "    gc sling polecat <formula> --formula --nudge"
echo ""
echo -e "  ${YELLOW}Detach:${NC} Ctrl-b d    ${YELLOW}Stop:${NC} gc stop"
echo ""

tmux attach-session -t "$DEMO_SESSION"
