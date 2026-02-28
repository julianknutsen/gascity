#!/usr/bin/env bash
# peek-cycle.sh — Continuously cycle through active agent sessions.
#
# Shows recent terminal output from each agent in rotation, giving
# a live "security camera" view of what agents are doing.
#
# Usage:
#   ./peek-cycle.sh [lines] [delay]
#
# Args:
#   lines  — lines of output per agent (default: 20)
#   delay  — seconds to display each agent (default: 3)

set -euo pipefail

LINES="${1:-20}"
DELAY="${2:-3}"

CYAN='\033[0;36m'
DIM='\033[2m'
BOLD='\033[1m'
NC='\033[0m'

while true; do
    # Get list of running agents.
    agents=$(gc agent list --format name 2>/dev/null || true)

    if [ -z "$agents" ]; then
        clear
        echo -e "${DIM}Waiting for agents to start...${NC}"
        sleep "$DELAY"
        continue
    fi

    for agent in $agents; do
        clear
        echo -e "${CYAN}${BOLD}═══ $agent ═══${NC}"
        echo ""
        gc agent peek "$agent" --lines "$LINES" 2>/dev/null || echo -e "${DIM}(no output)${NC}"
        sleep "$DELAY"
    done
done
