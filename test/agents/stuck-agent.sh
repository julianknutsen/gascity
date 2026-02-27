#!/bin/bash
# Bash agent: stuck agent (test target).
# Simulates a stuck agent: sleeps forever, ignoring all nudges.
# Used as a target for shutdown dance and health monitoring tests.
#
# Required env vars (set by gc start):
#   GC_AGENT — this agent's name
#   GC_CITY  — path to the city directory

set -euo pipefail

# Stuck forever — never processes work
sleep 3600
