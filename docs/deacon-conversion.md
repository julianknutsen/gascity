# Deacon Conversion Decision Record

The deacon is the **LLM sidekick to the controller**. It handles periodic
tasks that require judgment, observation, or cross-rig coordination — things
the Go controller can't or shouldn't do. It also dispatches work to utility
agent pools (dogs) for multi-step infrastructure tasks.

This document reviews every job in `mol-deacon-patrol.formula.toml` (v11,
23 steps) and records the conversion decision for each.

## Context: What Changed

| Layer | Gas Town | Gas City |
|-------|----------|----------|
| Process lifecycle | Witness nukes, daemon heartbeats | Controller reconcile loop |
| Zombie detection | Witness + deacon cross-reference | Controller detects, restarts |
| Orphaned beads | Deacon dispatches dog | Witness recovers per-rig |
| Agent restart | Daemon + deacon LIFECYCLE mail | `gc agent request-restart <target>` |
| Work flow | MR beads, POLECAT_DONE mail | Metadata on work beads, direct flow |
| Worktree lifecycle | Polecat self-nuke via `gt done` | Formula creates, witness cleans |
| Gate checks | Both deacon and witness (overlap) | Deacon only (town-wide) |
| Gated molecule dispatch | Deacon polls `bd mol ready --gated` | Agents add blocking dep on work bead; normal scheduling resumes when gate clears |
| Convoy feeding | Deacon dispatches dog to feed | `bd ready` queries replace explicit feeding |
| Cross-rig deps | Dog dispatched for propagation | Deacon inline: convert `blocks` → `related` deps |
| Stuck agent recovery | Death warrant → Boot kills | Death warrant → shutdown dance formula dispatched to dog pool |

## Decision Format

For each step: **KEEP**, **REMOVE**, **SIMPLIFY**, or **MERGE** with rationale.

---

## 1. inbox-check — Handle callbacks from agents

**Decision: KEEP, simplify message types**

The deacon's inbox is the town-level callback channel.

| Message Type | Gas Town | Gas City | Decision |
|---|---|---|---|
| HELP / Escalation | Assess, forward to Mayor | Same | Keep |
| DOG_DONE | Dog reports completion | Same | Keep |
| LIFECYCLE | Deacon kills/restarts sessions | Replaced by `gc agent request-restart <target>` | Remove |
| CONVOY_NEEDS_FEEDING | Refinery triggers convoy feed | `bd ready` queries replace feeding | Remove |
| RECOVERED_BEAD | Witness sends after orphan recovery | Witness + controller are self-sufficient | Remove |
| POLECAT_STARTED | Polecat announces startup | Controller manages, no announcement needed | Remove |
| POLECAT_DONE | Polecat reports completion | Direct work bead flow via metadata | Remove |
| Other / informational | Archive after reading | Same | Keep |

**Changes:**
- Remove LIFECYCLE handling — `gc agent request-restart <target>` replaces it
- Remove POLECAT_STARTED / POLECAT_DONE (direct work bead flow)
- Remove RECOVERED_BEAD (witness + controller self-sufficient)
- Remove CONVOY_NEEDS_FEEDING (`bd ready` replaces feeding)
- Context check at start of each cycle via RSS check + `gc agent request-restart`

---

## 2. orphan-process-cleanup — Kill orphaned claude subagent processes

**Decision: KEEP**

Claude Code's Task tool spawns subagent processes that sometimes don't clean
up. These accumulate and consume significant memory. Detection requires
interpreting `ps` output (TTY = "?" means orphaned) — this is judgment work
the Go controller shouldn't hardcode (ZFC).

**Changes:** None. The step is already well-written.

---

## 3. gate-evaluation — Evaluate pending async gates

**Decision: KEEP as `check-gates`, REMOVE from witness**

Gates are town-wide coordination primitives. The deacon is the right home:
- Gates can span rigs (cross-rig coordination)
- The deacon handles all town-level scheduling
- Having both witness and deacon check gates is redundant

The deacon closes gates when conditions are met (timer elapsed, condition
true, etc.). Once a gate closes, scheduling of the unblocked work happens
automatically — agents add a `blocks` dep from their work bead to the gate
bead, and when the gate closes the dep is satisfied and `bd ready` finds
the work bead. Crash-loop backoff limits waste if agents don't follow the
protocol.

**Follow-up:** Remove `check-gates` AND `check-swarms` from witness formula.
Witness shrinks to 4 steps: check-inbox, recover-orphaned-beads,
check-refinery, next-iteration.

**Changes:**
- Close gates inline (timer, condition, etc.)
- No dispatch step needed — normal scheduling handles unblocked work

---

## 4. dispatch-gated-molecules — Dispatch molecules with resolved gates

**Decision: REMOVE**

Agents add a `blocks` dep from their work bead to the gate bead when they
hit a blocked step, then unassign and exit:

```bash
bd dep add <work-bead> <gate-id>                  # work bead blocked by gate
bd update <work-bead> --status=open --assignee=""  # back to pool
exit
```

When the gate closes, `computeBlockedIDs` no longer marks the work bead
as blocked, and `bd ready` finds it. Normal scheduling re-dispatches.

Wasteful restarts (agent hits gate, doesn't follow protocol, crash-loops)
are acceptable with crash-loop backoff. Simpler than deacon-managed dispatch.

---

## 5. check-convoy-completion — Check convoy completion

**Decision: KEEP as `check-convoys`**

Convoys are cross-rig coordination beads tracking work across multiple rigs.
Convoy completion requires:
1. Cross-rig status lookup (tracked issues may be in different beads databases)
2. Close the convoy bead when all children are closed
3. Notification fan-out to owner + subscribers

This is inherently cross-rig — a per-rig witness can't do it. The deacon
runs `gc convoy check` inline (queries all open convoys, checks tracked
issue status cross-rig, auto-closes completed convoys, notifies).

**Changes:** None. `gc convoy check` works as-is.

---

## 6. feed-stranded-convoys — Feed stranded convoys

**Decision: REMOVE**

`bd ready` queries replace explicit convoy feeding. Ready work in a convoy
is visible via normal scheduling. No dog dispatch needed.

---

## 7. resolve-external-deps — Resolve external dependencies

**Decision: MERGE with fire-notifications (step 8) as `cross-rig-deps`**

Cross-rig dependency resolution. When an issue in rig A closes, dependent
issues in rig B may still be blocked because `computeBlockedIDs` doesn't
resolve external deps (see `docs/bd-roadmap.md` item 1).

**Workaround:** The deacon actively resolves cross-rig deps by converting
`blocks` → `related` deps, preserving the audit trail while removing the
blocking semantics:

```bash
# 1. Find recently closed issues
bd list --status=closed --closed-after=<last-patrol> --json

# 2. For each, find cross-rig dependents
bd dep list <id> --direction=up --type=blocks --json
# Filter for external: deps

# 3. Resolve: convert blocks → related (preserves audit trail)
bd dep remove <dependent-id> external:<project>:<closed-issue>
bd dep add <dependent-id> external:<project>:<closed-issue> --type=related
```

No dog dispatch needed — `bd` routing handles cross-rig database access.
No witness notification needed — `bd ready` picks up unblocked work naturally.

**Changes:**
- Deacon inline, no dog dispatch (remove `mol-dep-propagate`)
- Convert `blocks` → `related` instead of removing (audit trail)
- No notifications — scheduling is automatic once dep is resolved
- Filed as bd enhancement to eventually eliminate this step entirely

---

## 8. fire-notifications — Fire notifications

**Decision: MERGE into step 7 (cross-rig-deps)**

Notifications are unnecessary — resolving the dep makes work appear in
`bd ready` automatically. The witness and controller handle scheduling.

---

## 9. health-scan — Check Witness and Refinery health

**Decision: SIMPLIFY to work-layer observation**

The controller owns "is the agent running?" The deacon owns "is work
flowing?" These are different questions.

**Remove:**
- "Are agents running?" checks — controller concern
- Health nudges (HEALTH_CHECK) — Idle Town Principle violation
- `state.json` tracking — "No status files"
- Cycle counting / decision matrix — ZFC violation

**Keep:** Work-layer health observation per rig:
- **Witness:** Check wisp freshness. Each patrol cycle burns a wisp. If
  the last wisp is older than max backoff cap + buffer, something is wrong.
  But: if no work exists, the witness is legitimately idle — not stuck.
- **Refinery:** Check wisp freshness + queue state. Wisp open + no work
  assigned = idle (fine). Wisp open + work assigned + stale UpdatedAt =
  stuck (concern).

No hardcoded thresholds. The deacon reads wisp timestamps, queue state,
and makes a judgment call (ZFC-aligned). Escalate to mayor if something
looks stuck.

---

## 10. zombie-scan — Detect zombie polecats

**Decision: REMOVE**

Fully redundant with Gas City's controller reconcile loop:
- Controller detects dead agents via `IsRunning()` check
- Controller restarts them (with crash-loop backoff)
- Witness handles orphaned beads when agents won't come back

---

## 11. plugin-run — Execute registered plugins

**Decision: KEEP as `periodic-formulas`**

Plugins are just formulas with gate conditions. The deacon checks if any
registered maintenance formulas are due and pours wisps for them.

**Changes:**
- Reframe: "plugins" → "registered maintenance formulas"
- The formula IS the plugin. No separate plugin.md with TOML frontmatter.
- Source: configured in `city.toml` under deacon's agent config

---

## 12. dog-pool-maintenance — Maintain dog pool

**Decision: REMOVE**

Controller pool reconciliation handles pool sizing via `[[agents]]` config.

---

## 13. dog-health-check — Check for stuck dogs

**Decision: KEEP as `utility-agent-health`, with shutdown dance dispatch**

The controller detects dead agents. But "working too long" is a work-layer
judgment: is this agent stuck, or is the task just slow? The controller
can't know this. The deacon can (by checking bead/wisp timestamps).

When the deacon detects a stuck utility agent (or any agent with stale
work), it files a death warrant and dispatches the shutdown dance formula
to the utility agent pool:

```bash
# File warrant bead
WARRANT=$(bd create --type=warrant --title="Stuck: <agent>" \
  --metadata '{"target":"<session>","reason":"<reason>","requester":"deacon"}')

# Pour shutdown dance, assign to dog pool
WISP=$(bd mol wisp mol-shutdown-dance \
  --var warrant_id=$WARRANT \
  --var target=<stuck-session> \
  --var reason="Stale work bead, no progress" \
  --var requester=deacon \
  --json | jq -r '.new_epic_id')
bd update "$WISP" --label=role:dog
```

The shutdown dance is a 3-attempt interrogation protocol (60s → 120s →
240s) that gives the agent multiple chances to prove it's alive before
killing the session. This is due process, not just "kill stuck things."
The formula handles the multi-stage logic; the deacon just dispatches.

**Changes:**
- Rename: "dog-health-check" → "utility-agent-health"
- Dispatch `mol-shutdown-dance` instead of filing death warrant for Boot
- Remove state-based chronic failure tracking (no status files)
- Keep the shutdown dance formula (updated for Gas City)

---

## 14. orphan-check — Detect abandoned work

**Decision: SIMPLIFY to town-wide sweep**

The witness handles detailed per-rig orphaned bead recovery (with worktree
salvage). The deacon does a lightweight town-wide sanity check:
- Beads assigned to agents that don't exist in ANY rig
- Beads assigned to the deacon's own utility agents that died
- Cross-rig orphans the per-rig witness might miss

**Changes:**
- Lightweight: `bd list --status=in_progress` → cross-reference with
  `gc agent list` → reset obviously orphaned beads
- Defer detailed worktree recovery to per-rig witness

---

## 15. session-gc — Detect cleanup needs

**Decision: SIMPLIFY as `system-health`**

Run `gc doctor` inline (quick check), act on simple findings, escalate
complex issues to mayor. No dog dispatch needed.

---

## 16. wisp-compact — Compact expired wisps

**Decision: KEEP**

Wisps accumulate over time. TTL-based cleanup is valid periodic maintenance.
Delete closed wisps past TTL.

**Changes:** Simplify — remove promotion logic, just delete expired wisps.

---

## 17. compact-report — Send compaction digest report

**Decision: REMOVE**

Telemetry, not core. Re-add as a registered periodic formula if needed.

---

## 18. costs-digest — Aggregate daily costs [DISABLED]

**Decision: REMOVE**

Already disabled. Dead code. No data source.

---

## 19. patrol-digest — Aggregate daily patrol digests

**Decision: REMOVE**

Telemetry, not core. Re-add as a periodic formula if needed.

---

## 20. log-maintenance — Rotate logs and prune state

**Decision: REMOVE**

- `daemon.log` — Gas Town artifact. Controller manages its own logs.
- `state.json` — violates "No status files — query live state."
- Log rotation — OS-level concern (logrotate), not an agent job.

---

## 21. patrol-cleanup — End-of-cycle inbox hygiene

**Decision: MERGE into next-iteration**

Quick inbox check before looping. Doesn't warrant its own step.

---

## 22. context-check — Check own context limit

**Decision: MERGE into next-iteration**

Use `gc agent request-restart` like witness and refinery.

---

## 23. loop-or-exit — Loop or exit for respawn

**Decision: KEEP as `next-iteration`**

Standard patrol loop machinery. Pour next wisp before burning current one.
Exponential backoff wait.

**Changes:**
- Merge patrol-cleanup and context-check into this step
- Use `gc agent request-restart` for context exhaustion
- Use pour-before-burn pattern (consistent with witness/refinery)

---

## Summary

### Steps in new formula (12 steps, down from 23)

| # | ID | Source | What it does |
|---|---|---|---|
| 1 | check-inbox | inbox-check (simplified) | Context check + mail handling |
| 2 | orphan-process-cleanup | kept | Kill orphaned subagent processes |
| 3 | check-gates | gate-evaluation (kept) | Close elapsed timer gates, evaluate conditions |
| 4 | check-convoys | check-convoy-completion (kept) | Cross-rig convoy completion + close + notify |
| 5 | cross-rig-deps | resolve-external-deps + fire-notifications (merged) | Convert satisfied cross-rig `blocks` → `related` deps |
| 6 | health-scan | health-scan (simplified) | Work-layer health: wisp freshness + queue state |
| 7 | periodic-formulas | plugin-run (reframed) | Dispatch registered maintenance formulas |
| 8 | utility-agent-health | dog-health-check (renamed) | Detect stuck agents, dispatch shutdown dance |
| 9 | town-orphan-sweep | orphan-check (simplified) | Lightweight cross-rig orphan detection |
| 10 | system-health | session-gc (simplified) | Run `gc doctor`, act on findings |
| 11 | wisp-compact | wisp-compact (simplified) | TTL-based wisp cleanup |
| 12 | next-iteration | loop-or-exit + patrol-cleanup + context-check (merged) | Pour next wisp, context check, wait, loop |

### Steps removed (11)

| Step | Reason |
|---|---|
| dispatch-gated-molecules | Agents add blocking dep on work bead; normal scheduling handles |
| feed-stranded-convoys | `bd ready` queries replace convoy feeding |
| zombie-scan | Controller reconcile loop handles zombie detection |
| dog-pool-maintenance | Controller pool scaling handles pool sizing |
| compact-report | Telemetry, not core. Re-add as periodic formula if needed |
| costs-digest | Dead code (disabled, no data source) |
| patrol-digest | Telemetry, not core. Re-add as periodic formula if needed |
| log-maintenance | No status files. OS-level concern |
| patrol-cleanup | Merged into next-iteration |
| context-check | Merged into next-iteration |
| fire-notifications | Merged into cross-rig-deps |

### Universal shutdown dance protocol

When ANY agent detects a stuck agent, the response is always:
1. File a warrant bead: `bd create --type=warrant --label=role:dog`
2. The dog pool picks it up and runs `mol-shutdown-dance`
3. The shutdown dance gives the stuck agent due process (3 attempts)

| Detector | Stuck agent | Detection method |
|---|---|---|
| Deacon health-scan | Witness | Stale patrol wisp |
| Deacon health-scan | Refinery | Stale wisp + queue has work |
| Deacon utility-agent-health | Dog | Stale wisp/bead |
| Witness check-polecat-health | Polecat | Stale work bead, no progress |

No agent kills anything directly. The shutdown dance is the single
recovery mechanism for all stuck agents.

### Follow-up changes needed

1. **Remove `check-gates` from witness formula** — gates are town-wide,
   deacon handles them.
2. **Remove `check-swarms` from witness formula** — swarms are town-wide
   batch coordination (convoys). Deacon handles.
3. **Add `check-polecat-health` to witness formula** — detect stuck
   polecats via wisp/bead staleness, file warrants for dog pool.
4. **Update witness formula** to 5 steps: check-inbox,
   recover-orphaned-beads, check-refinery, check-polecat-health,
   next-iteration.
5. **Update `mol-shutdown-dance`** for Gas City — remove Boot/dog-pool
   references, use `gc agent request-restart <target>` or tmux kill as
   final step, remove state files.
6. **File bd enhancement** for cross-rig `computeBlockedIDs` (see
   `docs/bd-roadmap.md`) — would eliminate the `cross-rig-deps` step.
7. **Implement `gc agent request-restart <target>`** — non-blocking
   third-party variant. Sets `GC_RESTART_REQUESTED` on target session,
   returns immediately.

### Prompt changes needed

| Remove | Reason |
|---|---|
| Capability Ledger section | Motivational fluff, not operational |
| `state.json` / `heartbeat.json` | No status files |
| Step banners with emojis | Gas Town aesthetic |
| "Where to File Beads" section | Gas Town multi-rig routing |
| Prefix-based routing section | Gas Town specific |
| Boot/death warrant for Boot references | Boot is Gas Town role; warrants stay but dispatch to dog pool |
| `gc context --usage` | Use `gc agent request-restart` |
| `gc rig suspend/resume` | Naming TBD — park/dock in Gas City |

| Keep/Update | Reason |
|---|---|
| Propulsion Principle | Core philosophy, valid |
| Startup Protocol | Standard GUPP pattern |
| Mol discovery (`bd mol current`) | Standard pattern |
| Hookable Mail | Valid ad-hoc instruction mechanism |
| Idle Town Principle | Valid — don't disturb idle agents |
| Architecture diagram | Update for Gas City (controller, not daemon) |
| Command quick-reference | Update for Gas City commands |
| Shutdown dance dispatch | Keep — dispatch `mol-shutdown-dance` to dog pool |
