# Beads (bd) Enhancement Roadmap

Enhancements to the beads CLI/store that would simplify Gas City's
orchestration layer. Filed here for tracking; implementation lives in
the beads repo.

## 1. Cross-rig `computeBlockedIDs`

**Problem:** `computeBlockedIDs` only checks the local `activeIDs` table.
An `external:beads:some-feature` dep won't be in `activeIDs`, so the
blocking relationship is invisible to `bd ready` in the dependent rig.

**Current workaround:** The deacon's `cross-rig-deps` patrol step polls
for recently closed issues, checks cross-rig dependents via
`bd dep list --direction=up`, and notifies affected witnesses manually.

**Desired behavior:** `computeBlockedIDs` resolves external deps via the
routing layer (like `bd dep list` already does). When an issue in rig A
closes, `bd ready` in rig B automatically sees the unblocked issue.

**Impact:** Eliminates the deacon's `cross-rig-deps` step entirely.
Cross-rig unblocking becomes automatic, matching same-rig behavior.

**Complexity:** Medium. `resolveExternalDependencies` already opens
remote stores and fetches issues. The same pattern needs to be applied
inside `computeBlockedIDs` for external `blocks` deps. Caching and
performance matter since `computeBlockedIDs` runs on every `bd ready`.

## 2. Transitive blocking (molecule root ← step ← gate)

**Problem:** When a gate bead blocks a molecule step, the work bead is
not transitively blocked. `bd ready` can still return the work bead,
leading to agents picking it up and immediately hitting the blocked step.

**Current workaround:** Formula instructions tell agents to
`bd dep add <work-bead> <gate-id>` and unassign themselves when they
hit a blocked step. If agents don't follow instructions, crash-loop
backoff limits the waste.

**Desired behavior:** When all ready steps in a molecule are blocked
(by gates or other deps), the root/work bead is implicitly blocked.
`bd ready` would not return it. When the gate closes and a step becomes
ready, the transitive block lifts and the work bead reappears in
`bd ready`.

**Impact:** Eliminates the need for agents to manually manage blocking
deps on work beads. Gated molecules schedule themselves automatically.

**Complexity:** High. Requires `computeBlockedIDs` to understand the
molecule→step→gate hierarchy and propagate blocking upward. Needs
careful semantics: block root when ALL steps are blocked? When the
CURRENT step is blocked? Needs cycle detection.

**Design question:** Should this use the existing `waits-for` dep type
with a new gate mode, or a new transitive propagation mechanism?
