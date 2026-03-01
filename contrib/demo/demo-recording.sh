#!/usr/bin/env bash
# demo-recording.sh — Top-level orchestrator for the 4-act demo recording.
#
# Sequences acts with banner screens and narration pause points.
# Each act is independently runnable. This script provides the full
# recording flow with cleanup between acts.
#
# Usage:
#   ./demo-recording.sh [act1|act2|act3|act4|all]
#
# Examples:
#   ./demo-recording.sh all    # Full 4-act recording
#   ./demo-recording.sh act2   # Just topology comparison
#   ./demo-recording.sh act4   # Just hyperscale demo

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=narrate.sh
source "$SCRIPT_DIR/narrate.sh"

ACT="${1:-all}"

# ── Cleanup helper ────────────────────────────────────────────────────────

cleanup_between_acts() {
    # Kill any lingering demo tmux sessions.
    tmux kill-session -t gc-demo 2>/dev/null || true
    tmux kill-session -t gc-hyperscale 2>/dev/null || true

    # Stop any running city.
    local demo_city="${DEMO_CITY:-$HOME/demo-city}"
    if [ -d "$demo_city" ]; then
        (cd "$demo_city" && gc stop 2>/dev/null) || true
    fi
}

# ── Act runners ───────────────────────────────────────────────────────────

run_act1() {
    "$SCRIPT_DIR/act1-providers.sh"
    cleanup_between_acts
}

run_act2() {
    "$SCRIPT_DIR/act2-topologies.sh"
    cleanup_between_acts
}

run_act3() {
    "$SCRIPT_DIR/act3-wasteland.sh"
    cleanup_between_acts
}

run_act4() {
    "$SCRIPT_DIR/act4-hyperscale.sh"
    cleanup_between_acts
}

# ── Main ──────────────────────────────────────────────────────────────────

case "$ACT" in
    act1)
        run_act1
        ;;
    act2)
        run_act2
        ;;
    act3)
        run_act3
        ;;
    act4)
        run_act4
        ;;
    all)
        narrate "Gas City Demo Recording" --sub "Four acts, one SDK"
        echo "  Act 1: Provider Pluggability — same topology, 3 stacks"
        echo "  Act 2: Topology Comparison  — gastown vs swarm"
        echo "  Act 3: Wasteland Auto-Claim — external work intake"
        echo "  Act 4: 100-Agent Hyperscale — K8s pod storm"
        echo ""
        pause "Press Enter to begin recording..."

        # ── Act 1 ──
        run_act1

        narrate "Act 1 done" --sub "Next: Topology Comparison"
        pause

        # ── Act 2 ──
        run_act2

        narrate "Act 2 done" --sub "Next: Wasteland Auto-Claim"
        pause

        # ── Act 3 ──
        run_act3

        narrate "Act 3 done" --sub "Next: 100-Agent Hyperscale"
        pause

        # ── Act 4 ──
        run_act4

        # ── Finale ──
        narrate "Demo Recording Complete" --sub "Gas City: orchestration as configuration"
        echo "  Four capabilities demonstrated:"
        echo "    1. Provider pluggability — tmux, Docker, K8s"
        echo "    2. Topology comparison  — hierarchical vs flat peer"
        echo "    3. External work intake  — Wasteland federation"
        echo "    4. Hyperscale            — 100 agents, one controller"
        echo ""
        ;;
    *)
        echo "Usage: demo-recording.sh [act1|act2|act3|act4|all]" >&2
        exit 1
        ;;
esac
