---
title: Operator-authored environment as Launch identity
type: plan
date: 2026-09-08
root_bead: ga-mxs22e
architecture_decision: ga-3a42sp
status: implementation-ready
---

# Operator-authored environment as Launch identity

## Outcome

After an operator changes environment values in provider, workspace, agent, or
rig-patch configuration and reloads the City, every affected alive session
relaunches with the new values within one reconciliation tick. The relaunch
keeps the warm runtime and the resumable session identity intact.

Credential rotation, controller-process passthrough, and generated or ephemeral
environment values remain outside session identity. They must not trigger
session churn.

This plan implements Option A′ from the settled architecture decision on
`ga-3a42sp`. It does not reopen the Provision-versus-Launch classification.

## Scope

In scope:

- A dedicated `runtime.Config` identity field for operator-authored config env.
- Core and Launch fingerprint coverage for that field, with a v5→v6 format
  bump and matching golden fixture.
- Capture of the effective operator-authored values from `[workspace].env`,
  `[providers.*].env`, `[[agent]].env`, and provider selection through
  `[[rigs.patches]].provider`.
- Regression coverage for both rig-scope provider-resolution paths and for an
  alive named session's relaunch behavior.

Out of scope:

- Widening `envFingerprintAllow` or reusing `FingerprintExtra`; both are
  Provision-tier and would choose the wrong restart behavior.
- Hashing the final merged process environment, upstream credentials,
  controller passthrough, generated session values, or ephemeral ports.
- Changing the asleep-session resume behavior tracked by `ga-580naz`.
- Building the immediate `gc doctor` visibility check tracked by `ga-i3yh2a`.
- Any new role-specific behavior or decision logic.

## Settled constraints

- Operator-authored config env is Launch-tier because the agent process reads it
  when invoked and the existing relaunch path re-executes that process with the
  current environment.
- The new identity surface must be separate from both `Env` and
  `FingerprintExtra`.
- The three authored layers are captured before the process environment gains
  passthrough, generated agent values, and resolved credentials.
- An isolated change moves `CoreFingerprint` and `LaunchFingerprint`, never
  `ProvisionFingerprint`.
- A fingerprint version change silently rebaselines existing sessions; it is not
  a deployment-time fleet drain.
- The existing `Config.Upstream` Launch-tier change in commit `fbf203fcab` is the
  repository precedent to reuse where applicable.

## Work packages

| Order | Bead | Owner | Deliverable | Acceptance summary |
|---|---|---|---|---|
| 1 | `ga-ngqc5f` | Validator (`needs-tests`) | RED contract suite | Proves the Launch-only partition, exact env boundary, both rig-resolution paths, and alive-session relaunch/session-key preservation. |
| 2 | `ga-i91hrn` | Builder (`ready-to-build`) | Runtime identity and fingerprint v6 | Adds the separate map field, deterministic Core+Launch hashing, exhaustive partition classification, version pin, and regenerated golden fixture. |
| 3 | `ga-evj082` | Builder (`ready-to-build`) | Resolution and reconciliation integration | Populates the field from exactly the authored layers and makes the complete validator suite pass through the existing relaunch path. |

Each bead contains its full measurable acceptance criteria. All three carry a
`discovered-from:ga-mxs22e` edge so the work remains traceable to the
operator-authored goal.

## Dependency graph

```text
ga-ngqc5f  RED contracts
    ↓
ga-i91hrn  runtime identity + fingerprint v6
    ↓
ga-evj082  template resolution + relaunch integration
```

The ordering is intentional. Tests establish the contract before production
changes; template resolution cannot populate the new field until the runtime
type and hashing contract exist.

## Acceptance across the plan

The initiative is complete when all of the following are true:

1. Changing only an operator-authored env value changes Core and Launch
   identity, not Provision identity.
2. Workspace, provider, agent, and rig-patch provider paths all contribute their
   effective expanded values with the same precedence as the runtime env.
3. Controller passthrough, resolved credentials, generated agent values, and
   ephemeral values do not contribute to the new identity.
4. A provider env value reaches a rig-scoped agent through both workspace-default
   and rig-patch provider selection.
5. An alive named session receives exactly one relaunch for an authored-env-only
   drift, keeps its `session_key`, avoids a full reset/reprovision, and observes
   the new value within one reconciliation tick.
6. Fingerprint v6 golden and partition guards pass, along with focused package
   tests, the prescribed fast baseline, `go vet ./...`, and the staged
   pre-commit gate.

## Rollout coordination and risks

- `ga-i3yh2a` is the immediate doctor-based visibility safety net. It is already
  labeled for build and budget-deferred until 2026-09-09T00:00:00Z. Do not
  duplicate it or make this plan depend on it.
- `ga-580naz` tracks the pre-existing asleep named-session path that can mint a
  new session key. Prefer the same rollout window, but do not add a hard
  dependency: the architecture decision confines this plan's required proof to
  the already-safe alive-session relaunch path.
- If the accepted tests show that `session_reconciler.go` requires new decision
  logic, stop and return that finding to architecture. The current decision
  expects the existing Launch-only drift path to work without logic changes.
- If implementation introduces a new `config.Agent` field despite the planned
  direct runtime capture, the repository's patch, override, merge, and clone
  completeness rules all apply before the change can ship.

## Handoff

Route `ga-ngqc5f` to the validator first. The builder beads retain their routing
labels and dependency edges so they become ready in order. The handoff messages
must point downstream agents to `ga-3a42sp` for the settled rationale and to the
acceptance criteria on their own beads; no agent should re-derive the tier
decision.
