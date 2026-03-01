#!/usr/bin/env bash
# act1-providers.sh — Act 1: Provider Pluggability
#
# Demonstrates: Same topology, 3 infrastructure stacks.
# Runs gastown on local, docker, and k8s combos sequentially with
# narration pauses between each.
#
# Usage:
#   ./act1-providers.sh
#
# Each combo: start -> dispatch one formula -> show events -> auto-teardown.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=narrate.sh
source "$SCRIPT_DIR/narrate.sh"

QUICK_TIMEOUT="${ACT1_TIMEOUT:-60}"

# ── Act 1 ─────────────────────────────────────────────────────────────────

narrate "Act 1: Provider Pluggability" --sub "Same topology, three stacks"
pause

# ── Combo 1: Local ────────────────────────────────────────────────────────

narrate "Combo 1: Local" --sub "tmux sessions + bd (dolt) + file JSONL events"

step "Provider env vars:"
echo "  GC_SESSION=tmux (default)"
echo "  GC_BEADS=bd (default)"
echo "  GC_EVENTS=file (default)"
echo ""
pause "Press Enter to start local combo..."

QUICK_TIMEOUT="$QUICK_TIMEOUT" "$SCRIPT_DIR/run-demo.sh" --quick local

step "Local combo complete"
echo ""

# ── Show local event storage ──────────────────────────────────────────────

DEMO_CITY="${DEMO_CITY:-$HOME/demo-city}"
if [ -f "$DEMO_CITY/.gc/events.jsonl" ]; then
    step "Local event storage — .gc/events.jsonl:"
    echo ""
    tail -5 "$DEMO_CITY/.gc/events.jsonl" | jq . 2>/dev/null || tail -5 "$DEMO_CITY/.gc/events.jsonl"
    echo ""
fi

narrate "Switching from tmux to Docker..." --sub "Same city.toml, different session provider"
pause

# ── Combo 2: Docker ───────────────────────────────────────────────────────

narrate "Combo 2: Docker" --sub "Docker containers + br (beads_rust) + file JSONL events"

step "Provider env vars:"
echo "  GC_SESSION=exec:scripts/gc-session-docker"
echo "  GC_BEADS=exec:contrib/beads-scripts/gc-beads-br"
echo "  GC_DOCKER_IMAGE=gc-agent:latest"
echo ""
pause "Press Enter to start Docker combo..."

QUICK_TIMEOUT="$QUICK_TIMEOUT" "$SCRIPT_DIR/run-demo.sh" --quick docker

step "Docker combo complete"
echo ""

narrate "Switching from Docker to Kubernetes..." --sub "Same city.toml, native K8s provider"
pause

# ── Combo 3: Kubernetes ───────────────────────────────────────────────────

narrate "Combo 3: Kubernetes" --sub "K8s pods + dolt StatefulSet + ConfigMap events"

step "Provider env vars:"
echo "  GC_SESSION=k8s"
echo "  GC_BEADS=exec:contrib/beads-scripts/gc-beads-k8s"
echo "  GC_EVENTS=exec:contrib/events-scripts/gc-events-k8s"
echo "  GC_K8S_IMAGE=gc-agent:latest"
echo "  GC_K8S_NAMESPACE=gc"
echo ""
pause "Press Enter to start K8s combo..."

QUICK_TIMEOUT="$QUICK_TIMEOUT" "$SCRIPT_DIR/run-demo.sh" --quick k8s

# ── Show K8s event storage ────────────────────────────────────────────────

step "K8s event storage — ConfigMaps:"
echo ""
kubectl -n gc get configmaps -l gc/component=event --no-headers 2>/dev/null \
    | head -10 || echo "  (no event ConfigMaps found)"
echo ""

step "Same 'gc events --watch' command, different storage backends"
echo ""

# ── Done ──────────────────────────────────────────────────────────────────

narrate "Act 1 Complete" --sub "Three stacks, one SDK, identical orchestration"
pause "Press Enter to continue to next act..."
