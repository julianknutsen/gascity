# Gas Town Example — Future Work

Tracks gc commands and features referenced in prompts/formulas that
don't exist yet in the gc binary. This file is the gap analysis between
"Gas Town expressed as configuration" and "gc can actually run it."

## Missing gc commands

Commands referenced in prompts and formulas but not yet implemented.
Grouped by priority tier.

### Tier 1: Core Propulsion

These are required for any agent to do useful work.

| Command | Description | Referenced in |
|---------|-------------|---------------|
| `gc hook` | Check hooked work (the fundamental primitive) | All 8 prompts, most formulas |
| `gc sling <bead> <rig>` | Dispatch work to a polecat/dog | mayor, deacon, convoy-feed, orphan-scan, session-gc |
| `gc done` | Submit completed work to merge queue + self-clean | polecat, dog |
| `gc nudge <target> "msg"` | Send message to agent's tmux session | mayor, deacon, witness, crew, refinery, boot-triage |
| `gc polecat list/nuke/status/remove` | Polecat lifecycle management | mayor, witness, refinery |
| `gc session status/start/stop` | Agent session lifecycle | witness, deacon, boot-triage |

### Tier 2: Agent Management

Required before multi-agent orchestration works.

| Command | Description | Referenced in |
|---------|-------------|---------------|
| `gc handoff -s "..." -m "..."` | Cycle to fresh session with context notes | mayor, deacon, witness, refinery, polecat, crew |
| `gc prime` | Load full context after compaction/new session | polecat, all prompts (recovery note) |
| `gc escalate "desc" -s SEVERITY` | File escalation for blockers | polecat |
| `gc mq list/submit/integration` | Merge queue operations | refinery |
| `gc deacon heartbeat/cleanup-orphans/redispatch/zombie-scan` | Daemon patrol executor commands | deacon |
| `gc boot status/spawn/triage` | Daemon watchdog commands | boot |
| `gc dog status/done/clear/list/add/remove` | Infrastructure worker lifecycle | dog, deacon-patrol |
| `gc mayor stop/start` | Mayor agent control | deacon |

### Tier 3: Operational

Important for full Gas Town operation.

| Command | Description | Referenced in |
|---------|-------------|---------------|
| `gc peek <target> [lines]` | View last N lines of agent session | witness, boot-triage |
| `gc feed --since <duration>` | View activity feed | deacon-patrol, boot-triage, digest-generate |
| `gc worktree <rig>` / `list` / `remove` | Cross-rig worktree management | crew |
| `gc convoy list/check/stranded/create/status` | Batch work coordination | deacon-patrol, convoy-feed, convoy-cleanup |
| `gc context --usage` | Check own context usage | deacon-patrol, refinery-patrol |
| `gc rig start/stop/park/dock/unpark/undock/restart/reboot/status` | Full rig lifecycle | deacon, witness, mayor, crew |
| `gc crew stop <name>` | Crew member lifecycle | crew |

### Tier 4: Maintenance

Supporting infrastructure for long-running systems.

| Command | Description | Referenced in |
|---------|-------------|---------------|
| `gc warrant file <target> --reason "..."` | Death warrant for zombie processes | deacon-patrol |
| `gc compact --dry-run/--verbose/report` | Wisp compaction and reporting | deacon-patrol |
| `gc patrol digest --yesterday` | Aggregate daily patrol digests | deacon-patrol |
| `gc doctor -v / --fix` | System health diagnostics | session-gc, deacon-patrol |
| `gc costs` | Session cost tracking (currently disabled) | deacon-patrol |

### Tier 5: Extended mail operations

Mail is partially implemented; these subcommands are missing.

| Command | Description | Referenced in |
|---------|-------------|---------------|
| `gc mail archive <id>` | Archive processed message | deacon, witness, refinery |
| `gc mail delete <id>` | Delete message | deacon |
| `gc mail mark-read <id>` | Mark message as read | mayor |
| `gc mail hook <id>` | Hook existing mail as assignment | all prompts |
| `gc mail send --human` | Send to human overseer | crew |
| `gc mail send --notify` | Send with tmux bell notification | crew |

## Missing gc features

Features referenced in prompts/formulas that go beyond individual commands.

| Feature | Description | Referenced in |
|---------|-------------|---------------|
| Custom session naming templates | Gas Town uses `{prefix}-{name}` patterns; gc derives `gc-{city}-{agent}` | Implicit in all session references |
| Pre-start hooks (`needs_pre_sync`) | Run a command before agent starts (e.g., `git pull`) | refinery, polecat, crew role configs |
| Prompt template rendering | `{{ cmd }}`, `{{ .TownRoot }}`, `{{ .RigName }}`, etc. | All 8 prompts |
| Nudge delivery modes | `--mode=immediate/queue/wait-idle` | witness, deacon, refinery, crew |
| Event channel system | `gc mol step await-event --channel <name>` | refinery-patrol |
| Activity feed subscription | `gc mol step await-signal --agent-bead <id>` | deacon-patrol, witness-patrol |
| Gate system | `bd gate list/close/check` for async coordination | deacon-patrol |
| Plugin system | `$GT_ROOT/plugins/` with gate conditions | deacon-patrol |
| Wisp lifecycle | Create/squash/burn ephemeral molecules | deacon, witness, refinery |
| Agent bead protocol | Agent state tracking via beads | witness-patrol, deacon-patrol |

## What exists today

gc commands currently implemented (as of this writing):

- `gc start` / `gc stop` / `gc init`
- `gc rig add` / `gc rig list`
- `gc bd` (passthrough to beads CLI)
- `gc agent list/attach/claim/claimed/drain/undrain/drain-check/drain-ack`
- `gc mail send/inbox/read`
- `gc mol create/list/status/step`
- `gc formula list/show`
- `gc events`
- `gc version`

## Statistics

- **Total gc commands referenced:** ~75 unique subcommands
- **Currently implemented:** ~25 subcommands
- **Gap:** ~50 subcommands across 5 priority tiers
