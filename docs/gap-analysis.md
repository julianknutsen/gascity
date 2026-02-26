# Gap Analysis: Gastown → Gas City

Created 2026-02-26 after deep-diving all gastown packages (`upstream/main`)
and comparing against Gas City's current implementation.

**Purpose:** Decision record for every significant gastown feature that
doesn't have a Gas City parallel. Each item gets a verdict: PORT, DEFER,
or EXCLUDE — with rationale.

**Ground rules:**
- Gas City has ZERO hardcoded roles. Anything role-specific is config.
- The Primitive Test (`docs/primitive-test.md`) applies: Atomicity +
  Bitter Lesson + ZFC.
- "Worth porting" means it's infrastructure that ANY topology needs.
- "Gastown-specific" means it assumes Gas Town's particular role set.

---

## Verdicts

| Verdict | Meaning |
|---------|---------|
| **PORT** | Infrastructure primitive. Should be built. |
| **DEFER** | Useful but not needed until a specific use case arises. |
| **EXCLUDE** | Gastown-specific, fails Primitive Test, or deployment concern. |
| **DONE** | Already implemented in Gas City. |

---

## 1. Session Layer

### 1.1 Startup Beacon — DONE

**Gastown:** `session/startup.go` — Generates identification beacons
that appear in Claude Code's `/resume` picker. Format:
`[GAS TOWN] recipient <- sender • timestamp • topic`. Helps agents
find predecessor sessions after crash/restart.

**Gas City status:** Implemented. `session.FormatBeacon()` generates
`[city-name] agent-name • timestamp` prepended to every agent's
prompt at startup. Non-hook agents (detected via `config.AgentHasHooks`)
also get a "Run `gc prime`" instruction. Wired into both `buildAgents`
and `poolAgents`.


---

### 1.2 PID Tracking — DEFER

**Gastown:** `session/pidtrack.go` — Writes pane PID + process start
time to `.runtime/pids/<session>.pid`. On cleanup, verifies start time
matches before killing (prevents killing recycled PIDs). Defense-in-depth
for when `tmux kill-session` fails or tmux itself dies.

**Why DEFER (not PORT):** Gas City's `KillSessionWithProcesses` already
handles the normal case. PID tracking is a belt-and-suspenders measure
for long-running daemon mode where tmux might crash. Not needed until
the controller runs unattended.

**Violates "No status files"?** Borderline. PID files ARE status files
that go stale on crash. But gastown validates with start-time comparison,
which mitigates staleness. The alternative (scan entire process table
for matching command lines) is fragile. This is one of the few cases
where a status file with validation beats live query.


---

### 1.3 Session Staleness Detection — DONE

**Gastown:** `session/stale.go` — Compares message timestamp against
session creation time. If the message predates the session, it's stale.

**Gas City status:** Sufficient. `Tmux.GetSessionCreatedUnix()` and
`GetSessionInfo()` (which includes `session_created`) already exist.
The comparison logic (`StaleReasonForTimes`) is a trivial timestamp
comparison that any consumer can inline. No dedicated function needed.

---

### 1.4 SetAutoRespawnHook — DEFER

**Gastown:** `tmux.go` — Sets tmux `pane-died` hook:
`sleep 3 && respawn-pane -k && set remain-on-exit on`. The "let it
crash" mechanism — tmux restarts the agent process automatically.

**Why DEFER:** Gas City's controller already handles restarts via
reconciliation. Auto-respawn is a tmux-level fast path that reduces
restart latency from "next reconcile tick" to "3 seconds." Useful for
daemon mode but not needed when a human is running `gc start`.

**Dependencies:** SetRemainOnExit (DONE). `-e` flags survive respawn
(verified — see `startup-roadmap.md`).


---

### 1.5 Prefix Registry — DEFER

**Gastown:** `session/registry.go` — Bidirectional map: beads prefix
↔ rig name. Enables routing bead IDs to the correct rig's `.beads/`
directory. Required for multi-rig orchestration.

**Why DEFER:** Gas City is single-rig today. Multi-rig needs this.


---

### 1.6 Agent Identity Parsing — EXCLUDE

**Gastown:** `session/identity.go` — Parses addresses like
`gastown/crew/max` into `AgentIdentity` structs with role type,
rig, name. Knows about Mayor, Deacon, Witness, etc.

**Why EXCLUDE:** Deeply entangled with gastown's hardcoded role names.
Gas City agents have names and session names — that's sufficient.
Address parsing is a gastown deployment concern.

---

## 2. Beads Layer

### 2.1 Bead Locking (Per-Bead Flock) — PORT

**Gastown:** `beads_agent.go`, `audit.go` — File-based flock per bead
(`.locks/agent-{id}.lock`). Prevents concurrent read-modify-write
races when multiple agents touch the same bead.

**Why PORT:** Any multi-agent topology has concurrent bead access.
Without locking, two agents claiming the same bead is a data race.
This is a fundamental concurrency primitive.

**Gas City status:** None. Needed once multiple agents access the
same beads concurrently.


---

### 2.2 Merge Slot — DEFER

**Gastown:** `beads_merge_slot.go` — Mutex-like bead: one holder at
a time, others queued as waiters. Used to serialize merge operations
so only one agent merges at a time.

**Why DEFER:** The pattern is generic (any serialized resource), but
the only known use case is merge serialization. Build when formulas
need serialized steps.

(formulas & molecules).

---

### 2.3 Handoff Beads (Pinned State) — PORT

**Gastown:** `handoff.go` — Beads with `StatusPinned` that never close.
Represent persistent agent state: "what am I working on right now?"
The hook checks the handoff bead to find current work.

**Why PORT:** This is how `gc hook` works — the agent's hook bead IS
the handoff. Without pinned beads, agents lose track of in-progress
work across session restarts.

**Gas City status:** Partial. Beads have statuses but no explicit
`StatusPinned` or handoff concept. The hook command works differently.


---

### 2.4 Beads Routing — DEFER

**Gastown:** `routes.go` — Routes bead IDs by prefix to different
`.beads/` directories. Enables multi-rig: bead `gt-123` routes to
gastown rig, `bd-456` routes to beads rig.

**Why DEFER:** Single-rig today. Multi-rig needs this.


---

### 2.5 Redirect Handling — DEFER

**Gastown:** `beads_redirect.go` — `.beads/redirect` symlink enables
shared beads across agents. Follows redirect, detects circular refs.

**Why DEFER:** Same as routing — multi-rig concern.


---

### 2.6 Audit Logging — DEFER

**Gastown:** `audit.go` — JSONL audit trail for molecule operations
(detach, burn, squash). Atomic write with per-bead locking.

**Why DEFER:** Only needed when molecules have complex lifecycle
operations (squash, detach). Premature before formulas exist.


---

### 2.7 Molecule Catalog — DEFER

**Gastown:** `catalog.go` — Hierarchical template loading from three
levels (town → rig → project), later overrides earlier. JSONL
serialization, in-memory caching.

**Why DEFER:** Gas City already has `internal/formula` with TOML-based
formulas loaded from config. The hierarchical override pattern becomes
relevant with multi-rig.


---

### 2.8 Custom Bead Types — DEFER

**Gastown:** `beads_types.go` — Registers custom bead types via
`bd config set types.custom` with two-tier caching (in-memory +
sentinel file).

**Why DEFER:** Basic types (task, message) work today. Custom types
matter when formulas create specialized bead types.


---

### 2.9 Escalation Beads — EXCLUDE

**Gastown:** `beads_escalation.go` — Severity levels, ack tracking,
SLA monitoring, re-escalation chains. 260 lines.

**Why EXCLUDE:** Domain pattern, not a primitive. Escalation is a
specific workflow that can be built from beads + labels + formulas.
Fails Atomicity test — it's composed from existing primitives.

---

### 2.10 Channel Beads (Pub/Sub) — EXCLUDE

**Gastown:** `beads_channel.go` — Pub/sub channels with subscriber
lists and retention policies. 460 lines.

**Why EXCLUDE:** Domain pattern. Pub/sub can be composed from beads
(type=channel) + labels (subscribers) + formulas (retention). Adding
it to the SDK would be premature abstraction. If a topology needs
channels, it builds them from beads.

---

### 2.11 Queue Beads — EXCLUDE

**Gastown:** `beads_queue.go` — Persistent work queues with claim
patterns, FIFO/priority ordering, concurrency limits. 380 lines.

**Why EXCLUDE:** Domain pattern. Claim is already `bd update --claim`.
Ordering and concurrency limits are policy that belongs in config/
prompt, not Go code (ZFC violation).

---

### 2.12 Group Beads — EXCLUDE

**Gastown:** `beads_group.go` — Named recipient groups (mailing lists)
with nested membership. 350 lines.

**Why EXCLUDE:** Domain pattern. Groups can be a label on beads or a
config section. Not a primitive.

---

### 2.13 Delegation Tracking — EXCLUDE

**Gastown:** `beads_delegation.go` — Parent→child delegation with
credit cascade and acceptance criteria. 170 lines.

**Why EXCLUDE:** Domain pattern. Delegation is a relationship between
beads expressible via dependencies and labels. Credit tracking is a
gastown-specific concern.

---

## 3. Convoy Layer

### 3.1 Convoy Tracking — DEFER

**Gastown:** `convoy/operations.go` — Batch work coordination: track
issue completion across molecules, reactive feeding (when one issue
closes, dispatch next ready issue). Handles blocking dependencies
and staged states.

**Why DEFER:** Convoys are a derived mechanism (Layer 2-4) that
composes from beads + formulas + event bus. Need formulas first.

**Open design question:** Should convoys be bead metadata, molecule
grouping, or a separate primitive? Needs design work before building.


---

## 4. Formula Layer

### 4.1 Multi-Type Formulas — DEFER

**Gastown:** `formula/types.go` — Four formula types:
- `convoy` — parallel legs + synthesis
- `workflow` — sequential steps with dependencies
- `expansion` — template-based step generation
- `aspect` — multi-aspect parallel analysis

**Gas City status:** Has `workflow` steps only (sequential with
dependencies). No convoy/expansion/aspect types.

**Why DEFER:** Workflow type is sufficient for current use cases.


---

### 4.2 Molecule Step Parsing from Markdown — DEFER

**Gastown:** `beads/molecule.go` — Parses molecule steps from markdown
with `Needs:`, tier hints (haiku/sonnet/opus), `WaitsFor:` gates,
backoff config. Includes cycle detection (DFS).

**Gas City status:** Formulas are TOML. Molecule instantiation creates
child beads but doesn't parse markdown step descriptions.

**Why DEFER:** TOML formulas are working. Markdown parsing is an
alternative authoring format. Not needed until formulas are mature.


---

## 5. Events Layer

### 5.1 Cross-Process Flock on Events — DONE

**Gastown:** Uses `flock` for event file writes.

**Gas City status:** Sufficient. `FileRecorder` uses `O_APPEND` which
provides atomic writes up to `PIPE_BUF` (4096 bytes on Linux) — well
above the size of a single JSON event line. `sync.Mutex` handles
in-process goroutine serialization. Flock would add overhead without
fixing the only theoretical issue (duplicate seq numbers across
processes, which is benign — seq is for ordering, not uniqueness).

---

### 5.2 Visibility Tiers — DEFER

**Gastown:** Events have `audit`, `feed`, or `both` visibility. Audit
events are for debugging; feed events appear in user-facing activity
stream.

**Why DEFER:** Gas City currently logs all events equally. Tiers
matter when there's a user-facing feed with multiple agents.


---

### 5.3 Typed Event Payloads — DEFER

**Gastown:** Structured payloads per event type: `SlingPayload`,
`HookPayload`, `DonePayload`, etc. Enables filtering and querying
by payload fields.

**Gas City status:** Events have a `Message` string field. No
structured payloads.

**Why DEFER:** String messages are sufficient for logging. Structured
payloads matter when code needs to react to specific event fields
(e.g., `events --watch --type=agent.started` filtering by agent name).


---

## 6. Config Layer

### 6.1 Agent Preset Registry — EXCLUDE

**Gastown:** `config/agents.go` — 9 hardcoded agent presets (claude,
gemini, codex, cursor, etc.) with 20+ capability fields each. 500 lines.

**Why EXCLUDE:** Gas City already handles this via `config.Provider`
structs in city.toml. Presets are a convenience that hardcodes provider
knowledge into Go — fails Bitter Lesson (new providers require code
changes). Config-driven provider specs are more flexible.

**Gas City equivalent:** `[providers.<name>]` sections in city.toml
with command, args, env, process_names, ready_prompt_prefix, etc.

---

### 6.2 Cost Tier Management — EXCLUDE

**Gastown:** `config/cost_tier.go` — Standard/economy/budget model
assignment by role (opus for workers, sonnet for patrol, etc.). 237 lines.

**Why EXCLUDE:** Deployment concern. Which model runs which role is a
config decision, not an SDK primitive. A city.toml section can express
`provider = "claude-sonnet"` per agent without any Go code.

---

### 6.3 Overseer Identity Detection — EXCLUDE

**Gastown:** `config/overseer.go` — Detects human operator from git
config, GitHub CLI, environment. 92 lines.

**Why EXCLUDE:** Deployment convenience. Gas City agents know their
operator via config (`[city] owner = "..."`) or environment. Auto-
detection is nice-to-have polish, not infrastructure.

---

### 6.4 Rich Env Generation (AgentEnvConfig) — DEFER

**Gastown:** `config/env.go` — Generates 12+ environment variables
with OTEL context, safety guards (NODE_OPTIONS sanitization,
CLAUDECODE clearance), shell quoting utilities. 389 lines.

**Gas City status:** Env is set via `-e` flags from agent config.
No safety guards or OTEL injection.

**Why DEFER:** Most env vars are set by config today. Safety guards
(NODE_OPTIONS, CLAUDECODE) become relevant when agents spawn child
processes that might interfere.

interfere via environment leakage).

---

## 7. Other Infrastructure

### 7.1 KRC (Knowledge Request Cache) — EXCLUDE

**Gastown:** `krc/` — TTL-based knowledge caching with time decay
and autoprune. 25 KB across 3 files.

**Why EXCLUDE:** Optimization, not infrastructure. Fails Bitter Lesson
— as models get better context windows, caching becomes less important.

---

### 7.2 Telemetry (OpenTelemetry) — EXCLUDE

**Gastown:** `telemetry/` — OTLP export to VictoriaMetrics/Logs.

**Why EXCLUDE:** Deployment concern. Observability integration is
valuable but belongs in the deployment layer, not the SDK. A Gas City
user can add OTEL via agent env vars without SDK support.

---

### 7.3 Feed Curation — EXCLUDE

**Gastown:** `feed/` — Event deduplication and aggregation for
user-facing streams.

**Why EXCLUDE:** UX polish. Can be built as a consumer of the event
log without SDK changes.

---

### 7.4 Checkpoint/Recovery — DEFER

**Gastown:** `checkpoint/` — Save/restore session state for crash
recovery.

**Gas City status:** GUPP + beads already provide crash recovery:
work survives in beads, agent restarts and finds it via hook. Explicit
checkpoints are an optimization.

**Why DEFER:** The bead-based recovery model may make explicit
checkpoints unnecessary. Evaluate when daemon mode is mature.

---

### 7.5 Hooks Lifecycle Management — DEFER

**Gastown:** `hooks/config.go` — Base config + per-role overrides,
6 event types, matcher system, merge logic with field preservation.
665 lines.

**Gas City status:** Simple embedded hook file writer. No
base/override system, no matcher, no event type structure.

**Why DEFER:** Gas City's hook installation (`internal/hooks/`) is
config-driven and works today. The full lifecycle
(base + override + merge + discover) matters when hooks need to
compose from multiple sources.


---

## Summary

### PORT (build)
- Bead locking, handoff beads (pinned state)

### DEFER (build when needed)
- PID tracking, SetAutoRespawnHook, prefix registry, beads routing,
  redirect handling, merge slot, audit logging, molecule catalog,
  convoy tracking, multi-type formulas, molecule step parsing,
  visibility tiers, typed event payloads, custom bead types, rich
  env generation, hooks lifecycle, checkpoint/recovery

### DONE (already sufficient)
- Startup beacon, session staleness detection, cross-process event safety

### EXCLUDE (not SDK concerns)
- Escalation/channel/queue/group/delegation beads, agent preset
  registry, cost tiers, overseer identity, KRC, telemetry, feed
  curation, agent identity parsing
