# Gas Town Upstream Audit — Parity Tracking

Audit of 574 commits from `gastown:upstream/main` since Gas City was created
(2026-02-22). Organized by theme so we can review together and decide actions.

**Legend:** `[ ]` = pending review, `[x]` = addressed, `[-]` = skipped (N/A)

---

## 1. Persistent Polecat Pool (ARCHITECTURAL)

The biggest change in Gas Town: polecats no longer die after completing work.
"Done means idle, not dead." Sandboxes preserved for reuse, witness restarts
instead of nuking, completion signaling via agent beads instead of mail.

### 1a. Polecat lifecycle: done = idle
- [ ] **c410c10a** — `gt done` sets agent state to "idle" instead of self-nuking
  worktree. Sling reuses idle polecats before allocating new ones.
- [ ] **341fa43a** — Phase 1: `gt done` transitions to IDLE with sandbox preserved,
  worktree synced to main for immediate reuse.
- [ ] **0a653b11** — Polecats self-manage completion, set agent_state=idle directly.
  Witness is safety-net only for crash recovery.
- [ ] **63ad1454** — Branch-only reuse: after done, worktree syncs to main, old
  branch deleted. Next sling uses `git checkout -b` on existing worktree.
- **Action:** Update `mol-polecat-work.formula.toml` — line 408 says "You are
  GONE. Done means gone. There is no idle state." Change to reflect persistent
  model. Update polecat prompt similarly.

### 1b. Witness: restart, never nuke
- [ ] **016381ad** — All `gt polecat nuke` in zombie detection replaced with
  `gt session restart`. "Idle Polecat Heresy" replaced with "Completion Protocol."
- [ ] **b10863da** — Idle polecats with clean sandboxes skipped entirely by
  witness patrol. Dirty sandboxes escalated for recovery.
- **Action:** Update witness patrol formula and prompt: replace automatic
  nuking with restart-first policy. Idle polecats are healthy.

### 1c. Bead-based completion discovery (replaces POLECAT_DONE mail)
- [ ] **c5ce08ed** — Agent bead completion metadata: exit_type, mr_id, branch,
  mr_failed, completion_time.
- [ ] **b45d1511** — POLECAT_DONE mail deprecated. Polecats write completion
  metadata to agent bead + send tmux nudge. Witness reads bead state.
- [ ] **90d08948** — Witness patrol v9: survey-workers Step 4a uses
  DiscoverCompletions() from agent_state=done beads.
- **Action:** Update witness patrol formula: mark POLECAT_DONE mail handling
  as deprecated fallback. Step 4a is the PRIMARY completion signal.

### 1d. Polecat nuke behavior
- [ ] **330664c2** — Nuke no longer deletes remote branches. Refinery owns
  remote branch cleanup after merge.
- [ ] **4bd189be** — Nuke checks CommitsAhead before deleting remote branches.
  Unmerged commits preserved for refinery/human.
- **Action:** Update polecat prompt if it discusses cleanup behavior.

---

## 2. Polecat Work Formula v7

Major restructuring from 10 steps to 7, removing preflight tests entirely.

- [ ] **12cf3217** — Drop full test suite from polecat formula. Refinery owns
  main health via bisecting merge queue. Steps: remove preflight-tests, replace
  run-tests with build-check (compile + targeted tests only), consolidate
  cleanup-workspace and prepare-for-review.
- [ ] **9d64c0aa** — Sleepwalking polecat fix: HARD GATE requiring >= 1 commit
  ahead of origin/base_branch. Zero commits is now a hard error in commit-changes,
  cleanup-workspace, and submit-and-exit steps.
- [ ] **4ede6194** — No-changes exit protocol: polecat must run `bd close <id>
  --reason="no-changes: <explanation>"` + `gt done` when bead has nothing to
  implement. Prevents spawn storms.
- **Action:** Rewrite `mol-polecat-work.formula.toml` to match v7 structure.
  Add the HARD GATE commit verification and no-changes exit protocol.

---

## 3. Communication Hygiene: Nudge over Mail

Every mail creates a permanent Dolt commit. Nudges are free (tmux send-keys).

### 3a. Role template sections
- [x] **177606a4** — "Communication Hygiene: Nudge First, Mail Rarely" sections
  added to deacon, dog, polecat, and witness templates. Dogs should NEVER send
  mail. Polecats have 0-1 mail budget per session.
- [x] **a3ee0ae4** — "Dolt Health: Your Part" sections in polecat and witness
  prompts. Nudge don't mail, don't create unnecessary beads, close your beads.
- **Action:** ~~Add Communication Hygiene + Dolt Health sections to all four
  role prompts in examples/gastown.~~ DONE.

### 3b. Mail-to-nudge conversions (Go + formula)
- [ ] **7a578c2b** — Six mail sends converted to nudges: MERGE_FAILED,
  CONVOY_NEEDS_FEEDING, worker rejection, MERGE_READY, RECOVERY_NEEDED,
  HandleMergeFailed. Mail preserved only for convoy completion (handoff
  context) and escalation to mayor.
- [ ] **5872d9af** — LIFECYCLE:Shutdown, MERGED, MERGE_READY, MERGE_FAILED
  are now ephemeral wisps instead of permanent beads.
- [ ] **98767fa2** — WORK_DONE messages from `gt done` are ephemeral wisps.
- **Action:** Update refinery and witness prompt communication protocol
  sections to emphasize nudge for routine, mail for must-survive-session-death.

### 3c. Mail drain + improved instructions
- [ ] **655620a1** — Witness patrol v8: `gt mail drain` step archives stale
  protocol messages (>30 min). Batch processing when inbox > 10 messages.
- [ ] **9fb00901** — Overhauled mail instructions in crew and polecat templates:
  `--stdin` heredoc pattern, address format docs, common mistakes section.
- [ ] **8eb3d8bb** — Generic names (`alice/`) in crew template mail examples.
- **Action:** Update crew, polecat, and witness formulas/prompts with improved
  mail instructions and drain capability.

---

## 4. Batch-then-Bisect Merge Queue

Fundamental change to refinery processing model.

- [-] **7097b85b** — Batch-then-bisect merge queue. SDK-level Go machinery.
  Our event-driven one-branch-per-wisp model is intentional. N/A for topology.
- [-] **c39372f4** — `gt mq post-merge` replaces multi-step cleanup. Our direct
  work-bead model (no MR beads) already handles this atomically. N/A.
- [x] **048a73fe** — Duplicate bug check before filing pre-existing test failures.
  Added `bd list --search` dedup check to handle-failures step.
- **Also ported:** ZFC decision table in refinery prompt, patrol-summary step
  in formula for audit trail / handoff context.

---

## 5. Refinery Target-Aware Merging

Support for integration branches (not just always merging to main).

- [x] **75b72064 + 15b4955d + 33534823 + 87caa55d** — Target Resolution Rule.
  **Disposition:** No global toggle needed — polecat owns target via `metadata.target`,
  refinery reads it mechanically. Ported: FORBIDDEN clause for raw integration branch
  landing (prompt + formula), epic bead assignment for auto-land (formula), fixed
  command quick-reference to use `$TARGET` instead of hardcoded default branch.

---

## 6. Witness Patrol Improvements

### 6a. MR bead verification
- [-] **55c90da5** — Verify MR bead exists before sending MERGE_READY.
  **Disposition:** N/A — we don't use MR beads. Polecats assign work beads
  directly to refinery with branch metadata. The failure mode doesn't exist.

### 6b. Spawn storm detection
- [x] **70c1cbf8** — Track bead respawn count, escalate on threshold.
  **Disposition:** Implemented as exec automation `spawn-storm-detect` in
  maintenance topology. Script tracks reset counts in a ledger, mails mayor
  when any bead exceeds threshold. Witness sets `metadata.recovered=true`
  on reset beads to feed the detector.

### 6c. MQ verification in recovery
- [-] **b5553115** — Three-verdict recovery model.
  **Disposition:** N/A — our reset-to-pool model covers this. Work bead
  assignment to refinery IS submission. Witness already checks assignee
  before recovering. No intermediate MR state to verify.

### 6d. Policy decisions moved to prompts (ZFC)
- [x] **977953d8 + 3bf979db** — Remove hardcoded escalation policy.
  **Disposition:** Replaced "In ALL cases: notify mayor" with judgment-based
  notification table in witness formula and prompt. Routine pool resizes
  no longer generate mayor mail. Witness decides severity.

---

## 7. Root-Only Wisps Architecture

From batch 3 analysis (session summary).

- [ ] Root-only wisps: formula steps shown inline via `gt patrol report`
  instead of materialized as individual wisp rows. Dramatically reduces
  wisp table bloat.
- [ ] Deacon patrol formula: gated check command, stale dog detection,
  heartbeat steps updated.
- **Action:** Evaluate impact on deacon patrol and witness formulas in
  examples/gastown. Update formula step reporting model.

---

## 8. Infrastructure Dogs (New Formulas)

### 8a. Existing dogs updated
- [ ] **d2f9f2af** — JSONL Dog: spike detection + pollution firewall. New
  `verify` step between export and push. `spike_threshold` variable.
- [ ] **37d57150** — Reaper Dog: auto-close step for issues > 30 days
  (excluding epics, P0/P1, active deps). `stale_issue_age` variable.
  **Note:** Later reverted by ZFC fixes (915f1b7e, f61ff0ac).
- [ ] **bc9f395a** — Doctor Dog: structured JSON reporting model (advisory).
  **Then** 176b4963 re-adds automated actions with 10-min cooldowns.
  **Then** 89ccc218 reverts to configurable advisory recommendations.
- **Action:** Add JSONL Dog verify step. Doctor Dog should use advisory model.
  Remove reaper auto-close (ZFC violation per 915f1b7e).

### 8b. New dog formulas
- [ ] **739a36b7** — Janitor Dog: cleans orphan test DBs on Dolt test server.
  4 steps: scan, clean, verify (production read-only check), report.
- [ ] **85887e88** — Compactor Dog: flattens Dolt commit history. Steps:
  inspect, compact, verify, report. Threshold 10,000. Formula-only pattern.
- [ ] **1123b96c** — Surgical rebase mode for Compactor. `mode` config
  ('flatten'|'surgical'), `keep_recent` (default 50).
- [ ] **3924d560** — SQL-based flatten on running server. No downtime.
- **Action:** Add new dog formulas to maintenance topology if we want full
  parity. Consider whether all dogs are needed for the SDK example.

### 8c. Dog lifecycle
- [ ] **b4ed85bb** — `gt dog done` auto-terminates tmux session after 3s.
  Dogs should NOT idle at prompt.
- [ ] **427c6e8a** — Lifecycle defaults: Wisp Reaper (30m), Compactor (24h),
  Doctor (5m), Janitor (15m), JSONL Backup (15m), FS Backup (15m),
  Maintenance (daily 03:00, threshold 1000).
- **Action:** Update dog prompt for auto-termination. Add lifecycle defaults
  to maintenance topology config.

---

## 9. Prompt Template Updates

### 9a. Mayor
- [ ] **4c9309c8** — Rig Wake/Sleep Protocol: dormant-by-default workflow.
  All rigs start docked. Mayor undocks/docks as needed.
- [ ] **faf45d1c** — Fix-Merging Community PRs: `Co-Authored-By` attribution.
- [ ] **39962be0** — `auto_start_on_boot` renamed to `auto_start_on_up`.
- **Action:** Add Rig Wake/Sleep Protocol + community PR attribution to
  mayor prompt.

### 9b. Crew
- [ ] **12cf3217** — Identity clarification: "You are the AI agent (crew/{{.Polecat}}).
  The human is the Overseer."
- [ ] **faf45d1c** — Fix-Merging Community PRs section.
- [ ] **9fb00901** — Improved mail instructions with `--stdin` heredoc pattern,
  common mistakes section.
- **Action:** Add identity clarification + improved mail instructions to
  crew prompt.

### 9c. Boot
- [ ] **383945fb** — ZFC fix: removed Go decision engine from degraded triage.
  Decisions (heartbeat staleness, idle detection, backoff labels, molecule
  progress) now belong in boot formula, not Go code.
- **Action:** Ensure boot prompt includes sufficient triage decision guidance.

### 9d. Template path fix
- [ ] (batch 3) Template paths changed from `~/gt` to `{{ .TownRoot }}`.
- **Action:** Verify all template references use `{{ .CityRoot }}` or
  equivalent, not hardcoded paths.

---

## 10. Formula System Enhancements

- [ ] **67b0cdfe** — Formula parser now supports: Extends (composition), Compose,
  Advice/Pointcuts (AOP), Squash (completion behavior), Gate (conditional
  step execution), Preset (leg selection). Previously silently discarded.
- [ ] **330664c2** — GatesParallel=true by default: typecheck, lint, build,
  test run concurrently in merge queue (~2x gate speedup).
- **Action:** Document available formula features. Consider adding `[gate]`
  sections to formula steps where conditional execution is needed.

---

## 11. ZFC Fixes (Zero Framework Cognition)

Go code making decisions that belong in prompts — moved to prompts.

- [ ] **915f1b7e + f61ff0ac** — Remove auto-close of permanent issues from
  wisp reaper. Reaper only operates on ephemeral wisps.
- [ ] **977953d8** — Witness handlers report data, don't make policy decisions.
- [ ] **3bf979db** — Remove hardcoded role names from witness error messages.
- [ ] **383945fb** — Remove boot triage decision engine from Go.
- [ ] **89ccc218** — Doctor dog: advisory recommendations, not automated actions.
- [ ] **eb530d85** — Restart tracker crash-loop params configurable via
  `patrols.restart_tracker`.
- **Action:** Ensure all moved decisions are reflected in role prompts.
  Verify no hardcoded role names in examples/gastown.

---

## 12. Configuration / Operational

### 12a. Per-role config
- [ ] **bd8df1e8** — Dog recognized as role in AgentEnv(). Supports
  `role_agents.dog` overrides.
- [ ] **e060349b** — `worker_agents` map for per-crew-member agent overrides.
  Resolution: worker_agents[name] > role_agents[crew] > defaults.
- [ ] **2484936a** — Role registry: `autonomous` and `emoji` properties per role.
- **Action:** Document `worker_agents` and `role_agents` config in city.toml.

### 12b. Rig lifecycle
- [ ] **95eff925** — `auto_start_on_boot` per-rig config. Sling blocked for
  docked rigs. `IsRigParkedOrDocked` enforcement.
- [ ] **d2350f27** — Polecat pool: `gt polecat pool-init <rig>`,
  `polecat_pool_size` config, local branch cleanup in done.
- **Action:** Document rig lifecycle config options.

### 12c. Operational thresholds (ZFC)
- [ ] **3c1a9182 + 8325ebff** — OperationalConfig: 30+ hardcoded thresholds
  now configurable via config sub-sections (session, nudge, daemon, deacon,
  polecat, dolt, mail, web).
- **Action:** Consider documenting available threshold config.

### 12d. Multi-instance isolation
- [ ] **33362a75** — Per-city tmux sockets via `tmux -L <cityname>`. Prevents
  session name collisions across cities.
- **Action:** SDK-level change. Verify Gas City uses per-city tmux sockets.

### 12e. Misc operational
- [ ] **dab8af94** — `GIT_LFS_SKIP_SMUDGE=1` during worktree add. Reduces
  polecat spawn from ~87s to ~15s.
- [ ] **a4b381de** — Unified rig ops cycle group: witness, refinery, polecats
  share one n/p cycle group.
- [ ] **6ab5046a** — Town-root CLAUDE.md template with operational awareness
  guidance for all agents.
- [ ] **b06df94d** — `--to` flag for mail send. Accepts well-known role addresses.
- [ ] **9a242b6c** — Path references fixed: `~/.gt/` to `$GT_TOWN_ROOT/`.
- **Action:** Add LFS skip to worktree-setup.sh. Check cycle.sh for unified
  cycling. Consider CLAUDE.md template for examples/gastown.

---

## 13. New Formulas (from batch 3)

- [ ] 9 new formula files identified: idea-to-plan pipeline + dog formulas.
- [ ] Witness behavioral fixes: persistent polecat model, swim lane rule.
- [ ] Polecat persist-findings.
- [ ] Settings: `skipDangerousModePermissionPrompt`.
- [ ] Dangerous-command guard hooks.
- **Action:** Review each formula for relevance to examples/gastown topology.

---

## Review Order (Suggested)

1. **Persistent Polecat Pool** (Section 1) — foundational, affects everything
2. **Polecat Work Formula v7** (Section 2) — directly updates a key formula
3. **Communication Hygiene** (Section 3) — affects all role prompts
4. **Batch-then-Bisect MQ** (Section 4) — refinery formula rewrite
5. **Witness Patrol** (Section 6) — many behavioral changes
6. **Prompt Updates** (Section 9) — role-by-role prompt updates
7. **ZFC Fixes** (Section 11) — ensure no Go-level decisions leak in
8. **Infrastructure Dogs** (Section 8) — new formulas to consider
9. **Config/Operational** (Section 12) — SDK-level features
10. **Formula System** (Section 10) — new capabilities
11. Remaining sections as needed
