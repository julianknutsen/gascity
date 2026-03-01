#!/usr/bin/env bash
# progress.sh — Watch bead completion for a pool label.
#
# Loops until all beads for the given pool are closed, displaying a live
# progress counter.
#
# Usage:
#   ./progress.sh [pool] [total]
#
# Args:
#   pool   — pool label to watch (default: worker)
#   total  — expected total beads (default: 100)

set -euo pipefail

POOL="${1:-worker}"
TOTAL="${2:-100}"

BOLD='\033[1m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
NC='\033[0m'

while true; do
    done_count=$(bd list --label="pool:$POOL" --status=closed --json 2>/dev/null | jq length 2>/dev/null || echo 0)
    open_count=$(bd list --label="pool:$POOL" --status=open --json 2>/dev/null | jq length 2>/dev/null || echo 0)
    printf "\r  ${CYAN}Completed:${NC} ${BOLD}%d/%d${NC}  |  ${CYAN}Open:${NC} %d  " "$done_count" "$TOTAL" "$open_count"
    if [ "$done_count" -ge "$TOTAL" ]; then
        echo ""
        echo -e "  ${GREEN}${BOLD}All $TOTAL tasks complete!${NC}"
        break
    fi
    sleep 2
done
