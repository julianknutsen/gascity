# Gas City: Spec vs Implementation — Design Divergences

**Meeting: Steve + Chris, Feb 24 2026**

This document lists every place where the Gas City implementation made a different design choice than Chris's spec. These are all things we actually built and shipped — not gaps or missing features, but deliberate (or evolved) differences. Each section is a discussion point.

---

## 1. Hook vs Claim — The Naming Divergence

The spec uses Gas Town's "hook" metaphor throughout. We renamed to "claim."

| | Spec | Implementation |
|---|---|---|
| Operation names | `Hook()` / `Unhook()` | `Claim()` / `Unclaim()` |
| Bead status | `open` → `hooked` → `closed` | `open` → `in_progress` → `closed` |
| CLI syntax | `gc bead hook <id> --assignee <name>` | `gc agent claim <agent-name> <bead-id>` |
| Additional status | `pinned` (permanent infrastructure) | Not implemented |

**Rationale:** `Claim/Unclaim` is clearer to users who haven't internalized Gas Town's fishing-hook metaphor. `in_progress` is a more universally understood status than `hooked`. The CLI also inverts the argument order — the agent is the subject acting on the bead, rather than the bead being attached to an agent.

**Discussion:** Which naming should be canonical for the SDK? Is `hook` important to preserve for Gas Town cultural continuity, or is `claim` better for a general-purpose SDK?

---

## 2. CLI Command Namespace

We restructured how commands are organized.

| | Spec | Implementation |
|---|---|---|
| Bead commands | `gc bead create/list/show/close/ready` | `gc bd create/list/show/close/ready` |
| Agent hook | `gc hook <bead-id>` (top-level, agent-context) | `gc agent claim <agent> <bead-id>` (under agent subcommand) |
| Crew management | `gc crew add/list/start` | `gc agent add/list/attach` |
| Nudge | `gc nudge <target> <message>` (top-level) | `gc agent nudge <agent-name> <message>` (under agent) |
| Drain | Not specified | `gc agent drain/undrain/drain-ack/drain-check` |

**Rationale:** `gc bd` exists because it's a transparent proxy to the `bd` CLI (beads_rust binary). `gc agent` groups all agent operations under one namespace rather than scattering them across multiple top-level commands. This keeps the CLI surface smaller and more discoverable.

**Discussion:** Does Chris have opinions on the namespace structure? The spec has many top-level commands (`gc hook`, `gc nudge`, `gc done`, `gc sling`, `gc handoff`, `gc broadcast`) — should the SDK consolidate these under subcommands?

---

## 3. Bead Struct — Different Fields

The bead data model diverged significantly.

**Fields in the spec that we don't have:**

| Spec Field | Type | Purpose |
|---|---|---|
| `Priority` | `int` | Ordering (0 = highest) |
| `Labels` | `[]string` | AND-filtered queries |
| `Pinned` | `bool` | Permanent infrastructure records |
| `Tracks` | `[]string` | Convoy tracking refs |
| `Metadata` | `map[string]string` | Arbitrary key-value pairs |
| `Project` | `string` | Project scoping |
| `ClosedAt` | `*time.Time` | Completion timestamp |

**Fields we added that aren't in the spec:**

| Our Field | Type | Purpose |
|---|---|---|
| `ParentID` | `string` | Step → molecule parent link |
| `Ref` | `string` | Formula step ID or formula name |
| `Needs` | `[]string` | Dependency step refs (on every bead) |
| `From` | `string` | Message sender (for mail) |
| `Description` | `string` | Step instructions or message body |

**Key structural difference:** The spec keeps beads flat and uses `Metadata` for extensibility. We embed parent-child relationships and formula references directly on every bead, which makes molecule/step queries simpler but couples the bead struct to the formula system.

**Discussion:** Should we add Priority and Labels? Are they needed for Tutorial 01-02, or are they Level 3+ features? The `Metadata` map is the biggest philosophical difference — the spec uses it as a generic extension point, while we used typed fields.

---

## 4. Store Interface — Different Method Signatures

| Spec (`TaskBackend`) | Our Implementation (`Store`) |
|---|---|
| `List(filter TaskFilter) ([]Bead, error)` | `List() ([]Bead, error)` — no filtering |
| `Get(id string) (Bead, error)` | `Get(id string) (Bead, error)` — same |
| `Create(bead Bead) (string, error)` — returns ID | `Create(b Bead) (Bead, error)` — returns full bead |
| `Update(id string, updates BeadUpdates) error` | `Update(id string, opts UpdateOpts) error` — limited to Description |
| `Hook(beadID, agentID string) error` | `Claim(id, assignee string) error` |
| `Unhook(beadID, agentID string) error` | `Unclaim(id, assignee string) error` |
| `Close(beadID, agentID string) error` | `Close(id string) error` — no assignee check |
| `Pin(beadID string) error` | Not implemented |
| `Ready(filter TaskFilter) ([]Bead, error)` | `Ready() ([]Bead, error)` — no filtering |
| Not in spec | `Claimed(assignee string) (Bead, error)` |
| Not in spec | `Children(parentID string) ([]Bead, error)` |

**Notable:** The spec's `Close()` takes an agentID and verifies the caller is the assignee. Ours doesn't — anyone can close any bead. The spec's `List()` and `Ready()` accept filters; ours don't.

**Discussion:** Should `Close()` enforce assignee? The spec's `TaskFilter` (status, type, project, labels) is richer — do we need it now or is it a later tutorial feature?

---

## 5. Event Structure

| | Spec | Implementation |
|---|---|---|
| Payload | `Payload map[string]interface{}` (rich, nested) | `Subject string` + `Message string` (flat) |
| Visibility | `Visibility string` ("internal"/"external") | Not implemented |
| Timestamp key | `Timestamp` | `Ts` (shorter JSON) |
| Delivery | Tiered: bounded queues + overflow files + ring buffer | Simple append-only JSONL |
| Subscribers | Critical (bounded) vs Optional (fire-and-forget) | Single Recorder interface |

**Spec event types we don't have:**
`agent.stalled`, `agent.crashed`, `agent.quarantined`, `health.ping_ok`, `health.ping_fail`, `health.restart`, `nudge.sent`, `molecule.completed`, `pool.scale_up`, `pool.scale_down`, `convoy.created`, `convoy.closed`, `plugin.fired`, `plugin.failed`

**Event types we added that aren't in spec:**
`bead.unclaimed`, `agent.draining`, `agent.undrained`, `controller.started`, `controller.stopped`

**Discussion:** The flat Subject+Message model has been sufficient so far. When do we need the rich Payload? Is Visibility needed, or is that Level 6+ complexity?

---

## 6. Session/Agent Provider — Completely Different Architecture

This is the largest architectural divergence.

**Spec: `AgentProvider` — context-aware, handle-based, 12+ methods:**
```
Start(ctx, config) → (AgentHandle, error)
Stop(handle, graceful) → error
Restart(handle) → error
IsRunning(handle) → bool
SendPrompt(handle, Prompt) → error
ReadOutput(handle) → (string, error)
GetState(handle) → (AgentState, error)
Ping(handle) → (PingResult, error)
SupportsAttach() → bool
Attach(handle) / Detach(handle) → error
```

Plus separate `SandboxProvider` and `Adopter` interfaces.

**Our implementation: `session.Provider` — name-based, simpler, 10 methods:**
```
Start(name, Config) → error
Stop(name) → error
IsRunning(name) → bool
Attach(name) → error
ProcessAlive(name, processNames) → bool
Nudge(name, message) → error
SetMeta(name, key, value) → error
GetMeta(name, key) → (string, error)
RemoveMeta(name, key) → error
ListRunning(prefix) → ([]string, error)
```

**Key differences:**

- **No context.Context** — we use session names as handles, not UUID-bearing structs
- **No Ping/GetState/ReadOutput** — health monitoring not yet needed
- **No AgentHandle** — no UUID, PID, StartedAt, or Metadata on the handle
- **No SandboxProvider** — worktree creation is inline in start command
- **No Adopter** — crash recovery works through reconciliation, not explicit adoption
- **We added SetMeta/GetMeta/RemoveMeta** — per-session metadata for drain signals and config hashes (the spec doesn't have this abstraction)
- **We have ProcessAlive()** — zombie detection by checking process tree (the spec uses Ping() for this)

**Also different:** We have a separate `agent.Agent` interface that wraps `session.Provider`:
```
Name() → string
SessionName() → string
IsRunning() → bool
Start() / Stop() / Attach() → error
Nudge(message) → error
SessionConfig() → session.Config
```

The spec doesn't have this two-layer abstraction (Agent wrapping Provider). The spec's AgentProvider IS the agent.

**Discussion:** The spec's architecture is richer but more complex. Our two-layer approach (Agent = identity + Provider = transport) has worked well. Is there value in adding `Ping()` and `GetState()` now, or should that wait for Tutorial 05b (health patrol)?

---

## 7. Config Structure

### Projects vs Rigs

| Spec | Implementation |
|---|---|
| `[projects.<name>]` with `repo`, `branch` | `[[rigs]]` array with `name`, `path`, `prefix` |

The spec uses named tables (`[projects.gastown]`); we use an array of tables (`[[rigs]]`). We added `prefix` for beads routing (not in spec). The spec has `repo` (URL) and `branch`; we have `path` (local filesystem).

### Task/Beads Backend

| Spec | Implementation |
|---|---|
| `[tasks]` with `backend = "beads"` | `[beads]` with `provider = "bd"` or `"file"` |
| `[tasks.beads]` with `data_dir` | Implicit (`.beads/` or `.gc/beads.json`) |

### Agent Configuration

| Spec Field | Our Equivalent |
|---|---|
| `role` | Not implemented (roles are pure config) |
| `scope` ("workspace"/"project") | Not implemented |
| `ephemeral` | Not implemented |
| `depends_on` | Not implemented |
| `[agents.session]` (pattern, work_dir, needs_pre_sync) | Flat fields: `dir`, `isolation` |
| `[agents.loop]` | Not implemented |
| `[agents.health]` | Not implemented |
| `[agents.lifecycle]` | Not implemented |
| `[agents.resume]` | Not implemented |
| `[agents.env]` | `env` (flat map on agent) — same |
| `[agents.pool]` with themes, names, idle_timeout | `[agents.pool]` with `check`, `drain_timeout` |

### Sections We Added (Not in Spec)

| Section | Purpose |
|---|---|
| `[providers.<name>]` | Reusable provider presets (command, args, env, prompt_mode) |
| `[dolt]` | Dolt SQL server config (port, host) |
| `[formulas]` | Formula directory path |
| `[daemon]` | Patrol interval |

### Per-Agent Fields We Added (Not in Spec)

`start_command`, `args`, `prompt_mode`, `prompt_flag`, `ready_delay_ms`, `ready_prompt_prefix`, `process_names`, `emits_permission_warning`

These exist because we implemented provider resolution — detecting which AI coding tool is available and configuring its startup flags. The spec assumes this is handled by the provider implementation, not the config.

**Discussion:** The `[providers]` system and per-agent startup hints are the biggest additions. They solve a real problem (7+ different AI tools with different flags, prompt modes, and readiness detection). Should the spec adopt this pattern?

---

## 8. Pool Scaling — Completely Different Model

**Spec model (themed, identity-based):**
- Named pools with themes: `mad-max` (Toast, Furiosa, Nux...), `minerals`, `wasteland`
- 50 names per theme, overflow to `{project}-51`, etc.
- `idle_timeout` — auto-destroy idle agents
- `max_before_overflow` — when to switch to numeric naming
- `ephemeral = true` required
- Theme selected by hash of project name

**Our model (elastic, command-based):**
- Numeric suffixes: `worker-1`, `worker-2`, `worker-3`
- `check` — shell command that returns desired count (e.g., `bd ready --unassigned --limit 0 --json | jq length`)
- `drain_timeout` — graceful shutdown with ack protocol
- Elastic: re-evaluated each controller tick
- Not necessarily ephemeral

**Discussion:** The `check` command approach is more ZFC-compliant — the Go code doesn't decide when to scale; a shell command does. Themed names are Gas Town flavor. For a general-purpose SDK, are numeric suffixes the right default? Should we support themed names as an option?

---

## 9. Session Naming Convention

| | Spec | Implementation |
|---|---|---|
| Pattern | `gc-{project}-{name}` | `gc-{cityName}-{agentName}` |
| Scope | Per-project | Per-city (workspace) |
| Example | `gc-gastown-toast` | `gc-bright-lights-worker-1` |

The spec scopes sessions to projects. We scope to the city (workspace). In the spec, the same agent name in different projects gets different sessions. In ours, agent names must be unique across the entire city.

**Discussion:** Per-project scoping supports multi-project cities better (same role name in different projects). Per-city scoping is simpler. Which matters more for the SDK?

---

## 10. Provider Auto-Detection Order

| Spec | Implementation |
|---|---|
| claude → codex → gemini → opencode → cursor-agent → auggie → amp → subprocess | claude → codex → gemini → cursor → copilot → amp → opencode |

We include `copilot` (not in Chris's spec) and exclude `auggie`. The ordering of later providers differs. We also have detailed per-provider configuration (startup hints, ready detection, process names) that the spec doesn't specify.

**Discussion:** Should the canonical list include both copilot and auggie? Is the ordering important or just a default?

---

## 11. Error Sentinels

| Spec | Implementation |
|---|---|
| `ErrNotSupported` | — |
| `ErrNotFound` | `ErrNotFound` |
| `ErrConflict` | `ErrAlreadyClaimed` |
| `ErrInvalidState` | — |
| `ErrNotAssignee` | (reuses `ErrAlreadyClaimed`) |
| `ErrTemporarilyUnavailable` | — |

We collapsed the error space from 6 sentinel errors to 2. Notably, the spec distinguishes "conflict" (two agents racing to claim) from "not assignee" (wrong agent trying to unclaim). We use a single `ErrAlreadyClaimed` for both.

**Discussion:** Do we need finer-grained errors? The spec's `ErrTemporarilyUnavailable` (retry semantics) and `ErrInvalidState` (state machine violations) could be useful.

---

## 12. Reconciliation vs Startup Sequencer

**Spec:** Dependency DAG via `depends_on`. Agents start in topological order, parallel within tiers. If any agent fails to start, all previously started agents stop in reverse order (rollback). Formal property: `IsRunning(a) = true` before `Start(b)` when `a` is in `b.depends_on`.

**Our implementation:** Flat reconciliation loop. No `depends_on`. All agents start in config order. Failed agents are skipped; others continue. Config drift detection via SHA-256 hash comparison — agents whose config changed are stopped and restarted. Orphan sessions (running but not in config) are killed.

**What we have that the spec doesn't:** Config drift detection, orphan cleanup, drain protocol for pool scale-down.

**Discussion:** Is `depends_on` ordering important for Tutorial 01-03, or is it a Level 4+ feature? Our reconciliation approach is more NDI-aligned (convergent, idempotent) while the spec's is more OTP-aligned (ordered, with rollback).

---

## 13. Things We Built That Aren't in the Spec

| Feature | What It Does |
|---|---|
| **Drain protocol** | `GC_DRAIN` / `GC_DRAIN_ACK` metadata keys for graceful pool scale-down |
| **Config drift detection** | SHA-256 fingerprint of session config; restart on change |
| **Provider presets** | `[providers.<name>]` — reusable provider configs with inheritance |
| **Provider resolution chain** | agent → workspace → auto-detect, with per-agent overrides |
| **Startup hints** | `ready_delay_ms`, `ready_prompt_prefix`, `process_names`, `emits_permission_warning` |
| **Beads routing** | `routes.jsonl` for cross-rig bead store discovery |
| **Beads redirect** | `.beads/redirect` file in worktrees pointing to main repo's store |
| **Dolt integration** | SQL server lifecycle management for beads backend |
| **Formula validation** | Cycle detection (Kahn's algorithm), duplicate step IDs, dangling refs |
| **Molecule attachment** | `gc mol create <formula> --on <bead-id>` — attach molecule to existing bead |
| **Controller mode** | `gc start --controller` — persistent reconciliation loop with flock + fsnotify |
| **Session metadata** | `SetMeta/GetMeta/RemoveMeta` on Provider for drain signals and config hashes |
| **TOON format** | Token-optimized output format for AI-friendly bead display |

---

## Suggested Discussion Priorities

1. **Hook vs Claim naming** — Quick decision, big downstream impact
2. **Config structure** — `[providers]` system and agent startup hints are novel; worth aligning
3. **Pool model** — Fundamentally different philosophy (themed identity vs elastic check command)
4. **Provider architecture** — Two-layer (Agent + Provider) vs single interface
5. **Session naming scope** — Per-project vs per-city
6. **What to backport to spec** — Drain protocol, config drift, provider presets
