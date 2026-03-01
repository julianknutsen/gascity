# {Title}

<!--
TEMPLATE GUIDANCE: This is a design doc template for Gas City proposals.
It describes a FUTURE change -- how we WANT things to work.

For how things work TODAY, write an architecture doc in docs/architecture/.
For the architecture doc template, see docs/architecture/TEMPLATE.md.

When to write a design doc:
- Adding a new primitive or derived mechanism
- Changing an interface that other subsystems depend on
- Adding a config section that activates a new capability level
- Any change where the "how" is non-obvious and worth debating
- Cross-cutting changes that affect multiple packages

When to skip the design doc and just code it:
- Bug fixes with obvious solutions
- Adding a field to an existing, well-understood config section
- Refactors that don't change behavior
- Test improvements
- Documentation updates

Modeled after CockroachDB's RFC process, adapted for Gas City's
design principles (ZFC, Bitter Lesson, Primitive Test, GUPP, NDI).
-->

| Field | Value |
|---|---|
| Status | Draft |
| Date | {YYYY-MM-DD} |
| Author(s) | {names} |
| Issue | {link to GitHub issue or epic, if any} |
| Supersedes | {link to earlier design doc, if replacing one} |

<!--
STATUS VALUES:
- Draft     -- open for discussion, not yet accepted
- Accepted  -- approved for implementation
- Implemented -- code landed, doc kept for historical context
- Obsolete  -- superseded by a later design or abandoned

Update the status as the proposal moves through its lifecycle.
When moving to Implemented, link the relevant commits or PRs.
When moving to Obsolete, link the replacement design doc.
-->

## Summary

<!--
One paragraph. Must answer: WHAT is being proposed, WHY it matters,
HOW it works at the highest level, and WHAT the IMPACT is on existing
users/config/code.

Write this so an LLM reading ONLY this paragraph can decide whether
this proposal is relevant to their current task. Be specific -- "improve
performance" is useless; "reduce reconciler tick from 5s to 500ms by
replacing polling with event-driven wakeup" is useful.

Common mistake: writing a summary that assumes the reader already
understands the problem. State the problem AND the solution.
-->

## Motivation

<!--
Why is this change needed? What concrete problem does it solve?
What is broken, missing, or painful TODAY?

Include at least one concrete example showing the pain. Ideally two:
one simple case and one that shows why the naive fix doesn't work.

Good examples:
- "Today, starting 10 ephemeral workers requires 10 [[agents]] entries
  in city.toml. Adding or removing workers means editing config and
  restarting the city."
- "The reconciler polls every 5s even when nothing changes. In a city
  with 20 agents, this means 20 unnecessary health checks per tick."

Bad examples:
- "This would be nice to have."
- "Other systems do it this way."

If the motivation references a Gas City design principle (ZFC, Bitter
Lesson, GUPP, NDI, SDK self-sufficiency), call it out explicitly.
-->

## Guide-Level Explanation

<!--
AUDIENCE: A Gas City user who has completed the tutorials.

Explain the proposal as if you were teaching it. Show the user
experience FIRST, then explain how it works.

If the proposal changes config, show the new TOML:

    ```toml
    [[agents]]
    name = "scanner"
    prompt_template = "prompts/scanner.md"

    [agents.pool]
    min = 0
    max = 5
    check = "bd ready --label=scan --json | jq length"
    ```

If it changes prompt templates, show the new template syntax.
If it adds CLI commands, show the command and its output.
If it changes agent behavior, describe what the agent sees.

IMPORTANT: Do not use specific role names (mayor, deacon, polecat)
in examples. Gas City has ZERO hardcoded roles. Use generic names
like "scanner", "worker", "overseer", or "builder".

Common mistake: jumping straight to implementation details. This
section answers "what does it look like from the outside?" The next
section answers "how does it work on the inside?"
-->

## Reference-Level Explanation

<!--
AUDIENCE: A Gas City contributor implementing or reviewing this change.

Technical detail. Cover ALL of the following that apply:

1. NEW TYPES / INTERFACES
   - Go type definitions (or close approximations)
   - Which package owns each type
   - Why this interface shape (not another)

2. STATE TRANSITIONS
   - If there's a state machine, draw it (ASCII art)
   - Name every state and every transition
   - What triggers each transition

3. ERROR HANDLING
   - What can go wrong
   - How each error is surfaced (return error, event bus, log)
   - Recovery behavior (retry, abort, degrade)

4. CONCURRENCY
   - What runs concurrently
   - What locks/atomics are needed
   - Race conditions considered and mitigated

5. PERSISTENCE
   - What state is persisted, where, in what format
   - Migration path from current state (if any)
   - What happens on crash (NDI: does the system converge?)

6. PERFORMANCE
   - Expected cost (CPU, memory, I/O, latency)
   - Scaling characteristics (O(n) in what?)
   - Benchmarks to add

7. OBSERVABILITY
   - Events emitted to the event bus
   - What `gc status` should show
   - How an operator diagnoses problems

8. CONFIGURATION
   - New TOML fields with types and defaults
   - Validation rules
   - Progressive activation: which capability level does this require?

9. BACKWARD COMPATIBILITY
   - Does existing config still work unchanged?
   - Migration path for breaking changes
   - Deprecation timeline if applicable

Common mistake: describing the happy path only. Cover corner cases,
error paths, and what happens when agents crash mid-operation (NDI).
-->

## Primitive Test

<!--
GAS CITY SPECIFIC. Apply this section if your proposal adds or modifies
a primitive (Layer 0-1) or could be mistaken for one.

If ADDING a new primitive, you must pass all three conditions from
docs/primitive-test.md:

### Atomicity
Can agents do this safely without races using existing tools (bd, git,
tmux)? If the underlying tool already provides atomicity, this belongs
in the consumer layer, not the SDK.

### Bitter Lesson
Imagine a model 10x more capable. Does this capability become LESS
necessary (consumer layer) or exactly as necessary (primitive)?
If a smarter model would do it better from the prompt, it fails.

### ZFC
Does implementing this in Go require a judgment call? If any line of
Go would contain "if stuck then X" or "if this looks wrong then Y",
the decision belongs in the prompt, not the code.

| Condition | Pass/Fail | Reasoning |
|---|---|---|
| Atomicity | | |
| Bitter Lesson | | |
| ZFC | | |

If ADDING a derived mechanism (Layer 2-4), show the derivation proof:
demonstrate that it composes entirely from existing primitives. Name
which primitives it uses and how.

If NEITHER (e.g., a config change, CLI improvement, or refactor),
write "Not applicable -- this proposal does not add a primitive or
derived mechanism" and briefly explain why.

Common mistake: skipping this section. Every design doc must explicitly
address the Primitive Test, even if only to say it doesn't apply.
-->

## Drawbacks

<!--
Why should we NOT do this? Be honest. Every proposal has costs.

Consider:
- Complexity cost: does this make Gas City harder to understand?
- Maintenance cost: does this add surface area that must be kept working?
- Cognitive cost: does this add concepts users must learn?
- Does this risk violating any Gas City principle? (ZFC, Bitter Lesson,
  GUPP, NDI, SDK self-sufficiency, zero hardcoded roles)
- What are we giving up by choosing this approach?
- Could this become a permanent exclusion (like skills, capability
  flags, MCP/tool registration)?

Common mistake: writing "none" or listing only trivial drawbacks.
If you can't find real drawbacks, you haven't thought hard enough.
The best design docs have drawbacks that genuinely gave the author
pause.
-->

## Alternatives

<!--
What other approaches were considered? Why were they rejected?

Include at least one concrete alternative with enough detail that
a reader could evaluate it independently. For each alternative:

1. Describe the approach
2. List its advantages over the proposed design
3. Explain why it was rejected

The "do nothing" alternative is always valid -- explain what happens
if we don't make this change and why that's insufficient.

If a Kubernetes, Erlang/OTP, or other well-known system solves a
similar problem, reference their approach and explain why Gas City's
solution differs (scale, constraints, ZFC compliance, etc.).

Common mistake: strawman alternatives that nobody would seriously
consider. The alternatives should be genuinely plausible.
-->

## Unresolved Questions

<!--
What needs to be figured out during implementation? What is
intentionally left open for now?

Separate into two categories:

### Before accepting this design
Questions that must be answered before the design is approved.
These block the status moving from Draft to Accepted.

### During implementation
Questions that can be answered while coding. These are explicitly
deferred -- the design is good enough to start without resolving them.

Common mistake: listing questions that the design doc should have
answered. If you can answer it now, answer it in the relevant section
above instead of deferring it here.
-->

## Implementation Plan

<!--
OPTIONAL but recommended for larger proposals.

Break the work into phases, ordered by value delivered. Each phase
should be independently useful -- if we stop after phase 1, we still
have something valuable.

Reference the progressive capability model (Levels 0-8) if the
proposal spans multiple levels.

For each phase:
- What it delivers
- Rough scope (small/medium/large)
- Dependencies on other phases or external work

Common mistake: a single monolithic phase. If your proposal can't be
broken into incremental steps, it might be too large for one design doc.
-->
