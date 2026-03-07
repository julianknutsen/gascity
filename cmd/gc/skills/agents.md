# Agent Management

Agents are the workers in a Gas City workspace. Each runs in its own
session (tmux pane, container, etc).

## Listing and inspecting

```
gc agent list                          # List all agents and their status
gc agent peek <name>                   # Capture recent output from agent session
gc agent peek <name> 100               # Peek with custom line count
gc agent status <name>                 # Show detailed agent status
```

## Adding agents

```
gc agent add --name <name>             # Add agent to city root
gc agent add --name <name> --dir <rig> # Add agent scoped to a rig
```

## Communication

```
gc agent nudge <name> <message>        # Send a message to wake/redirect agent
gc agent attach <name>                 # Attach to agent's live session
gc agent claim <name> <bead-id>        # Put a bead on agent's hook
```

## Multi-instance agents (templates)

An agent with `multi = true` in config is a template — it doesn't start
automatically with `gc start`. Instead, you imperatively create named
instances from it. Unlike pools (declarative auto-scaling), multi is
imperative: you decide when to spin up and tear down instances.

Multi and pool are mutually exclusive on the same agent.

### Config

```toml
[[agents]]
name = "researcher"
multi = true
provider = "claude"
prompt = "prompts/researcher.md"
```

### Instance lifecycle

```
gc agent start <template>              # Start instance (auto-named: inst-1, inst-2, ...)
gc agent start <template> --name spike # Start instance with explicit name
gc agent stop <template/instance>      # Stop instance (session killed, bead marked stopped)
gc agent stop <instance>               # Stop by bare name (if unambiguous)
gc agent start <template> --name spike # Resume a stopped instance
gc agent destroy <template/instance>   # Permanently remove a stopped instance
```

### Addressing instances

Instances are addressed as `template/instance` (e.g., `researcher/spike-1`).
Bare instance names work when unambiguous across all multi templates.

### How it works

Each instance is tracked as a bead (type=multi-instance). The controller
picks up new instance beads and starts sessions for them. `gc start`
starts sessions for all existing running instances but does not create
new ones — use `gc agent start` for that.

## Logs

```
gc agent logs <name>                   # Show session log messages
gc agent logs <name> -f                # Follow new messages in real time
gc agent logs <name> --tail 0          # Show all segments (default: 1)
```

## Handoff

Convenience for context transfer between agents (or to self for session
restart). Sends mail and triggers a session restart.

```
gc handoff <subject> [message]         # Self-handoff: mail self + restart
gc handoff <subject> --target <agent>  # Remote: mail target + kill their session
```

Self-handoff requires agent context (`GC_AGENT`/`GC_CITY` env vars).
Remote handoff can be run from any context with access to the city.

## Lifecycle

```
gc agent suspend <name>                # Suspend agent (reconciler skips it)
gc agent resume <name>                 # Resume a suspended agent
gc agent drain <name>                  # Signal agent to wind down gracefully
gc agent undrain <name>                # Cancel drain
gc agent drain-check <name>            # Check if agent has been drained
gc agent drain-ack <name>              # Acknowledge drain (agent confirms exit)
gc agent request-restart <name>        # Request graceful restart
gc agent kill <name>                   # Force-kill agent session
gc agent destroy <template/instance>   # Permanently remove a stopped multi-instance
```
