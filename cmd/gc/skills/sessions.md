# Chat Sessions

Sessions are persistent conversations backed by agent templates. They can
be suspended to free resources and resumed later with full conversation
continuity.

## Creating sessions

```
gc session new <template>              # Create session and attach
gc session new <template> --title "x"  # Create with a descriptive title
gc session new <template> --no-attach  # Create without attaching
```

The template must be an agent defined in city.toml. Each session gets a
unique ID for later reference.

## Listing and inspecting

```
gc session list                        # List active and suspended sessions
gc session list --state all            # Include closed sessions
gc session list --state suspended      # Filter by state
gc session list --template helper      # Filter by template
gc session list --json                 # JSON output
gc session peek <id>                   # View output without attaching
gc session peek <id> --lines 100       # Custom line count (default: 50)
```

## Attaching and resuming

```
gc session attach <id>                 # Attach to active or resume suspended
```

If the session is active with a live tmux session, reattaches. If
suspended or the session died, resumes using the provider's resume
mechanism.

## Lifecycle management

```
gc session suspend <id>                # Save state and free resources
gc session close <id>                  # End conversation permanently
gc session rename <id> <title>         # Rename a session
gc session prune                       # Close old suspended sessions (default: 7d)
gc session prune --before 24h          # Custom age threshold
```

Prune only affects suspended sessions — active sessions are never pruned.
