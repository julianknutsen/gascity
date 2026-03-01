#!/usr/bin/env bash
# seed-hyperscale.sh — Create N work beads for the hyperscale pool.
#
# Usage:
#   ./seed-hyperscale.sh [count] [city-dir]
#
# Args:
#   count    — number of beads to create (default: 100)
#   city-dir — city directory to cd into (default: .)

set -euo pipefail

COUNT="${1:-100}"
CITY="${2:-.}"

cd "$CITY"

echo "Seeding $COUNT beads for pool:worker..."

for i in $(seq 1 "$COUNT"); do
    bd create "hyperscale-task-$i" --type task --label pool:worker
done

echo "Seeded $COUNT beads for pool:worker"
