#!/usr/bin/env bash
# wasteland-poll.sh — Poll Wasteland wanted board and create beads for workers.
#
# Env vars (inherited from controller process):
#   WL_BIN          — path to wl CLI (default: "wl")
#   WL_PROJECT      — filter by project (empty = all)
#   WL_TARGET_POOL  — pool label for created beads (default: "polecat")
#   WL_PROJECT_MAP  — comma-separated project=rig routing map
set -euo pipefail

WL_BIN="${WL_BIN:-wl}"
WL_TARGET_POOL="${WL_TARGET_POOL:-polecat}"
WL_PROJECT="${WL_PROJECT:-}"
WL_PROJECT_MAP="${WL_PROJECT_MAP:-}"

created=0
skipped=0
failed=0

# 1. Sync (best-effort).
"$WL_BIN" sync 2>/dev/null || true

# 2. Browse open items.
browse_args=(browse --status open --json)
if [[ -n "$WL_PROJECT" ]]; then
  browse_args+=(--project "$WL_PROJECT")
fi

items=$("$WL_BIN" "${browse_args[@]}" 2>/dev/null) || {
  echo "wasteland-poll: wl browse failed" >&2
  exit 1
}

# Parse items into array of (id, title, project) tuples.
count=$(echo "$items" | jq 'length' 2>/dev/null) || count=0
if [[ "$count" -eq 0 ]]; then
  echo "wasteland-poll: no open items"
  exit 0
fi

# 3. Parse project map (if set).
declare -A project_map
if [[ -n "$WL_PROJECT_MAP" ]]; then
  IFS=',' read -ra mappings <<< "$WL_PROJECT_MAP"
  for mapping in "${mappings[@]}"; do
    key="${mapping%%=*}"
    val="${mapping#*=}"
    project_map["$key"]="$val"
  done
fi

# 4. For each item: dedup, claim, create bead.
for i in $(seq 0 $((count - 1))); do
  item_id=$(echo "$items" | jq -r ".[$i].id" 2>/dev/null)
  item_title=$(echo "$items" | jq -r ".[$i].title" 2>/dev/null)
  item_project=$(echo "$items" | jq -r ".[$i].project // empty" 2>/dev/null)

  # a) Dedup check.
  existing=$(bd list --labels "wasteland:${item_id}" --json 2>/dev/null | jq 'length' 2>/dev/null) || existing=0
  if [[ "$existing" -gt 0 ]]; then
    skipped=$((skipped + 1))
    continue
  fi

  # b) Claim.
  if ! "$WL_BIN" claim "$item_id" 2>/dev/null; then
    skipped=$((skipped + 1))
    continue
  fi

  # c) Determine target pool.
  target="$WL_TARGET_POOL"
  if [[ -n "$item_project" && -n "${project_map[$item_project]+x}" ]]; then
    rig="${project_map[$item_project]}"
    target="${rig}/${WL_TARGET_POOL}"
  fi

  # d) Create bead.
  if bd create \
    --title "$item_title" \
    --labels "wasteland:${item_id},pool:${target}" \
    --metadata "{\"wasteland_id\":\"${item_id}\",\"wasteland_project\":\"${item_project}\"}" \
    2>/dev/null; then
    created=$((created + 1))
  else
    failed=$((failed + 1))
    echo "wasteland-poll: bd create failed for ${item_id}" >&2
  fi
done

# 5. Summary.
echo "wasteland-poll: created=${created} skipped=${skipped} failed=${failed}"
