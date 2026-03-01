#!/usr/bin/env bash
# narrate.sh — Display centered banners and narration pauses for demo recording.
#
# Source this file in act scripts to get narrate() and pause() functions.
#
# Usage:
#   source "$(dirname "$0")/narrate.sh"
#   narrate "Same topology, three stacks"
#   pause
#   narrate "Act complete" --sub "Switching to next act..."

# ── Colors ────────────────────────────────────────────────────────────────

NARR_BLUE='\033[0;34m'
NARR_CYAN='\033[0;36m'
NARR_GREEN='\033[0;32m'
NARR_BOLD='\033[1m'
NARR_DIM='\033[2m'
NARR_NC='\033[0m'

# ── Functions ─────────────────────────────────────────────────────────────

# narrate — Display a large centered banner.
#   narrate "Title text"
#   narrate "Title text" --sub "Subtitle text"
narrate() {
    local title="$1"
    local sub=""
    if [[ "${2:-}" == "--sub" ]]; then
        sub="${3:-}"
    fi

    local cols
    cols=$(tput cols 2>/dev/null || echo 80)
    local bar_len=$((cols > 70 ? 70 : cols))
    local bar
    bar=$(printf '%*s' "$bar_len" '' | tr ' ' '=')

    clear
    echo ""
    echo ""
    echo ""
    echo -e "${NARR_BLUE}${NARR_BOLD}  ${bar}${NARR_NC}"
    echo ""

    # Center the title.
    local pad=$(( (bar_len - ${#title}) / 2 ))
    [[ $pad -lt 2 ]] && pad=2
    printf "${NARR_CYAN}${NARR_BOLD}%*s%s${NARR_NC}\n" "$((pad + 2))" "" "$title"

    if [[ -n "$sub" ]]; then
        local sub_pad=$(( (bar_len - ${#sub}) / 2 ))
        [[ $sub_pad -lt 2 ]] && sub_pad=2
        echo ""
        printf "${NARR_DIM}%*s%s${NARR_NC}\n" "$((sub_pad + 2))" "" "$sub"
    fi

    echo ""
    echo -e "${NARR_BLUE}${NARR_BOLD}  ${bar}${NARR_NC}"
    echo ""
    echo ""
}

# pause — Wait for Enter key (narration pause point).
pause() {
    local msg="${1:-Press Enter to continue...}"
    echo -e "  ${NARR_DIM}${msg}${NARR_NC}"
    read -r
}

# step — Print a progress step.
step() {
    echo -e "${NARR_GREEN}>>>${NARR_NC} ${NARR_BOLD}$1${NARR_NC}"
}

# countdown — Visual countdown timer.
#   countdown 10 "Starting in"
countdown() {
    local secs="$1"
    local prefix="${2:-Continuing in}"
    for i in $(seq "$secs" -1 1); do
        printf "\r  ${NARR_DIM}${prefix} %d...${NARR_NC}  " "$i"
        sleep 1
    done
    printf "\r%*s\r" 40 ""
}
