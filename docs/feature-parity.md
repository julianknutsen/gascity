# Feature Parity: Gas Town → Gas City

Created 2026-02-27 from exhaustive exploration of gastown `upstream/main`
(92 top-level commands, 425+ subcommands, 180 command files) compared
against gascity `main` (23 top-level commands, ~60 implementation files).

**Goal:** 100% feature parity. Every gastown feature gets a Gas City
equivalent — either via a direct port, a configuration-driven
alternative, or a deliberate architectural decision to handle it
differently.

**Key constraint:** Gas City has ZERO hardcoded roles. Every gastown
command that references mayor/deacon/witness/refinery/polecat/crew must
become role-agnostic infrastructure that any topology can use.

---

## Status Legend

| Status | Meaning |
|--------|---------|
| **DONE** | Fully implemented in Gas City |
| **PARTIAL** | Core exists, missing subcommands or capabilities |
| **TODO** | Not yet implemented, needed for parity |
| **REMAP** | Gastown-specific; handled differently in Gas City by design |
| **N/A** | Deployment/polish concern, not SDK scope |

---

## 1. City/Town Lifecycle

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt install [path]` | `gc init [path]` | **DONE** | Auto-init on `gc start` too |
| `gt start [path]` | `gc start [path]` | **DONE** | One-shot + controller modes |
| `gt down` / `gt stop` | `gc stop [path]` | **DONE** | Graceful shutdown + orphan cleanup |
| `gt up` | `gc start` | **DONE** | gt up is idempotent boot; gc start one-shot reconcile is equivalent |
| `gt shutdown` | `gc stop --clean` | **N/A** | WONTFIX: `gc stop --clean` covers it. Graceful handoff wait and uncommitted work protection are domain-layer concerns for the topology config. |
| `gt restart` | `gc restart [path]` | **DONE** | Stop then start |
| `gt status` | `gc status [path]` | **DONE** | City-wide overview: controller, suspended state, all agents/pools, rigs, summary count. |
| `gt enable` / `gt disable` | `gc suspend` / `gc resume` | **DONE** | City-level suspend: hook injection becomes no-op. Also supports `GC_SUSPENDED=1` env override. |
| `gt version` | `gc version` | **DONE** | |
| `gt info` | — | **N/A** | Whats-new splash; polish |
| `gt stale` | — | **N/A** | Binary staleness check; polish |
| `gt uninstall` | — | **N/A** | Deployment concern |
| `gt git-init` | — | **REMAP** | `gc init` handles city setup; git init is user's job |
| `gt thanks` | — | **N/A** | Credits page; polish |

---

## 2. Daemon / Controller

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt daemon run` | `gc daemon run` | **DONE** | Foreground + log tee |
| `gt daemon start` | `gc daemon start` | **DONE** | Background fork |
| `gt daemon stop` | `gc daemon stop` | **DONE** | Socket shutdown |
| `gt daemon status` | `gc daemon status` | **DONE** | PID + uptime |
| `gt daemon logs` | `gc daemon logs` | **DONE** | Tail log file |
| `gt daemon enable-supervisor` | `gc daemon install` / `uninstall` | **DONE** | launchd + systemd |
| Controller flock | Controller flock | **DONE** | `acquireControllerLock` |
| Controller socket IPC | Controller socket IPC | **DONE** | Unix socket + "stop" command |
| Reconciliation loop | Reconciliation loop | **DONE** | Tick-based with fsnotify |
| Config hot-reload | Config hot-reload | **DONE** | Debounced, validates before apply |
| Crash tracking + backoff | Crash tracking + backoff | **DONE** | `crashTracker` with window |
| Idle timeout enforcement | Idle timeout enforcement | **DONE** | `idleTracker` per agent |
| Graceful shutdown dance | Graceful shutdown | **DONE** | Interrupt → wait → kill |
| PID file write/cleanup | PID file write/cleanup | **DONE** | In `runController` |
| Dolt health check ticker | Dolt `EnsureRunning` | **PARTIAL** | Gas City only checks on start. Gastown has a separate periodic ticker (default 30s) that detects crashes and restarts with backoff. |
| Dolt remotes patrol | — | **TODO** | Periodic `dolt push` to configured remotes (gastown default: 15min). Stages, commits, pushes each database. |
| Feed curator | — | **REMAP** | Gastown tails events.jsonl, deduplicates, aggregates, writes curated feed.jsonl. Gas City's tick-based reconciler covers recovery; curated feed is UX polish. |
| Convoy manager (event polling) | — | **TODO** | Gastown: event-driven (5s poll for close events across all stores) + periodic stranded scan (30s). Auto-feeds next ready issue, auto-closes empty convoys. |
| KRC pruner | — | **N/A** | No KRC in Gas City |

---

## 3. Agent Management

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt agents list` | `gc agent list` | **DONE** | Lists agents with pool/suspend annotations |
| `gt agents menu` | — | **N/A** | Interactive picker; UX polish |
| `gt agents check` | `gc doctor` | **DONE** | Agent health in doctor checks |
| `gt agents fix` | `gc doctor --fix` | **DONE** | |
| Agent start (spawn) | Reconciler auto-starts | **REMAP** | No `gc agent start`; reconciler spawns agents on tick. `gc agent attach` idempotently starts+attaches. |
| Agent stop (graceful) | `gc agent drain` / `gc agent suspend` | **REMAP** | No `gc agent stop`; drain stops gracefully with timeout, suspend prevents restart. |
| Agent kill | `gc agent kill <name>` | **DONE** | Force-kill; reconciler restarts on next tick |
| Agent attach | `gc agent attach <name>` | **DONE** | Interactive terminal; starts session if not running |
| Agent status | `gc agent status <name>` | **DONE** | Shows session, running, suspended, draining state |
| Agent peek | `gc agent peek [name]` | **DONE** | Scrollback capture with --lines |
| Agent drain | `gc agent drain <name>` | **DONE** | Pool drain with timeout + drain-ack + drain-check + undrain |
| Agent suspend | `gc agent suspend <name>` | **DONE** | Prevent reconciler spawn (sets `suspended=true` in city.toml) |
| Agent resume | `gc agent resume <name>` | **DONE** | Re-enable spawning (clears `suspended`) |
| Agent nudge | `gc agent nudge <name> <msg>` | **DONE** | Send input to running session via tmux send-keys |
| Agent add (runtime) | `gc agent add --name <name>` | **DONE** | Add agent to city.toml (supports --prompt-template, --dir, --suspended) |
| Agent request-restart | `gc agent request-restart <name>` | **DONE** | Signal agent to restart on next hook check |
| Session cycling (`gt cycle`) | — | **TODO** | `gc agent cycle next/prev` |
| Session restart with handoff | — | **TODO** | Kill session, respawn with context |
| `gt seance` | — | **TODO** | Predecessor session forking: list recent sessions, `--talk <id>` spawns `claude --fork-session --resume <id>`. Enables knowledge transfer between sessions. |
| `gt cleanup` | `gc doctor --fix` | **DONE** | Zombie/orphan cleanup |
| `gt shell install/remove` | — | **N/A** | Shell integration; deployment |

---

## 4. Pool / Polecat Management

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| Pool min/max scaling | Pool min/max/check | **DONE** | Elastic pool with check command |
| Pool drain with timeout | Pool drain with timeout | **DONE** | `drainOps` in reconciler |
| Polecat spawn (worktree) | Worktree isolation | **DONE** | `isolation = "worktree"` |
| Polecat name pool | — | **REMAP** | Gas City uses `{name}-{N}` numeric; names are config |
| `gt polecat list` | `gc agent list` | **DONE** | Pool instances shown with annotations |
| `gt polecat add/remove` | Config-driven | **REMAP** | Edit city.toml pool.max |
| `gt polecat status` | `gc agent status` | **DONE** | Per-instance |
| `gt polecat nuke` | `gc agent kill + gc stop --clean` | **DONE** | Kill + worktree cleanup |
| `gt polecat gc` | `gc doctor --fix` | **DONE** | Stale worktree cleanup |
| `gt polecat stale/prune` | Reconciler | **DONE** | Orphan detection in reconciler |
| `gt polecat identity` | — | **REMAP** | No identity system; agents are config |
| `gt namepool add/reset/set/themes` | — | **REMAP** | No name pool; numeric naming |
| `gt prune-branches` | — | **TODO** | Clean stale worktree branches |
| Polecat git-state check | — | **TODO** | Pre-nuke safety: uncommitted work check |
| Dolt branch isolation | — | **TODO** | Per-agent dolt branch for write isolation |

---

## 5. Crew Management

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt crew add/remove` | Config-driven | **REMAP** | Add `[[agents]]` to city.toml |
| `gt crew list` | `gc agent list` | **DONE** | |
| `gt crew start/stop` | Reconciler / `gc agent suspend+resume` | **REMAP** | No individual start/stop; reconciler auto-starts, suspend prevents restart |
| `gt crew restart` | `gc agent kill` (reconciler restarts) | **DONE** | |
| `gt crew status` | `gc agent status` | **DONE** | |
| `gt crew at <name>` | `gc agent attach <name>` | **DONE** | |
| `gt crew refresh` | — | **TODO** | Context cycle: mail-to-self handoff, then restart session with fresh context |
| `gt crew pristine` | — | **TODO** | Sync crew workspaces with remote (git pull/reset to clean state) |
| `gt crew next/prev` | — | **TODO** | Cycle between crew sessions |
| `gt crew rename` | Config-driven | **REMAP** | Edit city.toml |

---

## 6. Work Management (Beads)

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt show <bead-id>` | `bd show <id>` | **REMAP** | Delegates to bd CLI directly |
| `gt cat <bead-id>` | `bd show <id>` | **REMAP** | Same |
| `gt close [bead-id...]` | `bd close <id>` | **REMAP** | Delegates to bd |
| `gt done` | — | **REMAP** | Inlined to prompt: `git push` + `bd create --type=merge-request` + `bd close` + exit. No SDK command needed. |
| `gt release <issue-id>` | — | **TODO** | Release stuck in_progress back to pending |
| `gt ready` | `gc hook` (work_query) | **DONE** | Shows available work |
| Bead CRUD | Bead CRUD | **DONE** | FileStore + BdStore + MemStore |
| Bead dependencies | Bead dependencies | **DONE** | Needs field + Ready() query |
| Bead labels | Bead labels | **DONE** | Labels field |
| Bead types (custom) | — | **TODO** | Register custom bd types (message, agent, molecule, etc.) |
| Agent beads (registration) | — | **REMAP** | Just bd: `bd create --type=agent` + `bd update --label`. No SDK command needed. |
| Agent state tracking | — | **REMAP** | Just bd labels: `idle:N`, `backoff-until:TIMESTAMP`. Liveness = bead last-updated. |
| Bead slots (hook column) | — | **N/A** | WONTFIX: Gas City doesn't use hooked beads. Users can implement via bd labels if needed. |
| Merge request beads | — | **TODO** | MR fields: branch, target, worker, merge_commit, etc. |
| Cross-rig bead routing | Routes file | **DONE** | `routes.jsonl` for multi-rig |
| Beads redirect | Beads redirect | **DONE** | `setupBeadsRedirect` for worktrees |
| `gt audit` | `gc events --type` | **PARTIAL** | Events cover audit; no per-actor query |
| `gt migrate-bead-labels` | — | **N/A** | Migration tool; one-time |

---

## 7. Hook & Dispatch

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt hook` (show/attach/detach/clear) | `gc hook` | **DONE** | Has work_query. Attach/detach/clear are N/A — Gas City doesn't use hooked beads; users can implement via bd if needed. |
| `gt sling <bead> [target]` | `gc sling <target> <bead>` | **DONE** | Routes + nudges |
| `gt unsling` / `gt unhook` | — | **N/A** | WONTFIX: Gas City doesn't use hooked beads. Users can `bd update --hook=""` if needed. |
| Sling to self | — | **TODO** | `gc sling $GC_AGENT <bead>` should self-assign |
| Sling batch (multiple beads) | — | **TODO** | `gc sling <target> bead1 bead2 bead3` |
| Sling with formula instantiation | `gc sling --formula` | **DONE** | Creates wisp molecule |
| Sling idempotency | — | **TODO** | Skip if bead already assigned to target |
| Sling --args (natural language) | — | **TODO** | Store instructions on bead, show via gc prime |
| Sling --merge strategy | — | **TODO** | direct/mr/local merge strategy |
| Sling --account | — | **REMAP** | Provider accounts are config, not per-sling |
| Sling --agent override | — | **REMAP** | Provider is config |
| `gt handoff` | — | **TODO** | Hand off to fresh session with context |
| `gt broadcast` | — | **TODO** | Nudge all agents in city/rig |
| `gt nudge <target> [msg]` | `gc agent nudge <name> <msg>` | **DONE** | Direct message injection via tmux send-keys |

---

## 8. Mail / Messaging

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt mail send` | `gc mail send` | **DONE** | Creates message bead |
| `gt mail inbox` | `gc mail inbox` | **DONE** | Lists unread |
| `gt mail read` | `gc mail read` | **DONE** | Read + close |
| `gt mail peek` | — | **TODO** | Preview first unread |
| `gt mail delete` | — | **TODO** | Delete messages |
| `gt mail archive` | `gc mail archive` | **DONE** | |
| `gt mail mark-read/mark-unread` | — | **TODO** | Read state without closing |
| `gt mail check` | `gc mail check` | **DONE** | Count unread |
| `gt mail search` | — | **TODO** | Full-text search |
| `gt mail thread` / `gt mail reply` | — | **TODO** | Conversation threading |
| `gt mail claim/release` | — | **TODO** | Queue claim semantics |
| `gt mail clear` | — | **TODO** | Clear inbox |
| `gt mail hook` | — | **TODO** | Check mail from agent hook |
| `gt mail announces` | — | **REMAP** | No channels; direct addressing sufficient |
| `gt mail channel` | — | **REMAP** | Pub/sub channels; domain pattern |
| `gt mail queue` | — | **REMAP** | Claim queues; domain pattern |
| `gt mail group` | — | **REMAP** | Mailing lists; domain pattern |
| `gt mail directory` | — | **N/A** | Directory listing; UX polish |
| `gt mail identity` | — | **REMAP** | Identity is `$GC_AGENT` |
| Mail priority (urgent/high/normal/low) | — | **TODO** | Priority field on message beads |
| Mail type (task/scavenge/notification/reply) | — | **TODO** | Type labels on message beads |
| Mail delivery modes (queue/interrupt) | — | **TODO** | `--notify` flag for session injection |
| Mail threading (thread-id, reply-to) | — | **TODO** | Thread tracking via labels |
| Two-phase delivery (pending → acked) | — | **TODO** | Delivery confirmation labels |
| Mail CC | — | **TODO** | Carbon copy recipients |
| Address resolution (@town, @rig, groups) | — | **TODO** | Routing patterns for group send |

---

## 9. Formulas & Molecules

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| Formula TOML parsing | Formula TOML parsing | **DONE** | `internal/formula` |
| `gc formula list` | `gc formula list` | **DONE** | |
| `gc formula show` | `gc formula show` | **DONE** | |
| `gc formula validate` | — | **TODO** | Validate formula TOML syntax and dependencies |
| `gt formula create` | — | **TODO** | Scaffold new formula from template |
| `gt formula run` | — | **TODO** | Direct formula execution (creates molecule, runs) |
| Formula types: workflow | workflow | **DONE** | Sequential steps with dependencies |
| Formula types: convoy | — | **TODO** | Parallel legs + synthesis |
| Formula types: expansion | — | **TODO** | Template-based step generation |
| Formula types: aspect | — | **TODO** | Multi-aspect parallel analysis |
| Formula variables (--var) | — | **TODO** | Input variable substitution |
| Three-tier resolution (project → city → system) | Two-tier (city + rig) | **PARTIAL** | Missing embedded system formulas |
| Periodic formula dispatch | Config defined | **PARTIAL** | `[[formulas.periodic]]` parsed but dispatch not wired |
| `gt mol status` | — | **REMAP** | Just bd: `bd mol current --for=$GC_AGENT` |
| `gt mol current` | — | **REMAP** | Just bd: `bd mol current` shows steps with "YOU ARE HERE" |
| `gt mol progress` | — | **REMAP** | Just bd: `bd mol current` shows step status indicators |
| `gt mol attach/detach` | — | **REMAP** | Just bd: `bd update $WISP --assignee=$GC_AGENT` / `--assignee=""` |
| `gt mol step done` | — | **REMAP** | Just bd: `bd close <step-id>` auto-advances |
| `gt mol squash` | — | **REMAP** | Just bd: `bd close $MOL_ID` + `bd create --type=digest` |
| `gt mol burn` | — | **REMAP** | Just bd: `bd mol burn <wisp-id> --force` |
| `gt mol attach-from-mail` | — | **REMAP** | Prompt-level: read mail, pour wisp, assign |
| `gt mol await-signal/event` | — | **REMAP** | Just gc: `gc events --watch --type=... --timeout` |
| `gt mol emit-event` | — | **REMAP** | Just gc: `gc event emit ...` |
| Wisp molecules (ephemeral) | Wisp molecules | **DONE** | Ephemeral bead flag |
| `gt compact` | — | **TODO** | TTL-based wisp cleanup |

---

## 10. Convoy (Batch Work)

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt convoy create` | — | **TODO** | Create batch tracking bead |
| `gt convoy add` | — | **TODO** | Add issues to convoy |
| `gt convoy close` | — | **TODO** | Close convoy |
| `gt convoy status` | — | **TODO** | Show progress |
| `gt convoy list` | — | **TODO** | Dashboard view |
| `gt convoy check` | — | **TODO** | Completion check |
| `gt convoy land` | — | **TODO** | Land completed convoy (cleanup) |
| `gt convoy launch` | — | **TODO** | Dispatch convoy work |
| `gt convoy stage` | — | **TODO** | Stage convoy for validation |
| `gt convoy stranded` | — | **TODO** | Find convoys with stuck work |
| Auto-close on completion | — | **TODO** | Event-driven convoy close |
| Reactive feeding | — | **TODO** | Auto-dispatch next ready issue |
| Blocking dependency check | Bead dependencies | **PARTIAL** | Ready() exists; convoy-specific filtering missing |

---

## 11. Merge Queue

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt mq submit` | — | **REMAP** | Just bd: polecat sets `metadata.branch`/`metadata.target` + assigns to refinery |
| `gt mq list` | — | **REMAP** | Just bd: `bd list --assignee=refinery --status=open` |
| `gt mq status` | — | **REMAP** | Just bd: `bd show $WORK --json \| jq '.metadata'` |
| `gt mq retry` | — | **REMAP** | Just bd: refinery rejects back to pool, new polecat picks up |
| `gt mq reject` | — | **REMAP** | Just bd: `bd update --status=open --assignee="" --set-metadata rejection_reason=...` |
| `gt mq next` | — | **REMAP** | Just bd: `bd list --assignee=$GC_AGENT --limit=1` |
| `gt mq integration` | — | **REMAP** | Git workflow + bead metadata; gastown-gc helper territory |
| MR scoring (priority + age + retry) | — | **REMAP** | bd query ordering; prompt-level concern |
| Conflict detection + retry | — | **REMAP** | Pure git in refinery formula; prompt-level |
| MR bead fields (branch, target, etc.) | — | **REMAP** | Just bd metadata: `--set-metadata branch=X target=Y` |

---

## 12. Rig Management

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt rig add` | `gc rig add` | **DONE** | |
| `gt rig list` | `gc rig list` | **DONE** | |
| `gt rig remove` | — | **TODO** | Remove rig from city |
| `gt rig status` | `gc rig status` (via gc status) | **PARTIAL** | Per-rig agent status not separated |
| `gt rig start/stop` | `gc rig suspend/resume` | **DONE** | Different naming, same effect |
| `gt rig restart` | `gc rig restart` | **DONE** | Kill agents, reconciler restarts |
| `gt rig park/unpark` | `gc rig suspend/resume` | **DONE** | |
| `gt rig dock/undock` | — | **REMAP** | Same as suspend/resume |
| `gt rig boot` | `gc start` (auto-boots rigs) | **DONE** | |
| `gt rig shutdown` | `gc stop` | **DONE** | |
| `gt rig config show/set/unset` | — | **TODO** | Per-rig config management |
| `gt rig settings show/set/unset` | — | **TODO** | Per-rig runtime settings |
| `gt rig detect` | — | **TODO** | Auto-detect rigs from git repos |
| `gt rig quick-add` | — | **TODO** | One-command rig setup |
| `gt rig reset` | — | **TODO** | Reset rig to clean state |
| Per-rig agents (witness/refinery) | Rig-scoped agents (`dir = "rig"`) | **DONE** | |
| Rig beads prefix | `rig.prefix` / `EffectivePrefix()` | **DONE** | |
| Fork support (push_url) | — | **TODO** | Separate push URL for forks |

---

## 13. Health Monitoring

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt doctor` | `gc doctor` | **DONE** | Comprehensive health checks |
| `gt doctor --fix` | `gc doctor --fix` | **DONE** | Auto-repair |
| Witness patrol (rig-level) | Reconciler tick | **DONE** | Different mechanism, same outcome |
| Deacon patrol (town-level) | Controller loop | **DONE** | Same |
| Stall detection (30min threshold) | Idle timeout | **DONE** | Configurable per agent |
| GUPP violation detection | — | **TODO** | Detect work-on-hook but no progress |
| Orphaned work detection | Orphan session cleanup | **DONE** | Reconciler phase 2 |
| Zombie detection (tmux alive, process dead) | Doctor zombie check | **DONE** | |
| `gt deacon` (18 subcommands) | — | **REMAP** | Role-specific; controller handles patrol |
| `gt witness` (5 subcommands) | — | **REMAP** | Role-specific; per-agent health in config |
| `gt boot` (deacon watchdog) | — | **REMAP** | Controller IS the watchdog |
| `gt escalate` | — | **TODO** | Escalation system for stuck agents |
| `gt warrant` (death warrants) | — | **REMAP** | Controller handles force-kill decisions |
| Health heartbeat protocol | — | **TODO** | Agent liveness pings with configurable interval |
| `gt patrol` | — | **REMAP** | Patrol is the controller reconcile loop |
| `gt orphans` | Doctor orphan check | **DONE** | |

---

## 14. Hooks (Provider Integration)

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| Hook installation (Claude) | Hook installation (Claude) | **DONE** | `.gc/settings.json` |
| Hook installation (Gemini) | Hook installation (Gemini) | **DONE** | `.gemini/settings.json` |
| Hook installation (OpenCode) | Hook installation (OpenCode) | **DONE** | `.opencode/plugins/gascity.js` |
| Hook installation (Copilot) | Hook installation (Copilot) | **DONE** | `.github/copilot-instructions.md` |
| `gt hooks sync` | — | **TODO** | Regenerate all settings files from config |
| `gt hooks diff` | — | **TODO** | Preview what sync would change |
| `gt hooks base` | — | **TODO** | Edit shared base hook config |
| `gt hooks override <target>` | — | **TODO** | Per-role hook overrides |
| `gt hooks list` | — | **TODO** | Show all managed settings |
| `gt hooks scan` | — | **TODO** | Discover hooks in workspace |
| `gt hooks init` | — | **TODO** | Bootstrap from existing settings |
| `gt hooks registry` | — | **TODO** | Hook marketplace/registry |
| `gt hooks install <id>` | — | **TODO** | Install hook from registry |
| Base + override merge strategy | — | **TODO** | Per-matcher merge semantics |
| 6 hook event types | 4 of 6 implemented | **PARTIAL** | SessionStart, PreCompact, UserPromptSubmit, Stop all installed. Missing: PreToolUse, PostToolUse. |
| Roundtrip-safe settings editing | — | **TODO** | Preserve unknown fields when editing settings.json |

---

## 15. Plugins

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt plugin list` | — | **TODO** | List available plugins |
| `gt plugin show` | — | **TODO** | Show plugin details |
| `gt plugin run` | — | **TODO** | Execute plugin manually |
| `gt plugin history` | — | **TODO** | Show plugin execution history |
| Plugin gate types (cooldown/cron/condition/event/manual) | Config `[[formulas.periodic]]` | **PARTIAL** | Config has gate fields; no execution engine |
| Plugin TOML+markdown format | — | **TODO** | Plugin file parser |
| Plugin tracking (labels, digest) | — | **TODO** | Execution tracking |
| Plugin execution timeout | — | **TODO** | Timeout enforcement |
| Town-level + rig-level plugins | — | **TODO** | Two-tier plugin resolution |

---

## 16. Events & Activity

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt log` | `gc events` | **DONE** | JSONL event log |
| `gt log crash` | `gc events --type=agent.crashed` | **DONE** | |
| `gt feed` | — | **TODO** | Real-time activity feed (curated) |
| `gt activity emit` | `gc event emit` | **DONE** | |
| `gt trail` (recent/recap) | `gc events --since` | **DONE** | |
| `gt trail commits` | — | **TODO** | Git commit activity across agents |
| `gt trail beads` | — | **TODO** | Recent bead activity |
| `gt trail hooks` | — | **TODO** | Recent hook activity |
| Event visibility tiers (audit/feed/both) | — | **TODO** | Two-tier event visibility |
| Structured event payloads | `--payload` JSON | **PARTIAL** | Free-form; no typed builders |
| `gc events --watch` | `gc events --watch` | **DONE** | Block until events arrive |
| `gc events --payload-match` | `gc events --payload-match` | **DONE** | Filter by payload fields |

---

## 17. Config Management

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| Config load (TOML) | Config load (TOML) | **DONE** | city.toml with progressive activation |
| Config composition (includes) | Config composition | **DONE** | Fragment includes + layering |
| Config patches | Config patches | **DONE** | Per-agent overrides |
| Config validation | Config validation | **DONE** | Agent, rig, provider validation |
| Config hot-reload | Config hot-reload | **DONE** | fsnotify + debounce |
| `gt config set/get` | `gc config show` | **PARTIAL** | Show only; no set/get |
| `gt config cost-tier` | — | **REMAP** | Provider per agent is config |
| `gt config default-agent` | — | **REMAP** | `workspace.provider` |
| `gt config agent-email-domain` | — | **REMAP** | Agent env config |
| Remote topology fetch | Remote topology fetch | **DONE** | `gc topology fetch/list` |
| Topology lock file | Topology lock file | **DONE** | `.gc/topology.lock` |
| Config provenance tracking | Config provenance | **DONE** | Which file, which line |
| Config revision hash | Config revision hash | **DONE** | For change detection |
| Config --strict mode | Config --strict mode | **DONE** | Promote warnings to errors |

---

## 18. Prompt Templates

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| Role templates (7 roles) | Prompt templates | **DONE** | Any agent, any template file |
| Message templates (spawn/nudge/escalation/handoff) | — | **TODO** | Template rendering for messages |
| Template functions ({{ cmd }}) | Template functions | **DONE** | {{ cmd }}, {{ session }}, {{ basename }}, etc. |
| Shared template composition | Shared templates | **DONE** | `prompts/shared/` directory |
| Template variables (role data) | Template variables | **DONE** | CityRoot, AgentName, RigName, WorkDir, etc. |
| `gt prime` | `gc prime` | **DONE** | Output agent prompt |
| `gt role show/list/def/env/home/detect` | — | **REMAP** | Roles are config; `gc prime` + `gc config show` |
| Commands provisioning (`.claude/commands/`) | — | **TODO** | Provision role-specific slash commands |
| CLAUDE.md generation | — | **TODO** | Generate agent-specific CLAUDE.md files |

---

## 19. Worktree Isolation

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| Worktree creation (per agent) | Worktree creation | **DONE** | `isolation = "worktree"` |
| Worktree branch naming | Worktree branch naming | **DONE** | `gc-{rig}-{agent}` |
| Worktree cleanup (nuke) | `gc stop --clean` | **DONE** | |
| Worktree submodule init | — | **TODO** | `git submodule init` after worktree add |
| `gt worktree list` | — | **TODO** | List all worktrees across rigs |
| `gt worktree remove` | — | **TODO** | Remove specific worktree |
| Beads redirect in worktree | Beads redirect | **DONE** | Points to shared rig store |
| Formula symlink in worktree | Formula symlink | **DONE** | Materialized in worktree |
| Cross-rig worktrees | — | **TODO** | Worktree in another rig's repo |
| Stale worktree repair (doctor) | Doctor worktree check | **PARTIAL** | Cleanup exists; broken .git pointer repair missing |

---

## 20. Dogs (Cross-Rig Workers)

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt dog add/remove` | — | **REMAP** | Config-driven pool agents scoped to city |
| `gt dog list/status` | `gc agent list` | **REMAP** | City-wide agents shown |
| `gt dog call/dispatch/done/clear` | `gc sling` | **REMAP** | Sling to city-wide agent pool |

---

## 21. Costs & Accounts

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt costs` | — | **N/A** | Deployment analytics |
| `gt costs record/digest/migrate` | — | **N/A** | |
| `gt account list/add/default/status/switch` | — | **N/A** | Claude Code account management |
| `gt quota status/scan/clear/rotate` | — | **N/A** | Account quota rotation |

---

## 22. Dashboard & UI

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt dashboard` | — | **TODO** | Web dashboard for convoy tracking |
| `gt status-line` | — | **TODO** | tmux status line integration |
| `gt theme` | — | **N/A** | tmux theme management |
| `gt dnd` (Do Not Disturb) | — | **N/A** | Notification suppression |
| `gt notify` | — | **N/A** | Notification level |
| `gt issue show/set/clear` | — | **N/A** | Status line issue tracking |

---

## 23. Dolt Integration

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt dolt init` | `dolt.InitCity` | **DONE** | |
| `gt dolt start/stop/status` | `dolt.EnsureRunning/StopCity` | **DONE** | |
| `gt dolt logs` | — | **TODO** | Tail dolt server log |
| `gt dolt sql` | — | **TODO** | Interactive SQL shell |
| `gt dolt init-rig` | `dolt.InitRigBeads` | **DONE** | |
| `gt dolt list` | — | **TODO** | List dolt databases |
| `gt dolt migrate` | — | **N/A** | Schema migration; one-time |
| `gt dolt fix-metadata` | — | **TODO** | Repair metadata.json |
| `gt dolt recover` | — | **TODO** | Recover from corruption |
| `gt dolt cleanup` | — | **TODO** | Remove orphaned databases |
| `gt dolt rollback` | — | **TODO** | Rollback to previous state |
| `gt dolt sync` | — | **TODO** | Push to configured remotes |
| Dolt branch per agent | — | **TODO** | Write isolation branches |

---

## 24. Miscellaneous Gastown Commands

| Gastown | Gas City | Status | Notes |
|---------|----------|--------|-------|
| `gt callbacks process` | — | **REMAP** | Handled by hook system |
| `gt checkpoint write/read/clear` | — | **REMAP** | Beads-based recovery is sufficient |
| `gt commit` | — | **TODO** | Git commit with agent identity (GIT_AUTHOR_NAME from agent) |
| `gt signal stop` | — | **REMAP** | Hook signal; provider-specific |
| `gt tap guard` | — | **REMAP** | PR workflow guard; provider-specific hook |
| `gt town next/prev/cycle` | — | **N/A** | Multi-town switching; deployment |
| `gt wl` (wasteland federation) | — | **N/A** | Cross-town federation; future |
| `gt swarm` (deprecated) | — | **N/A** | Superseded by convoy |
| `gt synthesis` | — | **TODO** | Convoy synthesis step management |
| `gt whoami` | — | **TODO** | Show current agent identity |

---

## Priority Summary

### P0 — Critical for feature parity (blocks gastown-as-gc-config)

These are features that gastown's configuration depends on to function:

1. ~~**Agent nudge**~~ — DONE (`gc agent nudge <name> <msg>`)
2. ~~**`gc done`**~~ — REMAP (inlined to prompt: `git push` + `bd create` + `bd close` + exit)
3. ~~**Agent bead lifecycle**~~ — REMAP (just bd: `bd create --type=agent` + `bd update --label`)
4. ~~**Bead slot (hook) operations**~~ — N/A WONTFIX (no hooked beads; users can use bd)
5. ~~**Unsling/unhook**~~ — N/A WONTFIX (no hooked beads; users can use bd)
6. **Mail enhancements** — delete, mark-read/unread, peek, priority, threading
7. **Molecule lifecycle** — status, current, step-done, attach/detach, squash
8. **Merge queue** — submit, list, next, status (refinery processing)
9. **Convoy tracking** — create, add, close, status, check, reactive feeding
10. **`gc broadcast`** — Nudge all agents
11. **`gc handoff`** — Hand off work to fresh session
12. **Periodic formula dispatch** — Wire the config to the controller loop
13. **GUPP violation detection** — Work on hook but no progress

### P1 — Important for production use

14. ~~**`gc status`**~~ — DONE (`gc status [path]`)
15. **Hooks lifecycle** — sync, diff, base/override merge
16. **Plugin system** — list, show, run, gate evaluation
17. **Event visibility tiers** — audit vs feed
18. **Dashboard** — Web UI for convoy tracking
19. **tmux status line** — `gc status-line` for tmux integration
20. **Dolt management** — logs, sql, sync, recover, cleanup
21. **Rig management** — remove, config, settings, detect, quick-add
22. **Session cycling** — `gc agent cycle next/prev`
23. **Escalation system** — Stuck agent escalation
24. **Stale branch cleanup** — `gc prune-branches`
25. **`gc whoami`** — Show current identity
26. **`gc commit`** — Git commit with agent identity
27. **`gc release`** — Release stuck beads
28. **Address resolution** — @town, @rig group patterns for mail
29. **Commands provisioning** — Provision .claude/commands/ for agents
30. **Cross-rig worktrees** — Agent worktree in another rig's repo
31. **`gt seance`** — Predecessor session forking for knowledge transfer

### P2 — Nice-to-have / polish

32. **Feed curation** — Curated activity stream
33. **Trail subcommands** — commits, beads, hooks activity
34. **Formula types** — convoy, expansion, aspect (workflow exists)
35. **Formula create** — Scaffold from template
36. **Formula variables** — Input variable substitution
37. **Formula validate** — Validate formula TOML syntax and dependencies
38. **Config set/get** — CLI config editing
39. **Agent menu** — Interactive picker
40. **Crew refresh/pristine** — Workspace maintenance
41. **Worktree list/remove** — Worktree management commands
42. **Submodule init** — Post-worktree-add submodule setup
43. **Compact (wisp TTL)** — TTL-based ephemeral bead cleanup

### N/A — Not SDK scope

- Costs/accounts/quota (deployment analytics)
- Themes/DND/notifications (UX polish)
- Town cycling (multi-town deployment)
- Wasteland federation (cross-town)
- Shell integration (deployment)
- Agent presets (config handles this)
- Name pools (numeric naming is sufficient)

---

## Effort Estimates

| Priority | Items | Estimated Lines | Notes |
|----------|-------|-----------------|-------|
| P0 | 8 features | ~2,500-3,500 | Core dispatch + molecule + convoy (nudge + done + agent-bead + hook-slots resolved) |
| P1 | 18 features | ~3,000-4,000 | Hooks lifecycle + plugins + rig mgmt + status + seance |
| P2 | 12 features | ~1,500-2,000 | Formula types + polish commands + validate |
| **Total** | **42 features** | **~7,500-10,000** | |

Current Gas City: ~12,000 lines of Go (excl. tests, docs, generated).
Feature parity target: ~20,000-22,000 lines.
