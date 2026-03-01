# Tutorial Path: Spec vs Implementation

**Meeting: Steve + Chris, Feb 24 2026**
**Lens: Einstein's Razor** — "Everything should be as simple as possible, but not simpler."

Where did the implementation find a simpler path than the spec? Where did the spec's design prove necessary and we'll need to converge? Where did we discover complexity the spec didn't anticipate?

---

## The Two Tutorial Sequences

| # | Chris's Spec | Our Implementation | Status |
|---|---|---|---|
| 00 | Series Overview | (no equivalent) | — |
| 01 | Hello, Gas City | Hello, Gas City | Both exist, diverged |
| 02 | Named Crew | Named Crew | Chris has full tutorial; we have txtar test only |
| 03 | The Ralph Loop | (notes only in Chris's; nothing in ours) | Neither is complete |
| 04 | Agent Team | (listed in progression) | Not written |
| 05 | — | Mail (per roadmap) | We jumped ahead |
| 05a | Formulas | — | Chris has this slot |
| 05b | Health Patrol | (listed in progression) | Not written |
| 05c | Plugins | (listed in progression) | Not written |
| 05d | Full Orchestration | (listed in progression) | Not written |
| 06 | — | Formulas | We renumbered; full tutorial + txtar |
| 07 | — | Attached Molecules | Ours only; full txtar |
| 08 | — | Agent Pools | Ours only; full txtar |
| 09 | — | Scoped Directories | Ours only; config example |
| 10 | — | Multi-Rig Isolation | Ours only; config example |

**Key observation:** Chris's sequence is linear (01→02→03→04→05a-d). Ours jumped to implementation: we built what we needed when we needed it, resulting in a different ordering (01→02→06→07→08 with mail and pools before health patrol).

---

## Tutorial 01: Hello, Gas City — Aligned on the Problem, Diverged on the UX

Both tutorials solve the same problem: *Context window fills up. Agent loses everything. Beads persist work state so a fresh session picks up where the last one left off.*

### Where We Aligned

- The core narrative is identical: create a city, add a project, create a bead, agent works it, bead persists across sessions
- Bead lifecycle starts at `open`, ends at `closed`
- `gc rig add <path>` to register a project — same command, same output structure
- `gc agent attach mayor` — same command
- Bead output formats (ID/Status/Type/Title/Created/Assignee) — same structure
- The "What just happened?" insight: beads survive context loss

### Where We Diverged

**1. City initialization: two commands vs one**

| Chris | Us |
|---|---|
| `gc start ~/bright-lights` does everything (create + boot) | `gc init ~/bright-lights` creates; `gc start` boots |
| `gc init` is a wizard for choosing config tiers | `gc init` is the creation command; `gc start` auto-inits if needed |

Chris's version is simpler for the first-time user — one command. Our version separates concerns (create vs run) which is simpler for the mental model but adds a step. We partially reconciled this: `gc start` auto-initializes if no city exists, so in practice both work as a single command.

**Einstein's Razor says:** Chris's single-command approach is simpler for Tutorial 01. Our separation matters later (scripting, CI, idempotency). Both work.

**2. Bead commands: `gc bead` vs `gc bd`**

| Chris | Us |
|---|---|
| `gc bead create/list/ready/show` | `gc bd create/list/ready/show/close` |

We use `gc bd` because it's a transparent proxy to the `bd` CLI (beads_rust). When `bd` is installed, `gc bd` passes through directly. When using the file backend, `gc bd` implements the same interface in Go.

**Einstein's Razor says:** `gc bead` is more self-documenting. `gc bd` is shorter and reflects the actual tool name. For a tutorial, `gc bead` reads better. For daily use, `gc bd` types faster. This is a naming decision, not an architectural one.

**3. What the rig list shows**

| Chris | Us |
|---|---|
| Just the added rig: `tower-of-hanoi: Agents: [mayor]` | Two entries: `bright-lights (HQ): Prefix: bl` and `tower-of-hanoi: Prefix: toh` |

We show the city itself as an "HQ" rig because beads routing needs a prefix for the city's own bead store. The mayor lives on the city, not on the rig. Chris puts the mayor on the rig.

**Einstein's Razor says:** Chris's is simpler — one rig, one list. Ours reflects the actual architecture (city has its own bead store with its own prefix). This matters for multi-rig routing in Tutorial 10 but is confusing in Tutorial 01.

**4. Explicit close command**

| Chris | Us |
|---|---|
| No `gc bead close` shown — agent just "finishes" | Shows `gc bd close gc-1` explicitly |

We show the close command because it's part of the bead CRUD interface and users/agents need to know it exists. Chris implies it happens automatically.

**5. Config file: `settings.yaml` vs `city.toml`**

Chris's `gc start` output references `settings.yaml`. We use `city.toml`. This is a settled decision (TOML won). Chris's spec elsewhere also uses TOML, so the `settings.yaml` reference is likely stale.

---

## Tutorial 02: Named Crew — The Big Fork

This is where the paths diverge most. Chris wrote a full tutorial. We implemented the capability but with completely different commands and concepts.

### The Problem (Aligned)

Both: *In Tutorial 01, YOU were the router. You manually told the agent to check beads. Named agents with assignment let the mayor route work.*

### Chris's Design

Introduces three new command groups:
- `gc crew add tower-of-hanoi --name builder --agent codex` — register a named crew member on a rig
- `gc crew list tower-of-hanoi` — show crew for a rig
- `gc crew start builder --rig tower-of-hanoi` — start a crew member (separate from `gc agent attach`)
- `gc bead hook gc-2 --assignee builder` — assign a bead to a crew member

Bead lifecycle becomes `open → hooked → closed`. The status `hooked` means "assigned to a specific agent, atomically." The crew member auto-checks its hook on startup.

### Our Design

No crew commands. Instead:
- `gc agent add --name worker` — add an agent to city.toml (not rig-scoped)
- `gc agent list` — list all agents in the city
- `gc agent claim worker gc-1` — assign a bead to an agent
- `gc agent claimed worker` — show what an agent has claimed

Bead lifecycle is `open → in_progress → closed`. The status `in_progress` means "claimed by an agent."

### Side-by-Side

| Concept | Chris | Us |
|---|---|---|
| Adding an agent | `gc crew add <rig> --name <name> --agent <type>` | `gc agent add --name <name>` |
| Agent scope | Per-rig (crew member belongs to a rig) | Per-city (agent belongs to the workspace) |
| Assignment command | `gc bead hook <id> --assignee <name>` | `gc agent claim <agent> <bead-id>` |
| Assignment status | `hooked` | `in_progress` |
| Release command | (not shown, implied `unhook`) | `gc agent unclaim <agent> <bead-id>` |
| Starting an agent | `gc crew start <name> --rig <rig>` | `gc start` starts all agents via reconciliation |
| Agent type specified | Yes: `--agent codex` | No: provider resolved from config |

### What This Means

**Chris's model** has a richer agent identity: agents belong to rigs, have explicit types, and crew management is separate from agent lifecycle. The `hook` metaphor carries Gas Town cultural context.

**Our model** is flatter: agents belong to the city, provider resolution happens at start time from config, and `claim/unclaim` is a simpler metaphor. No separate "crew" concept — agents are agents.

**Einstein's Razor says:** Our model is simpler *for the SDK*. Chris's model is richer *for Gas Town*. The question is: does the SDK need per-rig agent scoping? If cities typically have one rig, our model is sufficient. If multi-rig is common (Tutorial 10), Chris's model handles it more naturally.

**The deepest divergence:** Chris's `gc crew start builder --rig tower-of-hanoi` starts a single named agent. Our `gc start` starts ALL agents via reconciliation. We don't have a "start one agent" command — the reconciler is the only way agents start. This is a fundamental architectural choice: imperative (start this agent) vs declarative (reconcile to desired state).

---

## Tutorial 03: The Ralph Loop — Both Incomplete, Same Direction

Chris has notes (not a full tutorial). We have nothing written but the capability is partially there through the claim system and reconciler.

### The Problem (Aligned)

Both: *Ten beads in the backlog. You don't want to hand-feed tasks one at a time. The agent should drain the queue automatically.*

### Chris's Design

Adds `[agents.loop]` config:
```toml
[agents.loop]
enabled = true
auto_execute = true
poll_interval = "30s"
```

The agent polls: check hook → check mail → check ready queue → claim → execute → repeat. Context survival: if the agent's session dies mid-bead, the bead stays `hooked`, and a fresh session picks it up.

### Our Position

We don't have `[agents.loop]`. The loop behavior lives in the agent's prompt template (ZFC — Go doesn't decide when to poll). The reconciler restarts dead agents. The claim system persists assignments across restarts. So the *effect* is the same, but the *mechanism* is different.

**Einstein's Razor says:** Chris puts the loop in config (infrastructure decides polling interval). We put it in the prompt (agent decides when to check). Both converge to the same behavior. The ZFC principle says the prompt approach is more correct — but `poll_interval` in config is useful for operators who want to tune without editing prompts.

---

## Tutorials We Built That Chris Didn't Spec

### Tutorial 06: Formulas (Chris's 05a)

Both agree formulas exist. We implemented them.

| | Chris's Spec Slot | Our Implementation |
|---|---|---|
| Formula format | `*.formula.toml` with steps + dependencies | Same: `formula`, `description`, `[[steps]]` with `id`, `title`, `description`, `needs` |
| Molecule creation | `gc mol create <formula>` | Same |
| Step completion | (not detailed) | `gc mol step done <mol-id> <step-ref>` |
| Dependency resolution | Steps ready when all `needs` closed | Same (implemented with CurrentStep logic) |
| Cycle detection | (not specified) | Kahn's algorithm in `formula.Validate()` |

**Our addition: Attached Molecules (Tutorial 07)**

`gc mol create cooking --on gc-1` — attaches a molecule to an existing bead. The base bead carries context ("Pancakes: flour, sugar, eggs..."), the molecule carries structure (steps). Commands can address the molecule through the base bead's ID. Status shows both the bead's context and the molecule's progress.

This isn't in Chris's spec at all. It solves: *"I have a task bead with context. I want to apply a formula to it without losing the context."*

### Tutorial 08: Agent Pools

Both specs describe pools. We implemented them differently (see the design divergences doc). The txtar test demonstrates:
- `check = "echo 3"` → starts 3 agents (worker-1, worker-2, worker-3)
- `min` floor enforced even when check returns 0
- Mixed configs: singleton mayor + pooled workers
- `max = 1` uses bare name (no `-1` suffix)

### Mail (Tutorial 05 in our progression)

Chris's spec has `[messaging]` as a config section and mail commands. We implemented mail as beads with `type = "message"`:
- `gc mail send mayor 'hey'` → creates a bead with type "message", assignee "mayor", from "human"
- `gc mail inbox mayor` → lists open messages for mayor
- `gc mail read gc-1` → shows message, marks as read (closes the bead)
- `GC_AGENT` env var determines sender identity

This matches Chris's spec in spirit (mail = beads) but differs in the config approach (no `[messaging]` section — mail just works because beads support type filtering).

**Einstein's Razor says:** No config section for messaging is simpler. Mail is just beads with `type = "message"`. The config section becomes necessary only when you need routing rules, priority tiers, or channels — Level 4+ features.

---

## The Controller / Daemon — Aligned Architecture, Different Scope

Both: persistent process with flock exclusion, Unix socket for CLI communication, config watching.

| | Chris's Spec | Our Implementation |
|---|---|---|
| Start command | `gc start --daemon` | `gc start --controller` |
| Config watching | (implied) | fsnotify on city.toml, atomic reload |
| Health patrol | Full: ping, stuck detection, quarantine, backoff, restart limits | Minimal: restart dead agents, no ping/stall |
| Drain protocol | Not specified | Full: set drain flag → wait for ack → force kill after timeout |
| Config drift | Not specified | SHA-256 fingerprint comparison, auto-restart on change |

**Einstein's Razor says:** Our controller is simpler (no health patrol yet) but has features the spec doesn't (drain protocol, config drift detection). These are orthogonal — the spec's health patrol and our drain/drift detection both need to exist eventually.

---

## Reconciliation Philosophy — The Deepest Difference

This isn't in any single tutorial but underlies everything.

**Chris's spec:** Imperative startup sequencing. `depends_on` creates a DAG. Agents start in topological order. Failure triggers rollback. `gc agent start <name>` starts one agent.

**Our implementation:** Declarative reconciliation. No `depends_on`. `gc start` reconciles ALL agents to desired state. No rollback — failed agents are skipped. The reconciler runs on every controller tick, continuously converging. Orphan detection kills unknown sessions. Config drift detection restarts changed agents.

| Property | Chris | Us |
|---|---|---|
| Start model | Imperative (start this agent) | Declarative (reconcile to desired state) |
| Failure handling | Rollback (stop previously started) | Skip and continue |
| Ordering | Topological from depends_on | Config order (flat) |
| Drift handling | Not specified | Auto-detect via hash, restart |
| Orphan handling | Not specified | Kill sessions not in config |
| Continuous | One-shot start | Controller loop re-reconciles |

**Einstein's Razor says:** Declarative reconciliation is simpler to reason about (one function handles start, restart, drift, and cleanup) but can't express ordering constraints. Chris's DAG ordering is necessary for real deployments (database before app server). We'll likely need both: declarative reconciliation with optional ordering hints.

---

## Summary: Where Einstein's Razor Led Us

| Decision | Simpler? | Trade-off |
|---|---|---|
| `claim` instead of `hook` | Yes — more universal | Loses Gas Town cultural resonance |
| `in_progress` instead of `hooked` | Yes — standard status name | Less precise (doesn't imply atomicity) |
| `gc bd` instead of `gc bead` | Debatable — shorter but less obvious | Reflects actual tool name (bd CLI) |
| No `gc crew` commands | Yes — agents are just agents | Loses per-rig scoping |
| No `[agents.loop]` config | Yes — loop lives in prompt (ZFC) | Operators can't tune poll interval without editing prompts |
| No `depends_on` ordering | Yes — flat reconciliation is simpler | Can't express "start A before B" |
| No `[messaging]` config | Yes — mail is just beads | Can't configure routing/priority without code |
| No `[tasks]` section | Yes — beads are implicit | Less explicit about what backend is in use |
| `[providers]` presets | We added complexity | Solves real problem: 7+ AI tools with different flags |
| `check` command for pools | Simpler + more ZFC | No themed names, no cultural flavor |
| Drain protocol | We added complexity | Necessary for graceful pool scale-down |
| Config drift detection | We added complexity | Necessary for controller mode correctness |
| Attached molecules | We added complexity | Necessary for formula reuse with context |

### Discussion Questions

1. **Should we adopt `hook`/`hooked` or keep `claim`/`in_progress`?** This affects every tutorial and all user-facing docs.

2. **Should we add `gc crew` commands, or is `gc agent` sufficient?** Per-rig scoping matters for multi-project cities.

3. **Should `[agents.loop]` exist in config?** ZFC says no (prompt decides), but operators want tuning knobs.

4. **Should we add `depends_on` ordering?** Our reconciler handles most cases, but "start database before app" is a real need.

5. **What's the canonical tutorial numbering?** Chris has 01-05d (8 tutorials). We have 01-10 (12 slots). Should we align?
