# Work Items (Beads)

Everything in Gas City is a bead — tasks, messages, molecules, convoys.
The `bd` CLI is the primary interface for bead CRUD.

## Rig-scoped beads

Each rig has its own `.beads/` database. When creating or querying beads
for a rig, you **must run `bd` from the rig directory** so it finds the
correct database. City-level beads (no rig) use the city root's `.beads/`.

```
bd create "title" --dir /path/to/rig   # Create in a specific rig's database
cd /path/to/rig && bd create "title"   # Or run from the rig directory
```

Use `gc rig list` to find rig paths. The bead ID prefix (e.g. `hw-` for
hello-world) tells you which rig a bead belongs to.

## Creating work

```
bd create "title"                      # Create in current directory's .beads/
bd create "title" -t bug               # Create with type
bd create "title" --label priority=high # Create with labels
bd create "title" --dir /path/to/rig   # Create in a specific rig
```

## Finding work

```
bd list                                # List beads in current .beads/
bd list --dir /path/to/rig             # List beads in a specific rig
bd ready                               # List beads available for claiming
bd ready --label role:worker           # Filter by label
bd show <id>                           # Show bead details
```

## Claiming and updating

```
bd update <id> --claim                 # Claim a bead (sets assignee + in_progress)
bd update <id> --status in_progress    # Update status
bd update <id> --label <key>=<value>   # Add/update labels
bd update <id> --note "progress..."    # Add a note
```

## Closing work

```
bd close <id>                          # Close a completed bead
bd close <id> --reason "done"          # Close with reason
```

## Dependency graphs

```
gc graph <bead-ids...>                 # Show dependency table for beads
gc graph <convoy-id>                   # Expand convoy children and show graph
gc graph <epic-id>                     # Expand epic children and show graph
gc graph <id> --mermaid                # Output Mermaid.js flowchart for Markdown
```

Readiness is computed within the displayed set — a bead is "ready" when
all its open blockers (within the set) are closed.

## Hooks

```
gc hook show <agent>                   # Show what's on an agent's hook
gc agent claim <agent> <id>            # Put a bead on an agent's hook
```
