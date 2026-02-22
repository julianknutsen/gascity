# Gas City

Gas City is an orchestration-builder SDK — a Go toolkit for composing
multi-agent coding workflows. It extracts the battle-tested subsystems from
Steve Yegge's Gas Town (github.com/steveyegge/gastown) into a configurable
SDK where **all role behavior is user-supplied configuration** and the SDK
provides only infrastructure. The core principle: **ZERO hardcoded roles.**
The SDK has no built-in Mayor, Deacon, Polecat, or any other role. If a
line of Go references a specific role name, it's a bug.

You can build Gas Town in Gas City, or Ralph, or Claude Code Agent Teams,
or any other orchestration topology — via specific configurations.

**Why Gas City exists:** Gas Town proved multi-agent orchestration works,
but all its roles are hardwired in Go code. Steve realized the MEOW stack
(Molecular Expression of Work) was powerful enough to abstract roles into
configuration. Gas City extracts that insight into an SDK where Gas Town
becomes one configuration among many.

## Development approach

**Tutorial-driven development.** The tutorials in `docs/tutorials/` define
the user experience. Each tutorial describes a working system. We build
exactly what each tutorial needs, in order, extracting from the Gas Town
codebase where code already exists. We do not build ahead of the current
tutorial.

**TDD.** Write the test first, watch it fail, make it pass. Every package
has `*_test.go` files next to the code. Integration tests that need real
infrastructure (tmux, filesystem) go in `test/` with build tags.

**The spec is a reference, not a blueprint.** When the tutorial's DX
conflicts with the spec, the tutorial wins. We update the spec to match.

## Architecture

**Work is the primitive, not orchestration.** Gas City's orchestration
is a thin layer atop the MEOW stack (beads → molecules → formulas).
The work definition and tracking infrastructure is what matters; the
orchestration shape is configurable on top.

### The nine concepts

Gas City has five irreducible primitives and four derived mechanisms.
Removing any primitive makes it impossible to rebuild Gas Town. Every
mechanism is provably composable from the primitives.

**Five primitives (Layer 0-1):**

1. **Agent Protocol** — start/stop/prompt/observe agents regardless of
   provider. Identity, pools, sandboxes, resume, crash adoption.
2. **Task Store (Beads)** — CRUD + Hook + Dependencies + Labels + Query
   over work units. Everything is a bead: tasks, mail, molecules, convoys.
3. **Event Bus** — append-only pub/sub log of all system activity. Two
   tiers: critical (bounded queue) and optional (fire-and-forget).
4. **Config** — TOML parsing with progressive activation (Levels 0-8 from
   section presence) and multi-layer override resolution.
5. **Prompt Templates** — Go `text/template` in Markdown defining what
   each role does. The behavioral specification.

**Four derived mechanisms (Layer 2-4):**

6. **Messaging** — Mail = `TaskStore.Create(bead{type:"message"})`.
   Nudge = `AgentProtocol.SendPrompt()`. No new primitive needed.
7. **Formulas & Molecules** — Formula = TOML parsed by Config. Molecule =
   root bead + child step beads in Task Store. Wisps = ephemeral molecules.
   Plugins = formulas with gate conditions on Event Bus.
8. **Dispatch (Sling)** — composed: find/spawn agent → select formula →
   create molecule → hook to agent → nudge → create convoy → log event.
9. **Health Patrol** — ping agents (Agent Protocol), compare thresholds
   (Config), publish stalls (Event Bus), restart with backoff.

### Layering invariants

1. **No upward dependencies.** Layer N never imports Layer N+1.
2. **Beads is the universal persistence substrate** for domain state.
3. **Event Bus is the universal observation substrate.**
4. **Config is the universal activation mechanism.**
5. **Side effects (I/O, process spawning) are confined to Layer 0.**

### Progressive capability model

Capabilities activate progressively via config presence. Only use
primitives that belong to the current tutorial's level.

| Level | Adds                    | Tutorial |
|-------|-------------------------|----------|
| 0-1   | Agent + tasks           | 01       |
| 2     | Task loop               | 03       |
| 3     | Multiple agents + pool  | 04       |
| 4     | Messaging               | 04       |
| 5     | Formulas & molecules    | 05a      |
| 6     | Health monitoring       | 05b      |
| 7     | Plugins                 | 05c      |
| 8     | Full orchestration      | 05d      |

## Design decisions (settled)

These decisions are final. Do not revisit them.

- **City-as-directory model.** A city is a directory on disk containing
  `city.toml`, `.gc/` runtime state, and `rigs/` infrastructure.
- **Fresh binary, not a Gas Town fork.** We build `gc` from scratch.
- **TOML for config.** `city.toml` is the single config file.
- **Tutorials win over spec.** When the spec disagrees, we update the spec.
- **No premature abstraction.** Don't build interfaces until two
  implementations exist.
- **Mayor is overseer, not worker.** The mayor plans; coding agents work.
- **`internal/` packages for now.** SDK exports (`pkg/`) are future work.
  Everything is private to the `gc` binary until the API stabilizes.
- **ZERO hardcoded roles.** Roles are pure configuration. No role name
  appears in Go source code.

## Key design principles

- **Zero Framework Cognition (ZFC)** — Go handles transport, not reasoning.
  If a line of Go contains a judgment call, it's a violation. **The ZFC
  test:** does any line of Go contain a judgment call? An `if stuck then
  restart` is framework intelligence. Move the decision to the prompt.
- **Bitter Lesson** — every primitive must become MORE useful as models
  improve, not less. Don't build heuristics or decision trees.
- **GUPP** — "If you find work on your hook, YOU RUN IT." No confirmation,
  no waiting. The hook having work IS the assignment. This is rendered into
  agent prompts via templates, not enforced by Go code.
- **Nondeterministic Idempotence (NDI)** — the system converges to correct
  outcomes because work (beads), hooks, and molecules are all persistent.
  Sessions come and go; the work survives. Multiple independent observers
  check the same state idempotently. Redundancy is the reliability mechanism.

## What Gas City does NOT contain

These are permanent exclusions, not "not yet." Each fails the Bitter
Lesson test — it becomes LESS useful as models improve.

- **No skills system** — the model IS the skill system
- **No capability flags** — a sentence in the prompt is sufficient
- **No MCP/tool registration** — if a tool has a CLI, the agent uses it
- **No decision logic in Go** — the agent decides from prompt and reality
- **No hardcoded role names** — roles are pure configuration

## Code conventions

- Unit tests next to code: `config.go` → `config_test.go`
- `t.TempDir()` for filesystem tests
- Integration tests use `//go:build integration`
- `cobra` for CLI, `github.com/BurntSushi/toml` for config
- Atomic file writes: temp file → `os.Rename`
- No panics in library code — return errors
- Error messages include context: `fmt.Errorf("adding rig %q: %w", name, err)`
- Role names never appear in Go code. If you're writing `if role == "mayor"`,
  it's a design error.

- `TESTING.md` — testing philosophy and tier boundaries. Read before writing any test.

## Code quality gates

Before considering any task complete:

- `go test ./...` passes
- `go vet ./...` clean
- Every exported function has a doc comment
- No premature abstractions
- Tests cover happy path AND edge cases
