#!/usr/bin/env bash
# act2-topologies.sh — Act 2: Topology Comparison
#
# Demonstrates: Same SDK, different orchestration shapes.
# Runs gastown (hierarchical) then swarm (flat peer) on local tmux,
# showing how the topology layer changes the orchestration pattern
# while the SDK primitives stay the same.
#
# Usage:
#   ./act2-topologies.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=narrate.sh
source "$SCRIPT_DIR/narrate.sh"

QUICK_TIMEOUT="${ACT2_TIMEOUT:-60}"
DEMO_CITY="${DEMO_CITY:-$HOME/demo-city}"

# ── Act 2 ─────────────────────────────────────────────────────────────────

narrate "Act 2: Topology Comparison" --sub "Same SDK, different orchestration shapes"
pause

# ── Gastown (Hierarchical) ────────────────────────────────────────────────

narrate "Gastown Topology" --sub "Hierarchical: mayor -> deacon -> polecat pool"

step "Topology shape:"
echo "  mayor (coordinator)"
echo "    -> deacon (health patrol)"
echo "    -> boot (bootstrapper)"
echo "    -> polecat pool (workers, worktree-isolated)"
echo "    -> refinery (code review)"
echo "    -> witness (commit/merge)"
echo ""
step "Key feature: Formula-based dispatch via gc sling"
echo ""
pause "Press Enter to start gastown..."

QUICK_TIMEOUT="$QUICK_TIMEOUT" "$SCRIPT_DIR/run-demo.sh" --quick local

step "Dispatching formula to polecat pool..."
(cd "$DEMO_CITY" && gc sling polecat polecat-work --formula --nudge 2>/dev/null) || true

step "Hierarchical dispatch chain visible in events"
echo ""
countdown 5 "Showing events for"

# Show recent events.
(cd "$DEMO_CITY" && gc events --last 10 2>/dev/null) || true
echo ""

step "Tearing down gastown..."
(cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
tmux kill-session -t gc-demo 2>/dev/null || true

narrate "Switching to swarm topology..." --sub "Flat peers, no formulas, no worktree isolation"
pause

# ── Swarm (Flat Peer) ─────────────────────────────────────────────────────

narrate "Swarm Topology" --sub "Flat: coder pool (self-organizing peers) + committer"

step "Topology shape:"
echo "  mayor (coordinator)"
echo "    -> deacon (health patrol)"
echo "    -> dog pool (infrastructure)"
echo "    -> coder pool (flat peers, self-organizing)"
echo "    -> committer (dedicated git ops)"
echo ""
step "Key feature: Peer coordination via beads + mail (no formulas)"
echo ""
pause "Press Enter to start swarm..."

QUICK_TIMEOUT="$QUICK_TIMEOUT" "$SCRIPT_DIR/run-demo.sh" --quick --topology examples/swarm local

step "Creating work for coder pool..."
(cd "$DEMO_CITY" && bd create "Review README" --type task --label pool:coder 2>/dev/null) || true
(cd "$DEMO_CITY" && bd create "Check test coverage" --type task --label pool:coder 2>/dev/null) || true

step "Peer coordination visible in events"
echo ""
countdown 5 "Showing events for"

# Show recent events.
(cd "$DEMO_CITY" && gc events --last 10 2>/dev/null) || true
echo ""

step "Tearing down swarm..."
(cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
tmux kill-session -t gc-demo 2>/dev/null || true

# ── Done ──────────────────────────────────────────────────────────────────

narrate "Act 2 Complete" --sub "Same primitives, different shapes — topology is configuration"
pause "Press Enter to continue to next act..."
