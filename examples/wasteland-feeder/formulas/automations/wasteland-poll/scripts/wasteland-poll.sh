#!/usr/bin/env bash
# wasteland-poll.sh — Poll Wasteland wanted board and create beads for workers.
#
# Dispatches inference items to the worker pool:
#   1. Open inference items → auto-claim → create bead with formula → pool.
#   2. Already-claimed inference items → create bead with formula → pool.
#
# Dedup ensures items processed in step 1 are skipped in step 2.
#
# Env vars (inherited from controller process):
#   WL_BIN          — path to wl CLI (default: "wl")
#   WL_PROJECT      — filter by project (empty = all)
#   WL_TARGET_POOL  — pool label for created beads (default: "polecat")
#   WL_PROJECT_MAP  — comma-separated project=rig routing map
#   WL_RIG_DIR      — rig directory for bd commands (default: cwd)
set -euo pipefail

WL_BIN="${WL_BIN:-wl}"
WL_TARGET_POOL="${WL_TARGET_POOL:-polecat}"
WL_PROJECT="${WL_PROJECT:-}"
WL_PROJECT_MAP="${WL_PROJECT_MAP:-}"
WL_RIG_DIR="${WL_RIG_DIR:-}"

# If WL_RIG_DIR is set, bd commands run there (where pool agents check for beads).
bd_cmd() {
  if [[ -n "$WL_RIG_DIR" ]]; then
    (cd "$WL_RIG_DIR" && bd "$@")
  else
    bd "$@"
  fi
}

created=0
skipped=0
failed=0

# 1. Sync (best-effort).
"$WL_BIN" sync 2>/dev/null || true

# 2. Parse project map (if set).
declare -A project_map
if [[ -n "$WL_PROJECT_MAP" ]]; then
  IFS=',' read -ra mappings <<< "$WL_PROJECT_MAP"
  for mapping in "${mappings[@]}"; do
    key="${mapping%%=*}"
    val="${mapping#*=}"
    project_map["$key"]="$val"
  done
fi

# resolve_pool determines the target pool for an item based on project routing.
resolve_pool() {
  local item_project="$1"
  local target="$WL_TARGET_POOL"
  if [[ -n "$item_project" && -n "${project_map[$item_project]+x}" ]]; then
    local rig="${project_map[$item_project]}"
    target="${rig}/${WL_TARGET_POOL}"
  fi
  echo "$target"
}

# dedup_check returns 0 if a bead already exists for the given wasteland ID.
dedup_check() {
  local item_id="$1"
  local existing
  existing=$(bd_cmd list --label "wasteland:${item_id}" --json 2>/dev/null | jq 'length' 2>/dev/null) || existing=0
  [[ "$existing" -gt 0 ]]
}

# dispatch_item creates a bead and slings it to the pool.
# Inference items get the inference formula.
dispatch_item() {
  local item_id="$1" item_title="$2" item_type="$3" item_project="$4"

  local target
  target=$(resolve_pool "$item_project")

  local bead_id
  bead_id=$(bd_cmd create \
    --title "$item_title" \
    --labels "wasteland:${item_id}" \
    --metadata "{\"wasteland_id\":\"${item_id}\",\"wasteland_type\":\"${item_type}\",\"wasteland_project\":\"${item_project}\"}" \
    2>/dev/null) || bead_id=""

  if [[ -z "$bead_id" ]]; then
    failed=$((failed + 1))
    echo "wasteland-poll: bd create failed for ${item_id}" >&2
    return
  fi

  local sling_args=("$target" "$bead_id" --force)
  if [[ "$item_type" == "inference" ]]; then
    sling_args+=(--on=mol-wasteland-inference --var "wasteland_id=${item_id}")
  fi

  if gc sling "${sling_args[@]}" 2>/dev/null; then
    created=$((created + 1))
  else
    failed=$((failed + 1))
    echo "wasteland-poll: gc sling failed for ${bead_id} (${item_id})" >&2
  fi
}

# ── Step 1: Auto-claim open inference items ──────────────────────────────

browse_args=(browse --status open --type inference --json)
if [[ -n "$WL_PROJECT" ]]; then
  browse_args+=(--project "$WL_PROJECT")
fi

infer_items=$("$WL_BIN" "${browse_args[@]}" 2>/dev/null) || infer_items="[]"
infer_count=$(echo "$infer_items" | jq 'length' 2>/dev/null) || infer_count=0

for i in $(seq 0 $((infer_count - 1))); do
  item_id=$(echo "$infer_items" | jq -r ".[$i].id" 2>/dev/null)
  item_title=$(echo "$infer_items" | jq -r ".[$i].title" 2>/dev/null)
  item_project=$(echo "$infer_items" | jq -r ".[$i].project // empty" 2>/dev/null)

  if dedup_check "$item_id"; then
    skipped=$((skipped + 1))
    continue
  fi

  if ! "$WL_BIN" claim "$item_id" 2>/dev/null; then
    skipped=$((skipped + 1))
    continue
  fi

  dispatch_item "$item_id" "$item_title" "inference" "$item_project"
done

# ── Step 2: Dispatch all claimed items ───────────────────────────────────

browse_args=(browse --status claimed --type inference --json)
if [[ -n "$WL_PROJECT" ]]; then
  browse_args+=(--project "$WL_PROJECT")
fi

claimed_items=$("$WL_BIN" "${browse_args[@]}" 2>/dev/null) || claimed_items="[]"
claimed_count=$(echo "$claimed_items" | jq 'length' 2>/dev/null) || claimed_count=0

for i in $(seq 0 $((claimed_count - 1))); do
  item_id=$(echo "$claimed_items" | jq -r ".[$i].id" 2>/dev/null)
  item_title=$(echo "$claimed_items" | jq -r ".[$i].title" 2>/dev/null)
  item_type=$(echo "$claimed_items" | jq -r ".[$i].type // empty" 2>/dev/null)
  item_project=$(echo "$claimed_items" | jq -r ".[$i].project // empty" 2>/dev/null)

  if dedup_check "$item_id"; then
    skipped=$((skipped + 1))
    continue
  fi

  dispatch_item "$item_id" "$item_title" "$item_type" "$item_project"
done

# 3. Summary.
echo "wasteland-poll: created=${created} skipped=${skipped} failed=${failed}"
