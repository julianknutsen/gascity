# {Subsystem Name}

<!--
TEMPLATE GUIDANCE: This is a current-state architecture document.
It describes how this subsystem works TODAY, not how we wish it worked.
For proposed changes, write a design doc in docs/design/ instead.

Audience: Gas City contributors (human and LLM agent).
Update this document when the implementation changes.
-->

> Last verified against code: {YYYY-MM-DD}

## Summary

<!--
2-4 sentences. What this subsystem IS, what problem it solves, and
where it sits in Gas City's layer model (0-1 primitive, 2-4 derived,
or infrastructure). An LLM reading only this paragraph should
understand whether this subsystem is relevant to their current task.
-->

## Key Concepts

<!--
Define the 3-7 terms specific to this subsystem. Use a definition
list format. These definitions are AUTHORITATIVE -- if they conflict
with usage elsewhere, this document wins and the other should be
updated. Cross-reference docs/architecture/glossary.md.
-->

- **Term**: Definition.

## Architecture

<!--
How the subsystem is structured internally. Include:
- The main types/interfaces and their relationships
- A data flow or state diagram (ASCII art)
- Which Go packages implement this subsystem

Link to specific files: `internal/beads/store.go`
-->

### Data Flow

<!--
Trace the most common operation end-to-end. For example, for the
bead store: Create -> Store -> Hook -> Claim -> Update -> Close.
Name the functions involved at each step.
-->

### Key Types

<!--
List the 3-5 most important types with one-line descriptions.
Link to their source files.
-->

## Invariants

<!--
CRITICAL SECTION. List the properties that MUST hold for this
subsystem to be correct. These are the things that, if violated,
indicate a bug. State them as testable assertions.

Example:
- A bead's hook field is set at most once (CAS semantics).
- The event bus never drops critical-tier events.

LLM agents use this section to validate their code changes.
-->

## Interactions

<!--
How this subsystem relates to others. Use tables:
-->

| Depends on | How |
|---|---|
| Package | Description of dependency |

| Depended on by | How |
|---|---|
| Package | Description of dependency |

## Code Map

<!--
Where to find the implementation. List packages and key files.
This is the "if you need to change this, start here" section.
-->

## Configuration

<!--
What config options control this subsystem's behavior.
Link to docs/reference/config.md for the full schema.
Show a minimal TOML example.
-->

## Testing

<!--
How this subsystem is tested. What test files exist, what
integration tests cover it, what edge cases are tested.
Link to TESTING.md for overall testing philosophy.
-->

## Known Limitations

<!--
What doesn't work yet, what's known to be fragile, what's
planned but not implemented. Be honest -- this helps both
human contributors and LLM agents avoid pitfalls.
-->

## See Also

<!--
Explicit cross-references to related documents. Use descriptive
link text that explains WHY someone would follow the link.

- [Config reference](../reference/config.md) -- full TOML schema
  for this subsystem's configuration options
- [Life of a Bead](./life-of-a-bead.md) -- end-to-end trace
  showing how this subsystem participates in bead lifecycle
-->
