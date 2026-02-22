# Gas City Testing Philosophy

## Three tiers, clear boundaries

### 1. Unit tests (`*_test.go` next to the code)

Test what the CODE does. Internal behavior, edge cases, precise failure
injection. These are fast and run everywhere.

- Use `t.TempDir()` for filesystem tests
- Use `require` for preconditions (fail immediately), `assert` for checks
- Construct exact broken states in Go — corrupt files, concurrent writes,
  duplicate IDs, missing directories
- No env vars for controlling behavior — pass dependencies directly
- Same package as the code under test (access to unexported functions)

```go
func TestBeadStore_CorruptLine(t *testing.T) {
    dir := t.TempDir()
    os.WriteFile(filepath.Join(dir, "beads.jsonl"),
        []byte("{\"id\":\"gc-1\"}\nthis is not json\n"), 0644)
    store := beads.NewStore(dir)
    items, err := store.List()
    require.NoError(t, err)
    assert.Len(t, items, 1) // skips bad line, doesn't crash
}
```

When to use: corrupted data, concurrent writes, specific error types,
double-claim conflicts, rollback behavior, boundary conditions.

### 2. Testscript (`.txtar` files in `cmd/gc/testdata/`)

Test what the USER sees. Run the real `gc` binary, assert on stdout/stderr.
These are the tutorial regression tests — each `.txtar` corresponds to a
tutorial's shell interactions.

- Uses `github.com/rogpeppe/go-internal/testscript`
- Env vars swap real infra for fakes: `GC_SESSION=fake`
- Fakes have at most three modes per dependency:
  - `GC_SESSION=fake` — works, but in-memory
  - `GC_SESSION=fail` — all operations return errors
  - (absent) — use real tmux
- `!` prefix means command should fail
- `stdout` / `stderr` assert on output
- `-- filename --` blocks create test fixtures

```
env GC_SESSION=fake

exec gc init $WORK/bright-lights
stdout 'City initialized'

exec gc rig add $WORK/tower-of-hanoi
stdout 'Adding rig'

exec gc bead create 'Build a Tower of Hanoi app'
stdout 'status: open'

-- $WORK/tower-of-hanoi/.git/HEAD --
ref: refs/heads/main
```

When to use: CLI output format, command success/failure, user-facing error
messages, tutorial flows end to end.

**The env var rule:** if you need more than two env vars to set up a failure
scenario, it's a unit test, not a testscript.

### 3. Integration tests (`//go:build integration`)

Test that real pieces fit together. Need real tmux, real filesystem, real
agent sessions. Run separately — not in CI by default.

```go
//go:build integration

func TestRealTmuxSession(t *testing.T) {
    // actually creates and kills tmux sessions
}
```

When to use: proving the fakes are honest, smoke testing the real infra,
testing tmux session lifecycle with real processes.

Run with: `go test -tags integration ./test/...`

### 4. Upstream verification (`//go:build doltserver_upstream`)

The `internal/dolt/` package contains code copied verbatim from gastown.
Its tests are gated behind the `doltserver_upstream` build tag so they
don't run on pre-commit or in normal CI. Run them manually after
re-copying upstream code to verify the copy is clean:

```
go test -tags doltserver_upstream ./internal/dolt/
```

Gas City's own tests for this code live in `gascity_test.go` (adapter
unit tests) and `test/integration/bdstore_test.go` (conformance).

#### Two flavors of integration tests

**Low-level** (`internal/session/tmux/tmux_test.go`): test raw tmux
operations (NewSession, HasSession, KillSession) directly against the
tmux library. Session names use the `gt-test-` prefix.

**End-to-end** (`test/integration/`): build the real `gc` binary and
run it against real tmux. Validates the tutorial experience: `gc init`,
`gc start`, `gc stop`, bead CRUD.

**BdStore conformance** (`test/integration/bdstore_test.go`): runs the
beads conformance suite against `BdStore` backed by a real dolt server.
Proves the full stack: dolt server → bd CLI → BdStore → beads.Store.
Requires dolt and bd installed; skips otherwise.

#### Session safety for end-to-end tests

Test cities use a **`gctest-<8hex>` naming prefix** so sessions are
visually distinct from real gascity sessions (`gc-<cityname>-<agent>`).

Three layers prevent orphan sessions:

1. **Pre-sweep** (TestMain): `KillAllTestSessions()` kills all
   `gc-gctest-*` sessions from prior crashed runs.
2. **Per-test** (`t.Cleanup`): the `tmuxtest.Guard` kills sessions
   matching its specific city prefix.
3. **Post-sweep** (TestMain defer): final sweep after all tests.

#### The `tmuxtest.Guard` pattern

```go
guard := tmuxtest.NewGuard(t) // generates "gctest-a1b2c3d4", registers cleanup
cityDir := setupRunningCity(t, guard)

session := guard.SessionName("mayor") // "gc-gctest-a1b2c3d4-mayor"
if !guard.HasSession(session) { ... }
```

- `test/tmuxtest/guard.go` — reusable session guard helper
- `RequireTmux(t)` — skips test if tmux not installed
- `KillAllTestSessions(t)` — package-level sweep for TestMain

## Decision guide

| Question you're testing | Tier |
|---|---|
| Does `gc bead create` print the right output? | Testscript |
| Does `gc start` fail gracefully without tmux? | Testscript (`GC_SESSION=fail`) |
| Does `gc rig add` fail for a missing path? | Testscript (real missing path) |
| Does the beads store skip corrupted JSONL lines? | Unit test |
| Does hook return ErrConflict on double-claim? | Unit test |
| Does concurrent bead creation avoid corruption? | Unit test |
| Does startup roll back if step 3 of 5 fails? | Unit test |
| Does a real tmux session start and respond to send-keys? | Integration |

## Dependencies

| Package | Purpose |
|---|---|
| `testing` (stdlib) | `t.TempDir()`, `t.Run()`, subtests, build tags |
| `github.com/stretchr/testify` | `assert` and `require` — cleaner assertions |
| `github.com/rogpeppe/go-internal/testscript` | Tutorial regression from `.txtar` files |

## Test doubles

No mock libraries. No `gomock`. No `mockgen`. Every test double is a
hand-written concrete type that lives in the same package as the
interface it implements.

### The four test doubles

| Double | Interface | Package | Strategy |
|---|---|---|---|
| `session.Fake` | `session.Provider` | `internal/session` | In-memory state + spy + broken mode |
| `fsys.Fake` | `fsys.FS` | `internal/fsys` | In-memory maps + spy + per-path error injection |
| `agent.Fake` | `agent.Agent` | `internal/agent` | Configurable returns + spy + per-method error injection |
| `beads.MemStore` | `beads.Store` | `internal/beads` | Real logic, in-memory backing (also used by `FileStore` internally) |

### Spy pattern

Every fake records calls as `[]Call` structs. Tests verify both the
result AND the call sequence:

```go
f := agent.NewFake("mayor", "gc-city-mayor")
doAgentAttach(f, &stdout, &stderr)

// Verify call sequence: IsRunning → Start → Name → Attach.
want := []string{"IsRunning", "Start", "Name", "Attach"}
for i, c := range f.Calls {
    if c.Method != want[i] { ... }
}
```

### Error injection strategies

Three patterns, used where they fit:

**Per-path errors** (`fsys.Fake`) — fine-grained, fail specific operations:
```go
f := fsys.NewFake()
f.Errors["/city/rigs"] = fmt.Errorf("disk full")
```

**Per-method errors** (`agent.Fake`) — one error field per method:
```go
f := agent.NewFake("mayor", "gc-city-mayor")
f.StartErr = fmt.Errorf("boom")
```

**Modal errors** (`session.Fake`) — all-or-nothing broken mode:
```go
f := session.NewFailFake() // Start/Stop/Attach all return errors
```

### Compile-time interface checks

Every fake has a compile-time assertion in its test file:

```go
var _ Provider = (*Fake)(nil)
```

### Fakes live next to the interface

Fakes are exported types in the same package as their interface. This
makes them importable by cross-package unit tests (e.g., `cmd/gc`
imports `session.NewFake()`).

## The do*() function pattern

Every CLI command splits into two functions:

- **`cmdFoo()`** — wires up real dependencies (reads cwd, loads config,
  calls `newSessionProvider()`), then calls `doFoo()`.
- **`doFoo()`** — pure logic. Accepts all dependencies as arguments.
  Returns an exit code.

Unit tests call `doFoo()` directly with fakes:
```go
f := agent.NewFake("mayor", "gc-city-mayor")
code := doAgentAttach(f, &stdout, &stderr)
```

Testscript tests call `gc foo` which routes through `cmdFoo()` →
`doFoo()`.

### When to use each

| I want to test... | Call |
|---|---|
| Pure logic with injected failures | `doFoo()` with a fake |
| CLI output format, exit codes | `exec gc foo` in txtar |
| That the factory wiring is correct | `exec gc foo` in txtar with `GC_SESSION=fake` |

## Env var fakes for testscript

Testscript needs fakes too, but can't inject Go objects. The CLI has
factory functions that check env vars and return the appropriate
implementation.

**Current env vars:**

| Env var | Values | Factory | Used by |
|---|---|---|---|
| `GC_SESSION` | `fake`, `fail`, (absent) | `newSessionProvider()` in `cmd/gc/providers.go` | `cmd_start.go`, `cmd_stop.go`, `cmd_agent.go` |
| `GC_BEADS` | `file`, `bd`, (absent) | `beadsProvider()` in `cmd/gc/providers.go` | bead commands, `cmd_init.go`, `cmd_start.go` |
| `GC_DOLT` | `skip`, (absent) | N/A (checked inline) | dolt lifecycle in `cmd_init.go`, `cmd_start.go`, `cmd_stop.go` |

**Design rules for env var fakes:**
- The fake never reads env vars itself — the factory function does
- At most three modes per dependency: works, fails, real
- If you need more than two env vars to set up a test scenario, it
  belongs in a unit test, not testscript

## MemStore: real implementation, not a fake

`beads.MemStore` is not a test-only fake — it's a real `Store`
implementation backed by a slice. `FileStore` composes `MemStore`
internally for its in-memory state and adds persistence on top. This
makes `MemStore` usable both as a production building block and as a
test double for code that needs a `Store` without disk I/O.
