# CLI Reference

> **Auto-generated** — do not edit. Run `go run ./cmd/genschema` to regenerate.

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--city` | string |  | path to the city directory (default: walk up from cwd) |

## gc

Gas City CLI — orchestration-builder for multi-agent workflows

```
gc [flags]
```

| Subcommand | Description |
|------------|-------------|
| [gc agent](#gc-agent) | Manage agents |
| [gc config](#gc-config) | Inspect and validate city configuration |
| [gc convoy](#gc-convoy) | Manage convoys (batch work tracking) |
| [gc daemon](#gc-daemon) | Manage the city daemon (background controller) |
| [gc doctor](#gc-doctor) | Check workspace health |
| [gc dolt](#gc-dolt) | Manage the Dolt SQL server |
| [gc event](#gc-event) | Event operations |
| [gc events](#gc-events) | Show the event log |
| [gc formula](#gc-formula) | Manage formulas (multi-step workflow templates) |
| [gc handoff](#gc-handoff) | Send handoff mail and restart agent session |
| [gc help](#gc-help) | Help about any command |
| [gc hook](#gc-hook) | Check for available work (use --inject for Stop hook output) |
| [gc init](#gc-init) | Initialize a new city |
| [gc mail](#gc-mail) | Send and receive messages between agents and humans |
| [gc plugin](#gc-plugin) | Manage plugins (periodic formula dispatch) |
| [gc prime](#gc-prime) | Output the behavioral prompt for an agent |
| [gc restart](#gc-restart) | Restart all agent sessions in the city |
| [gc resume](#gc-resume) | Resume a suspended city |
| [gc rig](#gc-rig) | Manage rigs (projects) |
| [gc sling](#gc-sling) | Route work to an agent or pool |
| [gc start](#gc-start) | Start the city (auto-initializes if needed) |
| [gc status](#gc-status) | Show city-wide status overview |
| [gc stop](#gc-stop) | Stop all agent sessions in the city |
| [gc suspend](#gc-suspend) | Suspend the city (all agents effectively suspended) |
| [gc topology](#gc-topology) | Manage remote topology sources |
| [gc version](#gc-version) | Print gc version information |
| [gc worktree](#gc-worktree) | Manage agent worktrees |

## gc agent

Manage agents in the city workspace.

Agents are the autonomous workers that execute tasks. Each agent runs
in its own tmux session with a configured provider (Claude, Codex, etc).
Agents can be fixed (single instance) or pooled (multiple instances
scaled by demand).

```
gc agent
```

| Subcommand | Description |
|------------|-------------|
| [gc agent add](#gc-agent-add) | Add an agent to the workspace |
| [gc agent attach](#gc-agent-attach) | Attach to an agent session |
| [gc agent drain](#gc-agent-drain) | Signal an agent to drain (wind down gracefully) |
| [gc agent drain-ack](#gc-agent-drain-ack) | Acknowledge drain — signal the controller to stop this session |
| [gc agent drain-check](#gc-agent-drain-check) | Check if this agent is draining (exit 0 = draining) |
| [gc agent kill](#gc-agent-kill) | Force-kill an agent session (reconciler will restart it) |
| [gc agent list](#gc-agent-list) | List workspace agents |
| [gc agent nudge](#gc-agent-nudge) | Send a message to wake or redirect an agent |
| [gc agent peek](#gc-agent-peek) | Capture recent output from an agent session |
| [gc agent request-restart](#gc-agent-request-restart) | Request controller restart this session (blocks until killed) |
| [gc agent resume](#gc-agent-resume) | Resume a suspended agent |
| [gc agent status](#gc-agent-status) | Show agent status |
| [gc agent suspend](#gc-agent-suspend) | Suspend an agent (reconciler will skip it) |
| [gc agent undrain](#gc-agent-undrain) | Cancel drain on an agent |

## gc agent add

Add a new agent to the workspace configuration.

Appends an [[agents]] block to city.toml. The agent will be started
on the next "gc start" or controller reconcile tick. Use --dir to
scope the agent to a rig's working directory.

```
gc agent add --name <name> [flags]
```

**Example:**

```
gc agent add --name mayor
  gc agent add --name polecat --dir my-project
  gc agent add --name worker --prompt-template prompts/worker.md --suspended
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dir` | string |  | Working directory for the agent (relative to city root) |
| `--name` | string |  | Name of the agent |
| `--prompt-template` | string |  | Path to prompt template file (relative to city root) |
| `--suspended` | bool |  | Register the agent in suspended state |

## gc agent attach

Attach to an agent's tmux session for interactive debugging.

Starts the session if not already running, then attaches your terminal.
Detach with the standard tmux detach key (Ctrl-B D by default). Supports
both fixed agents and pool instances (e.g. "polecat-2").

```
gc agent attach <name>
```

## gc agent drain

Signal an agent to drain — wind down its current work gracefully.

Sets a GC_DRAIN metadata flag on the session. The agent should check
for drain status periodically (via "gc agent drain-check") and finish
its current task before exiting. Use "gc agent undrain" to cancel.

```
gc agent drain <name>
```

## gc agent drain-ack

Acknowledge a drain signal — tell the controller to stop this session.

Sets GC_DRAIN_ACK metadata on the session. The controller will stop
the session on its next reconcile tick. Call this after the agent has
finished its current work in response to a drain signal.

```
gc agent drain-ack [name]
```

## gc agent drain-check

Check if this agent is currently draining.

Returns exit code 0 if draining, 1 if not. Designed for use in
conditionals: "if gc agent drain-check; then finish-up; fi".
Uses $GC_AGENT and $GC_CITY env vars when called without arguments.

```
gc agent drain-check [name]
```

## gc agent kill

Force-kill an agent's tmux session immediately.

The session is destroyed without graceful shutdown. If a controller is
running, it will restart the agent on its next reconcile tick. Use
"gc agent drain" for graceful wind-down instead.

```
gc agent kill <name>
```

## gc agent list

List all agents configured in city.toml with annotations.

Shows each agent's qualified name, suspension status, rig suspension
inheritance, and pool configuration. Use --dir to filter by working
directory.

```
gc agent list [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dir` | string |  | Filter agents by working directory |

## gc agent nudge

Send a text message to an agent's running session.

The message is typed into the agent's tmux session as if a human typed
it. Use this to redirect an agent's attention, provide new instructions,
or wake it from an idle state.

```
gc agent nudge <agent-name> <message>
```

## gc agent peek

Capture recent terminal output from an agent's tmux session.

Reads the session's scrollback buffer without attaching. Use --lines
to control how much output to capture (0 = all available scrollback).
Useful for monitoring agent progress without interrupting it.

```
gc agent peek <agent-name> [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--lines` | int | `50` | Number of lines to capture (0 = all scrollback) |

## gc agent request-restart

Signal the controller to stop and restart this agent session.

Sets GC_RESTART_REQUESTED metadata on the session, then blocks forever.
The controller will stop the session on its next reconcile tick and
restart it fresh. The blocking prevents the agent from consuming more
context while waiting.

This command is designed to be called from within an agent session
(uses GC_AGENT and GC_CITY env vars). It emits an agent.draining event
before blocking.

```
gc agent request-restart
```

## gc agent resume

Resume a suspended agent by clearing suspended in city.toml.

The reconciler will start the agent on its next tick. Supports bare
names (resolved via rig context) and qualified names (e.g. "myrig/worker").

```
gc agent resume <name>
```

## gc agent status

Show agent status

```
gc agent status <name>
```

## gc agent suspend

Suspend an agent by setting suspended=true in city.toml.

Suspended agents are skipped by the reconciler — their sessions are not
started or restarted. Existing sessions continue running but won't be
replaced if they exit. Use "gc agent resume" to restore.

```
gc agent suspend <name>
```

## gc agent undrain

Cancel a pending drain signal on an agent.

Clears the GC_DRAIN and GC_DRAIN_ACK metadata flags, allowing the
agent to continue normal operation.

```
gc agent undrain <name>
```

## gc config

Inspect, validate, and debug the resolved city configuration.

The config system supports multi-file composition with includes,
topologies, patches, and overrides. Use "show" to dump the resolved
config and "explain" to see where each value originated.

```
gc config
```

| Subcommand | Description |
|------------|-------------|
| [gc config explain](#gc-config-explain) | Show resolved agent config with provenance annotations |
| [gc config show](#gc-config-show) | Dump the resolved city configuration as TOML |

## gc config explain

Show the resolved configuration for each agent with provenance.

Displays every resolved field with an annotation showing which config
file provided the value. Use --rig and --agent to filter the output.
Useful for debugging config composition and understanding override
resolution.

```
gc config explain [flags]
```

**Example:**

```
gc config explain
  gc config explain --agent mayor
  gc config explain --rig my-project
  gc config explain -f overlay.toml --agent polecat
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--agent` | string |  | filter to a specific agent name |
| `-f`, `--file` | stringArray |  | additional config files to layer (can be repeated) |
| `--rig` | string |  | filter to agents in this rig |

## gc config show

Dump the fully resolved city configuration as TOML.

Loads city.toml with all includes, topologies, patches, and overrides,
then outputs the merged result. Use --validate to check for errors
without printing. Use --provenance to see which file contributed each
config element. Use -f to layer additional config files.

```
gc config show [flags]
```

**Example:**

```
gc config show
  gc config show --validate
  gc config show --provenance
  gc config show -f overlay.toml
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-f`, `--file` | stringArray |  | additional config files to layer (can be repeated) |
| `--provenance` | bool |  | show where each config element originated |
| `--validate` | bool |  | validate config and exit (0 = valid, 1 = errors) |

## gc convoy

Manage convoys — batch work tracking containers.

A convoy is a bead that groups related issues. Issues are linked to a
convoy via parent-child relationships. Convoys track completion progress
and can be auto-closed when all their issues are resolved.

```
gc convoy
```

| Subcommand | Description |
|------------|-------------|
| [gc convoy add](#gc-convoy-add) | Add an issue to a convoy |
| [gc convoy check](#gc-convoy-check) | Auto-close convoys where all issues are closed |
| [gc convoy close](#gc-convoy-close) | Close a convoy |
| [gc convoy create](#gc-convoy-create) | Create a convoy and optionally track issues |
| [gc convoy list](#gc-convoy-list) | List open convoys with progress |
| [gc convoy status](#gc-convoy-status) | Show detailed convoy status |
| [gc convoy stranded](#gc-convoy-stranded) | Find convoys with ready work but no workers |

## gc convoy add

Link an existing issue bead to a convoy.

Sets the issue's parent to the convoy ID, making it appear in the
convoy's progress tracking.

```
gc convoy add <convoy-id> <issue-id>
```

## gc convoy check

Scan open convoys and auto-close any where all child issues are resolved.

Evaluates each open convoy's children. If all children have status
"closed", the convoy is automatically closed and an event is recorded.

```
gc convoy check
```

## gc convoy close

Close a convoy bead manually.

Marks the convoy as closed regardless of child issue status. Use
"gc convoy check" to auto-close convoys where all issues are resolved.

```
gc convoy close <id>
```

## gc convoy create

Create a convoy and optionally link existing issues to it.

Creates a convoy bead and sets the parent of any provided issue IDs to
the new convoy. Issues can also be added later with "gc convoy add".

```
gc convoy create <name> [issue-ids...]
```

**Example:**

```
gc convoy create sprint-42
  gc convoy create sprint-42 issue-1 issue-2 issue-3
```

## gc convoy list

List all open convoys with completion progress.

Shows each convoy's ID, title, and the number of closed vs total
child issues.

```
gc convoy list
```

## gc convoy status

Show detailed status of a convoy and all its child issues.

Displays the convoy's ID, title, status, completion progress, and a
table of all child issues with their status and assignee.

```
gc convoy status <id>
```

## gc convoy stranded

Find open issues in convoys that have no assignee.

Lists issues that are ready for work but not claimed by any agent.
Useful for identifying bottlenecks in convoy processing.

```
gc convoy stranded
```

## gc daemon

Manage the city daemon — a persistent background controller.

The daemon runs "gc start --foreground" as a background process,
continuously reconciling agent state. It can be managed as a system
service via launchd (macOS) or systemd (Linux).

```
gc daemon
```

| Subcommand | Description |
|------------|-------------|
| [gc daemon install](#gc-daemon-install) | Install the daemon as a platform service (launchd/systemd) |
| [gc daemon logs](#gc-daemon-logs) | Tail the daemon log file |
| [gc daemon run](#gc-daemon-run) | Run the controller in the foreground (with log file) |
| [gc daemon start](#gc-daemon-start) | Start the daemon in the background |
| [gc daemon status](#gc-daemon-status) | Show daemon status (PID, uptime) |
| [gc daemon stop](#gc-daemon-stop) | Stop the running daemon |
| [gc daemon uninstall](#gc-daemon-uninstall) | Remove the platform service (launchd/systemd) |

## gc daemon install

Install the daemon as a platform service that starts on login.

Generates and loads a launchd plist (macOS) or systemd user unit
(Linux) that runs "gc daemon run" automatically.

```
gc daemon install [path]
```

## gc daemon logs

Tail the daemon log file (.gc/daemon.log).

Shows recent log output with optional follow mode. Equivalent to
"tail -n 50 .gc/daemon.log" (or "tail -f" with --follow).

```
gc daemon logs [path] [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-f`, `--follow` | bool |  | follow log output |
| `-n`, `--lines` | int | `50` | number of lines to show |

## gc daemon run

Run the controller in the foreground with log file output.

Starts the persistent reconciliation loop, writing output to both
stdout and .gc/daemon.log. This is the command that "gc daemon start"
forks in the background.

```
gc daemon run [path] [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-f`, `--file` | stringArray |  | additional config files to layer (can be repeated) |
| `--strict` | bool |  | promote config collision warnings to errors |

## gc daemon start

Fork the daemon as a background process.

Spawns "gc daemon run" as a detached child process and verifies it
acquired the controller lock. Only one daemon can run per city.

```
gc daemon start [path]
```

**Example:**

```
gc daemon start
  gc daemon start ~/my-city
```

## gc daemon status

Show whether the daemon is running, its PID, and uptime.

Reads the PID file and verifies the process is alive. Derives uptime
from the most recent controller.started event in the event log.

```
gc daemon status [path]
```

## gc daemon stop

Signal the running daemon to shut down gracefully.

Connects to the controller's unix socket and sends a stop command.
The daemon performs graceful agent shutdown before exiting.

```
gc daemon stop [path]
```

## gc daemon uninstall

Remove the platform service and stop the daemon.

Unloads and deletes the launchd plist (macOS) or systemd unit (Linux)
created by "gc daemon install".

```
gc daemon uninstall [path]
```

## gc doctor

Run diagnostic health checks on the city workspace.

Checks city structure, config validity, binary dependencies (tmux, git,
bd, dolt), controller status, agent sessions, zombie/orphan sessions,
bead stores, Dolt server health, event log integrity, and per-rig
health. Use --fix to attempt automatic repairs.

```
gc doctor [flags]
```

**Example:**

```
gc doctor
  gc doctor --fix
  gc doctor --verbose
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--fix` | bool |  | attempt to fix issues automatically |
| `-v`, `--verbose` | bool |  | show extra diagnostic details |

## gc dolt

Manage the Dolt SQL server used for bead storage.

Dolt provides the persistent database backing for the beads system.
These commands help inspect, recover, and sync the database.

```
gc dolt
```

| Subcommand | Description |
|------------|-------------|
| [gc dolt list](#gc-dolt-list) | List Dolt databases |
| [gc dolt logs](#gc-dolt-logs) | Tail the Dolt server log file |
| [gc dolt recover](#gc-dolt-recover) | Recover Dolt from read-only state |
| [gc dolt rollback](#gc-dolt-rollback) | List or restore from migration backups |
| [gc dolt sql](#gc-dolt-sql) | Open an interactive Dolt SQL shell |
| [gc dolt sync](#gc-dolt-sync) | Push databases to configured remotes |

## gc dolt list

List all Dolt databases with their filesystem paths.

Shows databases for the HQ (city) and all configured rigs.

```
gc dolt list
```

## gc dolt logs

Tail the Dolt server log file.

Shows recent log output with optional follow mode.

```
gc dolt logs [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-f`, `--follow` | bool |  | follow the log in real time |
| `-n`, `--lines` | int | `50` | number of lines to show |

## gc dolt recover

Check for and recover from Dolt read-only state.

Dolt can enter read-only mode after certain failures. This command
detects the condition and attempts automatic recovery by restarting
the server.

```
gc dolt recover
```

## gc dolt rollback

List available migration backups or restore from one.

With no arguments, lists all migration backups (newest first).
With a backup path or timestamp, restores from that backup.
Restore is destructive and requires --force.

```
gc dolt rollback [path-or-timestamp] [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool |  | required for destructive restore |

## gc dolt sql

Open an interactive Dolt SQL shell.

Connects to the running Dolt server if available, otherwise opens
in embedded mode using the first database directory found.

```
gc dolt sql
```

## gc dolt sync

Push Dolt databases to their configured remotes.

Stops the server for a clean push, syncs each database, then restarts.
Use --gc to purge closed ephemeral beads before syncing to reduce
transfer size. Use --dry-run to preview without pushing.

```
gc dolt sync [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--db` | string |  | sync only the named database |
| `--dry-run` | bool |  | show what would be pushed without pushing |
| `--force` | bool |  | force-push to remotes |
| `--gc` | bool |  | purge closed ephemeral beads before sync |

## gc event

Event operations

```
gc event
```

| Subcommand | Description |
|------------|-------------|
| [gc event emit](#gc-event-emit) | Emit an event to the city event log |

## gc event emit

Record a custom event to the city event log.

Best-effort: always exits 0 so bead hooks never fail. Supports
attaching arbitrary JSON payloads.

```
gc event emit <type> [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--actor` | string |  | Actor name (default: GC_AGENT or "human") |
| `--message` | string |  | Event message |
| `--payload` | string |  | JSON payload to attach to the event |
| `--subject` | string |  | Event subject (e.g. bead ID) |

## gc events

Show the city event log with optional filtering.

Events are recorded to .gc/events.jsonl by the controller, agent
lifecycle operations, and bead mutations. Use --type and --since to
filter. Use --watch to block until matching events arrive (useful for
scripting and automation).

```
gc events [flags]
```

**Example:**

```
gc events
  gc events --type bead.created --since 1h
  gc events --watch --type convoy.closed --timeout 5m
  gc events --seq
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--after` | uint64 |  | Resume watching from this sequence number (0 = current head) |
| `--payload-match` | stringArray |  | Filter by payload field (key=value, repeatable) |
| `--seq` | bool |  | Print the current head sequence number and exit |
| `--since` | string |  | Show events since duration ago (e.g. 1h, 30m) |
| `--timeout` | string | `30s` | Max wait duration for --watch (e.g. 30s, 5m) |
| `--type` | string |  | Filter by event type (e.g. bead.created) |
| `--watch` | bool |  | Block until matching events arrive |

## gc formula

Manage formulas — TOML-defined multi-step workflow templates.

Formulas define sequences of steps (beads) with dependency relationships.
They are instantiated as molecules (step trees with a root bead) or
wisps (ephemeral molecules). Formulas live in the .gc/formulas/ directory.

```
gc formula
```

| Subcommand | Description |
|------------|-------------|
| [gc formula list](#gc-formula-list) | List available formulas |
| [gc formula show](#gc-formula-show) | Show details of a formula |

## gc formula list

List all available formulas by scanning the formulas directory.

Looks for *.formula.toml files in the configured formulas directory
and prints their names.

```
gc formula list
```

## gc formula show

Parse and display the details of a named formula.

Shows the formula name, description, step count, and each step with
its ID, title, and dependency chain. Validates the formula before
displaying.

```
gc formula show <name>
```

**Example:**

```
gc formula show code-review
  gc formula show pancakes
```

## gc handoff

Convenience command for context handoff.

Self-handoff (default): sends mail to self and blocks until controller
restarts the session. Equivalent to:

  gc mail send $GC_AGENT <subject> [message]
  gc agent request-restart

Remote handoff (--target): sends mail to target agent and kills its
session. The reconciler restarts it with the handoff mail waiting.
Returns immediately. Equivalent to:

  gc mail send <target> <subject> [message]
  gc agent kill <target>

Self-handoff requires agent context (GC_AGENT/GC_CITY env vars).
Remote handoff can be run from any context with access to the city.

```
gc handoff <subject> [message] [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--target` | string |  | Remote agent to handoff (sends mail + kills session) |

## gc help

Help provides help for any command in the application.
Simply type gc help [path to command] for full details.

```
gc help [command]
```

## gc hook

Checks for available work using the agent's work_query config.

Without --inject: prints raw output, exits 0 if work exists, 1 if empty.
With --inject: wraps output in <system-reminder> for hook injection, always exits 0.

The agent is determined from $GC_AGENT or a positional argument.

```
gc hook [agent] [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--inject` | bool |  | output <system-reminder> block for hook injection |

## gc init

Create a new Gas City workspace in the given directory (or cwd).

Runs an interactive wizard to choose a config template and coding agent
provider. Creates the .gc/ runtime directory, rigs/ directory, default
prompts and formulas, and writes city.toml. Use --file to skip the
wizard and initialize from an existing TOML config file.

```
gc init [path] [flags]
```

**Example:**

```
gc init
  gc init ~/my-city
  gc init --file examples/gastown.toml ~/bright-lights
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--file` | string |  | path to a TOML file to use as city.toml |

## gc mail

Send and receive messages between agents and humans.

Mail is implemented as beads with type="message". Messages have a
sender, recipient, and body. Use "gc mail check --inject" in agent
hooks to deliver mail notifications into agent prompts.

```
gc mail
```

| Subcommand | Description |
|------------|-------------|
| [gc mail archive](#gc-mail-archive) | Archive a message without reading it |
| [gc mail check](#gc-mail-check) | Check for unread mail (use --inject for hook output) |
| [gc mail inbox](#gc-mail-inbox) | List unread messages (defaults to your inbox) |
| [gc mail read](#gc-mail-read) | Read a message and mark it as read |
| [gc mail send](#gc-mail-send) | Send a message to an agent or human |

## gc mail archive

Close a message bead without displaying its contents.

Use this to dismiss a message without reading it. The message is marked
as closed and will no longer appear in mail check or inbox results.

```
gc mail archive <id>
```

## gc mail check

Check for unread mail addressed to an agent.

Without --inject: prints the count and exits 0 if mail exists, 1 if
empty. With --inject: outputs a <system-reminder> block suitable for
hook injection (always exits 0). The recipient defaults to $GC_AGENT
or "human".

```
gc mail check [agent] [flags]
```

**Example:**

```
gc mail check
  gc mail check --inject
  gc mail check mayor
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--inject` | bool |  | output <system-reminder> block for hook injection |

## gc mail inbox

List all unread messages for an agent or human.

Shows message ID, sender, and body in a table. The recipient defaults
to $GC_AGENT or "human". Pass an agent name to view another agent's inbox.

```
gc mail inbox [agent]
```

## gc mail read

Display a message and mark it as read.

Shows the full message details (ID, sender, recipient, date, body) and
closes the message bead. Closed messages no longer appear in mail check
or inbox results.

```
gc mail read <id>
```

## gc mail send

Send a message to an agent or human.

Creates a message bead addressed to the recipient. The sender defaults
to $GC_AGENT (in agent sessions) or "human". Use --notify to nudge
the recipient after sending. Use --from to override the sender identity.

```
gc mail send <to> <body> [flags]
```

**Example:**

```
gc mail send mayor "Build is green"
  gc mail send human "Review needed for PR #42"
  gc mail send polecat "Priority task" --notify
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--from` | string |  | sender identity (default: $GC_AGENT or "human") |
| `--notify` | bool |  | nudge the recipient after sending |

## gc plugin

Manage plugins — formulas with gate conditions for periodic dispatch.

Plugins are formulas annotated with scheduling gates (interval, cron
schedule, or shell check commands). The controller evaluates gates
periodically and dispatches plugin formulas when they are due.

```
gc plugin
```

| Subcommand | Description |
|------------|-------------|
| [gc plugin check](#gc-plugin-check) | Check which plugins are due to run |
| [gc plugin history](#gc-plugin-history) | Show plugin execution history |
| [gc plugin list](#gc-plugin-list) | List available plugins |
| [gc plugin run](#gc-plugin-run) | Execute a plugin manually |
| [gc plugin show](#gc-plugin-show) | Show details of a plugin |

## gc plugin check

Evaluate gate conditions for all plugins and show which are due.

Prints a table with each plugin's gate, due status, and reason. Returns
exit code 0 if any plugin is due, 1 if none are due.

```
gc plugin check
```

## gc plugin history

Show execution history for plugins.

Queries bead history for past plugin runs. Optionally filter by plugin
name.

```
gc plugin history [name]
```

## gc plugin list

List all available plugins with their gate type, schedule, and target pool.

Scans formula layers for formulas that have plugin metadata
(gate, interval, schedule, check, pool).

```
gc plugin list
```

## gc plugin run

Execute a plugin manually, bypassing its gate conditions.

Instantiates a wisp from the plugin's formula and routes it to the
target pool (if configured). Useful for testing plugins or triggering
them outside their normal schedule.

```
gc plugin run <name>
```

## gc plugin show

Display detailed information about a named plugin.

Shows the plugin name, description, formula reference, gate type,
scheduling parameters, check command, target pool, and source file.

```
gc plugin show <name>
```

## gc prime

Outputs the behavioral prompt for an agent.

Use it to prime any CLI coding agent with city-aware instructions:
  claude "$(gc prime mayor)"
  codex --prompt "$(gc prime worker)"

If agent-name matches a configured agent with a prompt_template,
that template is output. Otherwise outputs a default worker prompt.

```
gc prime [agent-name]
```

## gc restart

Restart the city by stopping all agents then starting them again.

Equivalent to running "gc stop" followed by "gc start". Performs a
full one-shot reconciliation after stopping, which re-reads city.toml
and starts all configured agents.

```
gc restart [path]
```

## gc resume

Resume a suspended city by clearing workspace.suspended in city.toml.

Restores normal operation: the reconciler will spawn agents again and
gc hook/prime will return work. Use "gc agent resume" to resume
individual agents, or "gc rig resume" for rigs.

```
gc resume [path]
```

## gc rig

Manage rigs (external project directories) registered with the city.

Rigs are project directories that the city orchestrates. Each rig gets
its own beads database, agent hooks, and cross-rig routing. Agents
are scoped to rigs via their "dir" field.

```
gc rig
```

| Subcommand | Description |
|------------|-------------|
| [gc rig add](#gc-rig-add) | Register a project as a rig |
| [gc rig list](#gc-rig-list) | List registered rigs |
| [gc rig restart](#gc-rig-restart) | Restart all agents in a rig |
| [gc rig resume](#gc-rig-resume) | Resume a suspended rig |
| [gc rig status](#gc-rig-status) | Show rig status and agent running state |
| [gc rig suspend](#gc-rig-suspend) | Suspend a rig (reconciler will skip its agents) |

## gc rig add

Register an external project directory as a rig.

Creates rig infrastructure (rigs/ directory, rig.toml, beads database),
installs agent hooks if configured, generates cross-rig routes, and
appends the rig to city.toml. Use --topology to apply a topology
directory that defines the rig's agent configuration.

```
gc rig add <path> [flags]
```

**Example:**

```
gc rig add /path/to/project
  gc rig add ./my-project --topology topologies/gastown
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--topology` | string |  | topology directory for rig agents |

## gc rig list

List all registered rigs with their paths, prefixes, and beads status.

Shows the HQ rig (the city itself) and all configured rigs. Each rig
displays its bead ID prefix and whether its beads database is initialized.

```
gc rig list
```

## gc rig restart

Kill all agent sessions belonging to a rig.

The reconciler will restart the agents on its next tick. This is a
quick way to force-refresh all agents working on a particular project.

```
gc rig restart <name>
```

## gc rig resume

Resume a suspended rig by clearing suspended in city.toml.

The reconciler will start the rig's agents on its next tick.

```
gc rig resume <name>
```

## gc rig status

Show rig status and agent running state

```
gc rig status <name>
```

## gc rig suspend

Suspend a rig by setting suspended=true in city.toml.

All agents scoped to the suspended rig are effectively suspended —
the reconciler skips them and gc hook returns empty. The rig's beads
database remains accessible. Use "gc rig resume" to restore.

```
gc rig suspend <name>
```

## gc sling

Route a bead to an agent or pool using the target's sling_query.

The target is an agent qualified name (e.g. "mayor" or "hello-world/polecat").
The second argument is a bead ID, or a formula name when --formula is set.

With --formula, a wisp (ephemeral molecule) is instantiated from the formula
and its root bead is routed to the target.

```
gc sling <target> <bead-or-formula> [flags]
```

**Example:**

```
gc sling mayor abc123
  gc sling polecat code-review --formula --nudge
  gc sling polecat my-formula --formula --title "Sprint work" --var repo=gascity
  gc sling mayor BL-1 --merge=mr
  gc sling mayor BL-1 --no-convoy
  gc sling mayor BL-1 --owned
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--force` | bool |  | suppress warnings for suspended/empty targets |
| `-f`, `--formula` | bool |  | treat argument as formula name |
| `--merge` | string |  | merge strategy: direct, mr, or local |
| `--no-convoy` | bool |  | skip auto-convoy creation |
| `--nudge` | bool |  | nudge target after routing |
| `--owned` | bool |  | mark auto-convoy as owned (skip auto-close) |
| `-t`, `--title` | string |  | wisp root bead title (with --formula) |
| `--var` | stringArray |  | variable substitution for formula (key=value, repeatable) |

## gc start

Start the city by launching all configured agent sessions.

Auto-initializes the city if no .gc/ directory exists. Fetches remote
topologies, resolves providers, installs hooks, and starts agent sessions
via one-shot reconciliation. Use --foreground for a persistent controller
that continuously reconciles agent state.

```
gc start [path] [flags]
```

**Example:**

```
gc start
  gc start ~/my-city
  gc start --foreground
  gc start -f overlay.toml --strict
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `-f`, `--file` | stringArray |  | additional config files to layer (can be repeated) |
| `--foreground` | bool |  | run as a persistent controller (reconcile loop) |
| `--strict` | bool |  | promote config collision warnings to errors |

## gc status

Shows a city-wide overview: controller state, suspension,
all agents with running status, rigs, and a summary count.

```
gc status [path]
```

## gc stop

Stop all agent sessions in the city with graceful shutdown.

Sends interrupt signals to running agents, waits for the configured
shutdown timeout, then force-kills any remaining sessions. Also stops
the Dolt server and cleans up orphan sessions. If a controller is
running, delegates shutdown to it.

```
gc stop [path]
```

## gc suspend

Suspends the city by setting workspace.suspended = true in city.toml.

This inherits downward — when the city is suspended, all agents are
effectively suspended regardless of their individual suspended fields.
The reconciler won't spawn agents, gc hook/prime return empty.

Use "gc resume" to restore.

```
gc suspend [path]
```

## gc topology

Manage remote topology sources that provide agent configurations.

Topologies are git repositories containing topology.toml files that
define agent configurations for rigs. They are cached locally and
can be pinned to specific git refs.

```
gc topology
```

| Subcommand | Description |
|------------|-------------|
| [gc topology fetch](#gc-topology-fetch) | Clone missing and update existing remote topologies |
| [gc topology list](#gc-topology-list) | Show remote topology sources and cache status |

## gc topology fetch

Clone missing and update existing remote topology caches.

Fetches all configured topology sources from their git repositories,
updates the local cache, and writes a lockfile with commit hashes
for reproducibility. Automatically called during "gc start".

```
gc topology fetch
```

## gc topology list

Show configured topology sources with their cache status.

Displays each topology's name, source URL, git ref, cache status,
and locked commit hash (if available).

```
gc topology list
```

## gc version

Print gc version, git commit, and build date.

Version information is injected via ldflags at build time.

```
gc version
```

## gc worktree

Manage git worktrees created for agents with isolation="worktree".

When agents use worktree isolation, each gets its own git worktree
branched from the rig's repository. These commands help inspect and
clean up worktrees.

```
gc worktree
```

| Subcommand | Description |
|------------|-------------|
| [gc worktree clean](#gc-worktree-clean) | Remove agent worktrees |
| [gc worktree list](#gc-worktree-list) | List all agent worktrees with status |

## gc worktree clean

Remove agent worktrees and their associated git branches.

By default, refuses to remove worktrees with uncommitted work, unpushed
commits, or stashes. Use --force to override. Use --all to clean all
worktrees at once (without --force, dirty ones are preserved).

```
gc worktree clean [paths...] [flags]
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--all` | bool |  | clean all worktrees |
| `--force` | bool |  | remove even dirty worktrees with uncommitted work |

## gc worktree list

List all agent worktrees with branch name, status, and path.

Status flags include "dirty" (uncommitted changes), "unpushed"
(commits not pushed to remote), and "stash" (stashed changes).
Returns "clean" if no issues are detected.

```
gc worktree list
```

