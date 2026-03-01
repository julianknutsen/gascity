#!/usr/bin/env bash
# run-demo.sh — "Three Stacks, One SDK" demo orchestration.
#
# Demonstrates Gas City's pluggable provider architecture by running the
# full Gas Town topology (8 roles) across 3 radically different infra
# combos — same city.toml, different env vars.
#
# Usage:
#   ./run-demo.sh <local|docker|k8s> [demo-repo-path]
#   ./run-demo.sh --quick <local|docker|k8s> [demo-repo-path]
#   ./run-demo.sh --topology examples/swarm <local|docker|k8s> [demo-repo-path]
#
# Flags:
#   --quick       Auto-dispatch one formula after startup, auto-teardown after
#                 QUICK_TIMEOUT seconds (default: 60).
#   --topology T  Use topology T instead of examples/gastown (e.g. examples/swarm).
#
# The demo-repo-path is a git repo the polecats will work on. If omitted,
# a small temp repo is created.
#
# Prerequisites:
#   local:  tmux, gc, bd/br
#   docker: docker, gc-agent:latest image (make docker-base docker-agent)
#   k8s:    kubectl, gc-agent + gc-controller images, gc namespace + dolt deployed
#
# Layout (4 tmux panes):
#   ┌──────────────────────┬──────────────────────┐
#   │ 1: Controller        │ 2: Events Stream     │
#   │ gc start --foreground│ gc events --watch     │
#   ├──────────────────────┼──────────────────────┤
#   │ 3: Mail Traffic      │ 4: Agent Peek        │
#   │ watch gc mail list   │ peek-cycle.sh         │
#   └──────────────────────┴──────────────────────┘
#
# K8s layout replaces pane 1 with controller pod deploy + log tail.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ── Parse flags ───────────────────────────────────────────────────────────

QUICK=false
QUICK_TIMEOUT="${QUICK_TIMEOUT:-60}"
TOPOLOGY=""

while [[ "${1:-}" == --* ]]; do
    case "$1" in
        --quick)
            QUICK=true
            shift
            ;;
        --topology)
            TOPOLOGY="${2:?--topology requires a value}"
            shift 2
            ;;
        *)
            echo "Unknown flag: $1" >&2
            exit 1
            ;;
    esac
done

COMBO="${1:?usage: run-demo.sh [--quick] [--topology T] <local|docker|k8s> [demo-repo-path]}"
DEMO_REPO="${2:-}"
DEMO_CITY="${DEMO_CITY:-$HOME/demo-city}"
DEMO_SESSION="gc-demo"
EXAMPLE="${TOPOLOGY:-examples/gastown}"

# Find the gascity source tree (where examples/ lives).
GC_SRC="$(cd "$SCRIPT_DIR/../.." && pwd)"

# K8s controller deploy script.
GC_CTRL_K8S="$GC_SRC/contrib/session-scripts/gc-controller-k8s"

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
            || die "gc-agent:latest image not found — build with: make docker-base docker-agent"
        ;;
    k8s)
        command -v kubectl >/dev/null 2>&1 || die "kubectl not found in PATH"
        kubectl get ns gc >/dev/null 2>&1 \
            || die "K8s namespace 'gc' not found — apply contrib/k8s/namespace.yaml first"
        docker image inspect gc-controller:latest >/dev/null 2>&1 \
            || die "gc-controller:latest image not found — build with: make docker-base docker-agent docker-controller"
        docker image inspect gc-mcp-mail:latest >/dev/null 2>&1 \
            || die "gc-mcp-mail:latest image not found — build with: docker build -f contrib/k8s/Dockerfile.mail -t gc-mcp-mail:latest ."
        # Verify controller RBAC exists.
        kubectl get serviceaccount gc-controller -n gc >/dev/null 2>&1 \
            || die "gc-controller ServiceAccount not found — apply contrib/k8s/controller-rbac.yaml first"
        ;;
esac

step "Preflight checks passed for '$COMBO' combo"

# ── Clean up previous demo ──────────────────────────────────────────────

banner "CLEANUP — removing previous demo"

if [ "$COMBO" = "k8s" ]; then
    # Stop the in-cluster controller pod.
    "$GC_CTRL_K8S" stop 2>/dev/null || true
fi

if [ -d "$DEMO_CITY" ]; then
    # Stop any running local controller first.
    if [ "$COMBO" != "k8s" ]; then
        (cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
    fi
    rm -rf "$DEMO_CITY"
    step "Removed $DEMO_CITY"
fi

# Kill any previous demo tmux session.
tmux kill-session -t "$DEMO_SESSION" 2>/dev/null || true

# ── Initialize city from gastown example ────────────────────────────────

banner "INIT — gc init --from $EXAMPLE"

gc init --from "$GC_SRC/$EXAMPLE" "$DEMO_CITY"
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

# Discover the primary topology dir name from the example.
RIG_TOPOLOGY=$(cd "$GC_SRC/$EXAMPLE" && ls -d topologies/*/ 2>/dev/null | head -1 | sed 's|/$||')
RIG_TOPOLOGY="${RIG_TOPOLOGY:-topologies/gastown}"

(cd "$DEMO_CITY" && gc rig add "$DEMO_REPO" --topology "$RIG_TOPOLOGY")
step "Rig registered: $(basename "$DEMO_REPO") -> $RIG_TOPOLOGY"

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
        # Monitoring env: local gc CLI uses these to reach the cluster.
        ENV_EXPORT="export GC_SESSION=k8s"
        ENV_EXPORT="$ENV_EXPORT; export GC_BEADS=exec:$GC_SRC/contrib/beads-scripts/gc-beads-k8s"
        ENV_EXPORT="$ENV_EXPORT; export GC_EVENTS=exec:$GC_SRC/contrib/events-scripts/gc-events-k8s"
        ENV_EXPORT="$ENV_EXPORT; export GC_K8S_IMAGE=gc-agent:latest"
        ENV_EXPORT="$ENV_EXPORT; export GC_K8S_NAMESPACE=gc"
        ENV_EXPORT="$ENV_EXPORT; export GC_DOLT_HOST=dolt.gc.svc.cluster.local"
        ENV_EXPORT="$ENV_EXPORT; export GC_DOLT_PORT=3307"
        step "Controller runs in-cluster (gc-controller pod)"
        step "GC_SESSION=k8s (native client-go provider)"
        step "GC_BEADS=exec:contrib/beads-scripts/gc-beads-k8s (monitoring)"
        step "GC_EVENTS=exec:contrib/events-scripts/gc-events-k8s (monitoring)"
        step "GC_K8S_IMAGE=gc-agent:latest"
        step "GC_K8S_NAMESPACE=gc"
        DASHBOARD_HINT="Open Lens and navigate to namespace 'gc' → Pods"
        ;;
esac

# ── Create tmux demo session with 4 panes ──────────────────────────────

banner "LAYOUT — building 4-pane tmux session"

cd "$DEMO_CITY"

# Create session with first pane (controller).
# Use pane IDs (-P -F) instead of index-based targets so the layout
# works regardless of tmux base-index / pane-base-index settings.
PANE_CTRL=$(tmux new-session -d -s "$DEMO_SESSION" -x 200 -y 50 -P -F "#{pane_id}")
PANE_EVENTS=$(tmux split-window -h -t "$PANE_CTRL" -P -F "#{pane_id}")
PANE_MAIL=$(tmux split-window -v -t "$PANE_CTRL" -P -F "#{pane_id}")
PANE_PEEK=$(tmux split-window -v -t "$PANE_EVENTS" -P -F "#{pane_id}")

# Label panes via pane titles.
if [ "$COMBO" = "k8s" ]; then
    tmux select-pane -t "$PANE_CTRL" -T "Controller (pod)"
else
    tmux select-pane -t "$PANE_CTRL" -T "Controller"
fi
tmux select-pane -t "$PANE_EVENTS" -T "Events"
tmux select-pane -t "$PANE_MAIL" -T "Mail"
tmux select-pane -t "$PANE_PEEK" -T "Peek"

# Enable pane border labels.
tmux set-option -t "$DEMO_SESSION" pane-border-status top
tmux set-option -t "$DEMO_SESSION" pane-border-format "#{pane_title}"

step "4-pane layout created in tmux session '$DEMO_SESSION'"

# ── Launch pane commands ────────────────────────────────────────────────

banner "LAUNCH — starting demo"

# Pane 2: Events stream (start first so it catches everything).
tmux send-keys -t "$PANE_EVENTS" \
    "cd $DEMO_CITY && $ENV_EXPORT; gc events --watch --timeout 3600" C-m

# Pane 3: Mail traffic (refreshes every 2s).
tmux send-keys -t "$PANE_MAIL" \
    "cd $DEMO_CITY && $ENV_EXPORT; watch -n2 'gc mail list --json 2>/dev/null | jq -r \".[] | \\\"[\\(.from) -> \\(.to)] \\(.subject)\\\"\" 2>/dev/null || echo \"No mail yet\"'" C-m

# Pane 4: Agent peek cycling.
tmux send-keys -t "$PANE_PEEK" \
    "cd $DEMO_CITY && $ENV_EXPORT; $SCRIPT_DIR/peek-cycle.sh" C-m

# ── Dashboard reminder ──────────────────────────────────────────────────

if [ "$QUICK" = true ]; then
    step "Quick mode: skipping dashboard pause"
else
    echo ""
    echo -e "${YELLOW}${BOLD}  DASHBOARD: $DASHBOARD_HINT${NC}"
    echo ""
    echo -e "  Press ${BOLD}Enter${NC} when your dashboard is positioned, then we'll start the controller."
    read -r
fi

# Pane 1: Controller — architecture depends on combo.
if [ "$COMBO" = "k8s" ]; then
    # Deploy mcp-agent-mail before starting controller so agents have mail.
    step "Deploying mcp-agent-mail service..."
    kubectl apply -f "$GC_SRC/contrib/k8s/mcp-mail-deployment.yaml"
    kubectl apply -f "$GC_SRC/contrib/k8s/mcp-mail-service.yaml"
    kubectl -n gc rollout status deployment/mcp-mail --timeout=60s
    step "mcp-agent-mail ready"

    # K8s: deploy controller pod, then tail its logs.
    tmux send-keys -t "$PANE_CTRL" \
        "export GC_K8S_NAMESPACE=gc; export GC_K8S_IMAGE=gc-agent:latest; $GC_CTRL_K8S deploy $DEMO_CITY && $GC_CTRL_K8S logs --follow" C-m
else
    # Local/Docker: run controller in foreground.
    tmux send-keys -t "$PANE_CTRL" \
        "cd $DEMO_CITY && $ENV_EXPORT; gc start --foreground" C-m
fi

step "Controller starting in pane 1"

# ── Quick mode: auto-dispatch + auto-teardown ────────────────────────────

if [ "$QUICK" = true ]; then
    step "Quick mode: waiting 10s for controller to stabilize..."
    sleep 10

    step "Quick mode: dispatching formula..."
    (cd "$DEMO_CITY" && gc sling polecat polecat-work --formula --nudge 2>/dev/null) || \
        warn "Quick mode: sling dispatch failed (agents may not be ready)"

    step "Quick mode: auto-teardown in ${QUICK_TIMEOUT}s"
    (
        sleep "$QUICK_TIMEOUT"
        if [ "$COMBO" = "k8s" ]; then
            "$GC_CTRL_K8S" stop 2>/dev/null || true
        else
            (cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
        fi
        tmux kill-session -t "$DEMO_SESSION" 2>/dev/null || true
    ) &
    TEARDOWN_PID=$!
    # Clean up teardown process if script exits early.
    trap 'kill $TEARDOWN_PID 2>/dev/null || true' EXIT
fi

# ── Attach to demo session ──────────────────────────────────────────────

banner "DEMO RUNNING — $COMBO combo"

if [ "$QUICK" = true ]; then
    step "Quick mode: running for ${QUICK_TIMEOUT}s then auto-teardown"
    # Wait for teardown to complete instead of attaching.
    wait "$TEARDOWN_PID" 2>/dev/null || true
    step "Quick mode: teardown complete"
else
    echo -e "  Attaching to tmux session '${BOLD}$DEMO_SESSION${NC}'..."
    echo ""

    if [ "$COMBO" = "k8s" ]; then
        echo -e "  ${CYAN}Pane 1${NC}: Controller logs (gc-controller pod)"
    else
        echo -e "  ${CYAN}Pane 1${NC}: Controller (gc start --foreground)"
    fi
    echo -e "  ${CYAN}Pane 2${NC}: Events stream (gc events --watch)"
    echo -e "  ${CYAN}Pane 3${NC}: Mail traffic (watch gc mail list)"
    echo -e "  ${CYAN}Pane 4${NC}: Agent peek cycle"
    echo ""
    echo -e "  ${YELLOW}To dispatch work:${NC}"
    echo -e "    gc sling polecat <formula> --formula --nudge"
    echo ""
    if [ "$COMBO" = "k8s" ]; then
        echo -e "  ${YELLOW}Detach:${NC} Ctrl-b d    ${YELLOW}Stop:${NC} $GC_CTRL_K8S stop"
    else
        echo -e "  ${YELLOW}Detach:${NC} Ctrl-b d    ${YELLOW}Stop:${NC} gc stop"
    fi
    echo ""

    tmux attach-session -t "$DEMO_SESSION"
fi
