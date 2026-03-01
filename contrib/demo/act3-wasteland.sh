#!/usr/bin/env bash
# act3-wasteland.sh — Act 3: Wasteland Auto-Claim
#
# Demonstrates: External work intake with zero human intervention.
# Shows the wasteland-feeder automation: poll -> claim -> bead -> sling
# to polecat pool -> polecat runs inference -> close.
#
# Prerequisite: Real `wl` binary installed and a Wasteland instance running.
#
# Usage:
#   ./act3-wasteland.sh
#
# Env vars:
#   WL_BIN           — path to wl CLI (default: wl)
#   ACT3_TIMEOUT     — auto-teardown seconds (default: 120)
#   ACT3_POLL_INTERVAL — wasteland poll interval (default: 10s)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
GC_SRC="$(cd "$SCRIPT_DIR/../.." && pwd)"
# shellcheck source=narrate.sh
source "$SCRIPT_DIR/narrate.sh"

DEMO_CITY="${DEMO_CITY:-$HOME/demo-city}"
DEMO_SESSION="gc-demo"
WL_BIN="${WL_BIN:-wl}"
ACT3_TIMEOUT="${ACT3_TIMEOUT:-120}"
ACT3_POLL_INTERVAL="${ACT3_POLL_INTERVAL:-10s}"

# ── Preflight ─────────────────────────────────────────────────────────────

command -v "$WL_BIN" >/dev/null 2>&1 || {
    echo "ERROR: wl binary not found at '$WL_BIN'" >&2
    echo "Set WL_BIN=/path/to/wl or install wasteland CLI." >&2
    exit 1
}

command -v gc >/dev/null 2>&1 || { echo "ERROR: gc not found in PATH" >&2; exit 1; }
command -v tmux >/dev/null 2>&1 || { echo "ERROR: tmux not found in PATH" >&2; exit 1; }

# ── Act 3 ─────────────────────────────────────────────────────────────────

narrate "Act 3: Wasteland Auto-Claim" --sub "External work intake — zero human intervention"
pause

# ── Seed wasteland items ──────────────────────────────────────────────────

step "Seeding Wasteland inference jobs..."
echo ""
echo "  Posting 3 inference jobs via wl infer post..."
echo ""

"$WL_BIN" infer post --prompt "What is the capital of France?" --model llama3.2:1b || \
    step "  (Post items manually: wl infer post --prompt '...' --model llama3.2:1b)"
"$WL_BIN" infer post --prompt "Explain gravity in one sentence" --model llama3.2:1b || true
"$WL_BIN" infer post --prompt "What is 2+2?" --model llama3.2:1b || true


echo ""
step "Inference jobs posted. Verify with: wl browse"
pause "Press Enter to start the city with wasteland-feeder..."

# ── Clean previous demo ──────────────────────────────────────────────────

if [ -d "$DEMO_CITY" ]; then
    (cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
    rm -rf "$DEMO_CITY"
fi
tmux kill-session -t "$DEMO_SESSION" 2>/dev/null || true

# ── Initialize city with gastown + wasteland-feeder ───────────────────────

step "Initializing city with gastown + wasteland-feeder topologies..."

gc init --from "$GC_SRC/examples/gastown" "$DEMO_CITY"

# Compose wasteland-feeder topology into the city.
# Add the wasteland-feeder topology to the city's topologies list.
if [ -d "$GC_SRC/topologies/wasteland-feeder" ]; then
    cp -r "$GC_SRC/topologies/wasteland-feeder" "$DEMO_CITY/topologies/"
fi

# Add wasteland-feeder to city.toml workspace topologies.
cd "$DEMO_CITY"

if grep -q '^topologies' city.toml; then
    # Already has topologies list — append wasteland-feeder.
    sed -i 's|\(topologies = \[.*\)\]|\1, "topologies/wasteland-feeder"]|' city.toml
elif grep -q '^\[workspace\]' city.toml; then
    # Has workspace section but no topologies — add it.
    sed -i '/^\[workspace\]/a topologies = ["topologies/wasteland-feeder"]' city.toml
fi

step "City configured with gastown + wasteland-feeder"

# ── Create demo rig ──────────────────────────────────────────────────────

DEMO_REPO="$DEMO_CITY/demo-repo"
mkdir -p "$DEMO_REPO"
(cd "$DEMO_REPO" && git init -q && echo "# Demo" > README.md && git add . && git commit -q -m "init")
gc rig add "$DEMO_REPO" --topology topologies/gastown

# ── Override poll interval for demo pacing ────────────────────────────────

step "Setting wasteland-poll interval to ${ACT3_POLL_INTERVAL} for demo pacing..."

# Override the automation interval via an overlay or direct edit.
POLL_TOML="$DEMO_CITY/topologies/wasteland-feeder/formulas/automations/wasteland-poll/automation.toml"
if [ -f "$POLL_TOML" ]; then
    sed -i "s|interval = \"2m\"|interval = \"${ACT3_POLL_INTERVAL}\"|" "$POLL_TOML"
    step "Poll interval set to ${ACT3_POLL_INTERVAL}"
fi

# ── Set up env vars for wasteland ─────────────────────────────────────────

export WL_BIN
export WL_TARGET_POOL="polecat"
export WL_RIG_DIR="$DEMO_REPO"

# ── Create 4-pane tmux session ────────────────────────────────────────────

step "Creating demo tmux layout..."

PANE_CTRL=$(tmux new-session -d -s "$DEMO_SESSION" -x 200 -y 50 -P -F "#{pane_id}")
PANE_EVENTS=$(tmux split-window -h -t "$PANE_CTRL" -P -F "#{pane_id}")
PANE_PEEK=$(tmux split-window -v -t "$PANE_CTRL" -P -F "#{pane_id}")
PANE_BEADS=$(tmux split-window -v -t "$PANE_EVENTS" -P -F "#{pane_id}")

tmux select-pane -t "$PANE_CTRL" -T "Controller"
tmux select-pane -t "$PANE_EVENTS" -T "Events"
tmux select-pane -t "$PANE_PEEK" -T "Peek"
tmux select-pane -t "$PANE_BEADS" -T "Beads"

tmux set-option -t "$DEMO_SESSION" pane-border-status top
tmux set-option -t "$DEMO_SESSION" pane-border-format "#{pane_title}"

# Pane 2: Events stream.
tmux send-keys -t "$PANE_EVENTS" \
    "cd $DEMO_CITY && gc events --follow" C-m

# Pane 3: Agent peek cycling.
tmux send-keys -t "$PANE_PEEK" \
    "cd $DEMO_CITY && $SCRIPT_DIR/peek-cycle.sh" C-m

# Pane 4: Bead watch for wasteland items.
# bd must run from the rig directory where .beads/ lives, not the city root.
tmux send-keys -t "$PANE_BEADS" \
    "cd $DEMO_REPO && watch -n5 'bd list --status=open --json 2>/dev/null | jq -r \".[] | [.id, .title, .status] | @tsv\" 2>/dev/null || echo \"No beads yet\"'" C-m

# Pane 1: Controller (foreground).
tmux send-keys -t "$PANE_CTRL" \
    "cd $DEMO_CITY && WL_BIN=$WL_BIN WL_TARGET_POOL=polecat WL_RIG_DIR=$DEMO_REPO gc start --foreground" C-m

step "City starting..."

# ── Auto-teardown timer ───────────────────────────────────────────────────

(
    sleep "$ACT3_TIMEOUT"
    (cd "$DEMO_CITY" && gc stop 2>/dev/null) || true
    tmux kill-session -t "$DEMO_SESSION" 2>/dev/null || true
) &
TEARDOWN_PID=$!
trap 'kill $TEARDOWN_PID 2>/dev/null || true' EXIT

# ── Narration ─────────────────────────────────────────────────────────────

echo ""
step "Watch the chain:"
echo "  1. wasteland-poll.sh fires every ${ACT3_POLL_INTERVAL}"
echo "  2. wl sync + wl browse finds open inference items"
echo "  3. Auto-claim via wl claim"
echo "  4. gc sling dispatches to polecat pool"
echo "  5. Polecat spawns, runs inference, closes bead"
echo "  6. Check Wasteland web UI — items marked completed"
echo ""
echo "  Auto-teardown in ${ACT3_TIMEOUT}s"
echo ""

tmux attach-session -t "$DEMO_SESSION"

# ── Done ──────────────────────────────────────────────────────────────────

narrate "Act 3 Complete" --sub "Wasteland items claimed, dispatched, and completed automatically"
pause "Press Enter to continue to next act..."
