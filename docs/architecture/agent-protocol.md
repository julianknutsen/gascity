# Agent Protocol

<!--
Current-state architecture document. Describes how the Agent Protocol
works TODAY. For proposed changes, write a design doc in docs/design/.

Audience: Gas City contributors (human and LLM agent).
Update this document when the implementation changes.
-->

> Last verified against code: 2026-03-01

## Summary

The Agent Protocol is a Layer 0-1 primitive that provides identity,
lifecycle management, communication, and liveness detection for agents
regardless of the underlying session technology. It is composed of two
interface layers: the `Agent` interface (identity + high-level
operations) and the `session.Provider` interface (low-level session
management with 16 methods). Three production providers -- tmux,
subprocess, and Kubernetes -- implement the session layer, while the
agent layer composes over any provider uniformly.

## Key Concepts

- **Agent**: An identity-bearing wrapper around a session provider.
  Exposes Name, SessionName, IsRunning, Start, Stop, Attach, Nudge,
  Peek, and SessionConfig. The CLI layer builds agents from config; the
  controller operates on them without knowing the session implementation.
  Defined in `internal/agent/agent.go`.

- **Session Provider**: The interface for managing agent sessions.
  Covers lifecycle (Start, Stop, Interrupt), querying (IsRunning,
  ProcessAlive), communication (Attach, Nudge, SendKeys, Peek),
  metadata (SetMeta, GetMeta, RemoveMeta), and utilities (ListRunning,
  GetLastActivity, ClearScrollback, CopyTo). Defined in
  `internal/session/session.go`.

- **StartupHints**: A configuration struct that carries provider startup
  behavior from config resolution through to session creation. All
  fields are optional -- zero values mean fire-and-forget with no
  special startup handling. Includes readiness detection parameters,
  pre-start commands, session setup scripts, overlay directories, and
  copy file lists. Defined in `internal/agent/agent.go`.

- **Beacon**: A startup identification string embedded in the agent's
  initial prompt. Format: `[city-name] agent-name . timestamp`.
  Makes crashed agent sessions discoverable in resume pickers (e.g.,
  Claude Code's `/resume`). Optionally includes a `gc prime`
  instruction for non-hook agents. Defined in
  `internal/session/beacon.go`.

- **Dialog**: An automated dismissal mechanism for interactive startup
  prompts that would otherwise block agent sessions. Handles workspace
  trust dialogs and bypass-permissions warnings using peek + sendKeys.
  Idempotent: safe to call on sessions without dialogs. Defined in
  `internal/session/dialog.go`.

- **Fingerprint**: A deterministic SHA-256 hash of config fields that
  define an agent's behavioral identity (command, env, fingerprint
  extras). Used by reconciliation to detect config drift without
  comparing full structs. Same config always produces the same hash
  regardless of map iteration order. Defined in
  `internal/session/fingerprint.go`.

- **Session Metadata**: A key-value store associated with each session,
  used for cross-session signaling. The controller stores config
  fingerprints (`GC_CONFIG_HASH`) for drift detection and drain signals
  (`GC_DRAINED`) for pool scaling. Backed by the provider's SetMeta /
  GetMeta / RemoveMeta methods.

## Architecture

The Agent Protocol has two layers: the `Agent` interface in
`internal/agent/` wraps a `session.Provider` from `internal/session/`.
This separation ensures that higher-level code (the controller,
reconciliation, health patrol) depends only on `Agent`, while the
session implementation details are confined to provider packages.

```
                ┌──────────────────────────────────────────┐
                │   Controller / CLI commands               │
                │   (cmd/gc/)                               │
                │                                          │
                │   operates on agent.Agent                │
                └──────────┬───────────────────────────────┘
                           │
                ┌──────────▼───────────────────────────────┐
                │   agent.Agent interface                   │
                │   internal/agent/agent.go                 │
                │                                          │
                │   Name · SessionName · IsRunning          │
                │   Start · Stop · Attach · Nudge · Peek    │
                │   SessionConfig                          │
                └──────────┬───────────────────────────────┘
                           │ delegates to
                ┌──────────▼───────────────────────────────┐
                │   session.Provider interface              │
                │   internal/session/session.go             │
                │                                          │
                │   16 methods across 5 categories          │
                └──┬──────────┬───────────────┬────────────┘
                   │          │               │
            ┌──────▼──┐ ┌────▼─────┐  ┌──────▼──────┐
            │  tmux   │ │subprocess│  │     k8s     │
            │Provider │ │ Provider │  │  Provider   │
            └─────────┘ └──────────┘  └─────────────┘
```

### Data Flow

The most common operation is starting an agent. End-to-end:

1. **Config resolution** (`cmd/gc/`). The CLI resolves provider
   presets, overlays, patches, and overrides into a flat `config.Agent`
   struct. Template variables are expanded for session_setup commands.

2. **Agent construction** (`agent.New()`). Creates a `managed` struct
   holding name, session provider, command, prompt, env, hints, work
   directory, and fingerprint extras. Computes the session name via
   `SessionNameFor()`.

3. **Start** (`managed.Start()`). Builds a `session.Config` from the
   agent's fields (command + shell-quoted prompt, env, all hints) and
   calls `provider.Start(sessionName, config)`.

4. **Provider Start** (e.g., `tmux.Provider.Start()`). Executes the
   multi-step startup sequence:
   - Run PreStart commands on the target filesystem
   - Copy overlay directory into work directory (additive, no overwrite)
   - Copy CopyFiles entries into work directory
   - Create the session (e.g., `tmux new-session -d`)
   - Run SessionSetup commands after session creation
   - Run SessionSetupScript if configured
   - Detect readiness via ReadyPromptPrefix polling or ReadyDelayMs
   - Dismiss startup dialogs (workspace trust, permission warnings)
   - Send Nudge text if configured

5. **Liveness check** (`managed.IsRunning()`). Two-layer verification:
   first `provider.IsRunning(sessionName)` checks session existence,
   then `provider.ProcessAlive(sessionName, processNames)` checks the
   agent process tree. Both must return true.

6. **Communication**. Nudge sends text + Enter to wake agents.
   SendKeys sends raw keystrokes for dialog dismissal. Peek captures
   scrollback for crash forensics.

7. **Stop** (`managed.Stop()`). Delegates to `provider.Stop()`, which
   destroys the session and cleans up resources. Idempotent: returns
   nil if the session does not exist.

### Key Types

- **`Agent`** (`internal/agent/agent.go`): Interface with 9 methods.
  The `managed` struct is the sole production implementation.

- **`session.Provider`** (`internal/session/session.go`): Interface with
  16 methods across 5 categories: lifecycle (Start, Stop, Interrupt),
  querying (IsRunning, ProcessAlive), communication (Attach, Nudge,
  SendKeys, Peek), metadata (SetMeta, GetMeta, RemoveMeta), and
  utilities (ListRunning, GetLastActivity, ClearScrollback, CopyTo).

- **`session.Config`** (`internal/session/session.go`): Value struct
  carrying all parameters for starting a session: WorkDir, Command,
  Env, startup hints (ReadyPromptPrefix, ReadyDelayMs, ProcessNames,
  EmitsPermissionWarning, Nudge), lifecycle commands (PreStart,
  SessionSetup, SessionSetupScript), file staging (OverlayDir,
  CopyFiles), and FingerprintExtra.

- **`StartupHints`** (`internal/agent/agent.go`): Value struct bridging
  config fields to session.Config. Includes ReadyPromptPrefix,
  ReadyDelayMs, ProcessNames, EmitsPermissionWarning, Nudge, PreStart,
  SessionSetup, SessionSetupScript, OverlayDir, and CopyFiles.

- **`CopyEntry`** (`internal/session/session.go`): Describes a file or
  directory to stage in the session's working directory before the
  agent command starts. Fields: Src (host-side path) and RelDst
  (destination relative to session work directory).

### Session Providers

**tmux** (`internal/session/tmux/`): The primary production provider.
Wraps the tmux CLI to create detached sessions. The `Provider` struct
in `internal/session/tmux/adapter.go` adapts the lower-level `Tmux`
struct (in `internal/session/tmux/tmux.go`) to the `session.Provider`
interface. Features: full startup hint support with readiness polling,
interactive attach, scrollback capture via `capture-pane`, per-session
nudge serialization (channel-based semaphores to prevent interleaving),
process tree inspection for zombie detection, session metadata via tmux
environment variables, and status bar theming.

**subprocess** (`internal/session/subprocess/`): A lightweight
alternative using `os/exec` child processes with no terminal attached.
Suitable for CI, testing, and environments without tmux. Process
tracking uses two layers: in-memory (same gc process) and PID files
(cross-process persistence). Limitations: no interactive attach (always
returns error), no startup hint support (fire-and-forget only).

**Kubernetes** (`internal/session/k8s/`): A native Kubernetes provider
using client-go. Each session maps to a pod in the configured namespace.
Eliminates subprocess overhead by making direct API calls over reused
HTTP/2 connections. Pod manifests are compatible with the gc-session-k8s
external adapter. Configuration via environment variables:
GC_K8S_NAMESPACE, GC_K8S_IMAGE, GC_K8S_CONTEXT, and resource
request/limit settings. The `k8sOps` interface abstracts Kubernetes API
calls for testability.

### Session Naming

Session names are computed by `SessionNameFor()` in
`internal/agent/agent.go`. The function is the single source of truth
for the naming convention.

Default pattern: `gc-{cityName}-{sanitizedAgentName}`

- Slash characters in agent names are replaced with `--` for tmux
  safety: `hello-world/worker` becomes `gc-city-hello-world--worker`.

Custom templates use Go `text/template` syntax with four variables:

| Variable | Description | Example |
|---|---|---|
| `{{.City}}` | Workspace name | `bright-lights` |
| `{{.Agent}}` | Tmux-safe qualified name (/ replaced with --) | `hello-world--worker` |
| `{{.Dir}}` | Rig/dir component (empty for singletons) | `hello-world` |
| `{{.Name}}` | Bare agent name | `worker` |

Invalid or failing templates fall back to the default pattern with a
warning on stderr.

### Fingerprint Computation

`ConfigFingerprint()` in `internal/session/fingerprint.go` produces a
hex-encoded SHA-256 hash from:

1. `Config.Command` (null-byte separated)
2. `Config.Env` (sorted-key deterministic order)
3. `Config.FingerprintExtra` (prefixed with "fp" to avoid key collisions
   with Env)

Excluded from the hash (observation-only hints that do not change agent
behavior): WorkDir, ReadyPromptPrefix, ReadyDelayMs, ProcessNames,
EmitsPermissionWarning, Nudge, PreStart, SessionSetup, OverlayDir,
CopyFiles.

The controller stores the fingerprint via `SetMeta(sessionName,
"GC_CONFIG_HASH", hash)` after a successful start, and compares it on
each reconciliation tick to detect drift.

### Beacon Format

`FormatBeacon()` in `internal/session/beacon.go` produces:

```
[city-name] agent-name . 2026-03-01T15:04:05
```

When `includePrimeInstruction` is true (for non-hook agents):

```
[city-name] agent-name . 2026-03-01T15:04:05

Run `gc prime $GC_AGENT` to initialize your context.
```

The beacon appears in the agent's initial prompt so that crashed
sessions are identifiable in the session provider's resume picker.

### Dialog Dismissal

`AcceptStartupDialogs()` in `internal/session/dialog.go` handles two
blocking dialogs in sequence:

1. **Workspace trust dialog**: Detects "trust this folder" or "Quick
   safety check" in session output. Sends Enter to accept the
   pre-selected "Yes, I trust this folder" option.

2. **Bypass permissions warning**: Detects "Bypass Permissions mode" in
   session output. Sends Down (to select "Yes, I accept") then Enter.

Both checks use peek (to read session output) and sendKeys (to send
raw keystrokes). The function is idempotent -- if no dialog is detected,
it returns immediately.

## Invariants

These properties must hold for the Agent Protocol to be correct.
Violations indicate bugs.

- **Session name uniqueness**: `SessionNameFor()` produces distinct
  session names for distinct (cityName, agentName) pairs. Two agents in
  the same city never collide. The session name is the primary key for
  all provider operations.

- **No PID files for liveness decisions**: Agent liveness is determined
  by querying `provider.IsRunning()` and `provider.ProcessAlive()`,
  which inspect the live process tree. No status files, lock files, or
  PID files are consulted. The process table is the single source of
  truth for "what is running."

- **Two-layer liveness check**: `managed.IsRunning()` requires both
  `provider.IsRunning(sessionName)` (session exists) AND
  `provider.ProcessAlive(sessionName, processNames)` (agent process
  alive in session). A session with a dead agent process (zombie)
  correctly reports as not running when processNames are configured.

- **ProcessAlive with empty names returns true**: When no
  `processNames` are configured, `ProcessAlive()` returns true
  (no deeper check is possible). This avoids false negatives for
  agents where the process name is unknown.

- **Stop is idempotent**: `provider.Stop()` returns nil if the session
  does not exist. Calling Stop on an already-stopped agent is a no-op.

- **Fingerprint is deterministic**: `ConfigFingerprint()` produces
  identical hashes for identical configs regardless of Go map iteration
  order. Map keys are sorted before hashing.

- **Fingerprint excludes observation hints**: Changes to
  ReadyPromptPrefix, ReadyDelayMs, ProcessNames, and other
  observation-only fields do not trigger agent restarts. Only behavioral
  identity fields (command, env, fingerprint extras) participate.

- **Overlay is additive**: Overlay directory contents are copied into
  the work directory without overwriting existing files. This prevents
  overlays from clobbering user modifications.

- **No role names in Go code**: The Agent Protocol operates on
  `agent.Agent` values constructed from config. No line of Go references
  a specific role name.

- **Zero startup hints means fire-and-forget**: An agent with all-zero
  `StartupHints` gets no readiness detection, no dialog dismissal, no
  pre-start commands, and no overlay installation. The session is
  created and the agent command runs immediately.

## Interactions

| Depends on | How |
|---|---|
| `internal/session` | `Provider` interface for all session operations. `Config` and `CopyEntry` structs for session parameterization. `ConfigFingerprint()` for drift detection. `FormatBeacon()` for session identification. `AcceptStartupDialogs()` for dialog dismissal. |
| `internal/overlay` | `CopyDir()` for copying overlay directory trees into agent work directories at startup. Used by tmux and subprocess providers. |
| `internal/config` | `Agent` struct provides the source data for agent construction: name, command, args, prompt template, env, hints, pool config, overlay dir, session setup. `Workspace.SessionTemplate` provides custom session naming. |

| Depended on by | How |
|---|---|
| `cmd/gc/controller.go` | Builds `agent.Agent` slices from config and passes them to reconciliation. Calls `IsRunning()`, `Start()`, `Stop()`, `SessionConfig()` on each tick. |
| `cmd/gc/reconcile.go` | Calls `agent.SessionConfig()` to compute fingerprints for drift detection. Calls `Start()` and `Stop()` for corrective actions. |
| `cmd/gc/cmd_start.go` | Entry point for `gc start` which constructs agents and starts the controller. |
| `cmd/gc/cmd_attach.go` | Calls `agent.Attach()` to connect the user's terminal to a running session. |
| `cmd/gc/cmd_nudge.go` | Calls `agent.Nudge()` to send messages to running agents. |
| `cmd/gc/cmd_peek.go` | Calls `agent.Peek()` to capture session output for inspection. |
| `cmd/gc/cmd_stop.go` | Calls `agent.Stop()` for individual agent shutdown. |
| `cmd/gc/pool.go` | Constructs pool member agents by deep-copying config and calling `agent.New()` for each instance. |
| Health Patrol | Queries `IsRunning()` for liveness, `Peek()` for crash forensics, `SessionConfig()` for fingerprinting. See [Health Patrol architecture](./health-patrol.md) for the full supervision model. |

## Code Map

| File | Responsibility |
|---|---|
| `internal/agent/agent.go` | `Agent` interface, `managed` implementation, `New()` constructor, `StartupHints` struct, `SessionNameFor()` session naming, `shellQuote()` |
| `internal/agent/fake.go` | `Fake` test double for `Agent` with spy recording and configurable errors |
| `internal/session/session.go` | `Provider` interface (16 methods), `Config` struct, `CopyEntry` struct |
| `internal/session/beacon.go` | `FormatBeacon()` and `FormatBeaconAt()` for startup identification strings |
| `internal/session/dialog.go` | `AcceptStartupDialogs()` for automated dialog dismissal (workspace trust, permission warnings) |
| `internal/session/fingerprint.go` | `ConfigFingerprint()` deterministic SHA-256 hashing for config drift detection |
| `internal/session/fake.go` | `Fake` test double for `Provider` with spy, configurable errors, zombie simulation, and canned peek/activity output |
| `internal/session/tmux/adapter.go` | `tmux.Provider` adapting `Tmux` to `session.Provider` |
| `internal/session/tmux/tmux.go` | `Tmux` struct: tmux CLI wrapper with session creation, nudge serialization, readiness polling, process tree inspection |
| `internal/session/subprocess/subprocess.go` | `subprocess.Provider`: child process management with PID file persistence |
| `internal/session/k8s/provider.go` | `k8s.Provider`: native Kubernetes provider using client-go |
| `internal/session/k8s/exec.go` | `k8sOps` interface abstracting Kubernetes API calls for testability |
| `internal/overlay/` | `CopyDir()` for recursive additive directory copying |

## Configuration

Agent Protocol behavior is configured via `[[agents]]` entries and
`[workspace]` settings in `city.toml`:

```toml
[workspace]
name = "my-city"
session_template = "gc-{{.City}}-{{.Agent}}"  # custom session naming

[[agents]]
name = "worker"
provider = "claude"                       # provider preset name
start_command = "claude"                  # overrides provider default
args = ["--dangerously-skip-permissions"] # appended to command
dir = "my-project"                        # working directory (rig name)
prompt_template = "prompts/worker.md"     # path to prompt template
prompt_mode = "arg"                       # "arg", "flag", or "none"
nudge = "Check your hook for new work."   # text sent after startup
env = { GC_CUSTOM = "value" }            # extra environment variables

# Startup hints (all optional)
ready_prompt_prefix = "> "               # readiness detection
ready_delay_ms = 5000                    # fallback fixed delay
process_names = ["claude", "node"]       # liveness check targets
emits_permission_warning = true          # auto-dismiss permission dialog

# Lifecycle commands
pre_start = ["mkdir -p /tmp/workspace"]  # run before session creation
session_setup = ["gc hook install {{.Session}}"]  # run after creation
session_setup_script = "scripts/setup.sh"         # script after setup

# File staging
overlay_dir = "overlays/worker"          # additive copy into workdir
```

Provider presets are defined in `[providers]` and supply defaults for
start_command, args, ready_prompt_prefix, ready_delay_ms,
process_names, and emits_permission_warning. Per-agent fields override
provider defaults. See [Config architecture](./config.md) for the full
override resolution chain.

## Testing

The Agent Protocol has comprehensive unit tests at both layers:

| Test file | Coverage |
|---|---|
| `internal/agent/agent_test.go` | Session naming (default + custom templates + fallback), agent construction, Start/Stop/Attach/Nudge delegation, prompt shell quoting, hint passthrough, SessionConfig computation, WorkDir propagation, OverlayDir propagation, prompt_mode=none behavior, zombie detection |
| `internal/agent/fake_test.go` | Fake agent test double correctness |
| `internal/session/fake_test.go` | Fake provider spy recording and state simulation |
| `internal/session/fake_conformance_test.go` | Conformance tests ensuring Fake behaves like production providers |
| `internal/session/beacon_test.go` | Beacon formatting with and without prime instruction |
| `internal/session/fingerprint_test.go` | Deterministic hashing, env key sorting, FingerprintExtra inclusion/exclusion |
| `internal/session/tmux/adapter_test.go` | Tmux adapter delegation |
| `internal/session/tmux/tmux_test.go` | Tmux operations: session create/destroy, nudge serialization, readiness detection, process tree inspection |
| `internal/session/tmux/startup_test.go` | Multi-step startup sequence: overlay copy, dialog dismissal, readiness polling |
| `internal/session/k8s/provider_test.go` | K8s provider: pod creation, lifecycle, metadata, and all Provider methods via faked k8sOps |

All unit tests use in-memory fakes (`session.Fake`, `agent.Fake`) with
no external infrastructure dependencies. Integration tests requiring
real tmux are in `internal/session/tmux/` with build tags. See
[TESTING.md](../../TESTING.md) for the overall testing philosophy and
tier boundaries.

## Known Limitations

- **No hot-swap of session provider**: The provider is selected at
  startup and cannot be changed without restarting the controller. All
  agents in a city share the same provider.

- **Subprocess provider has no attach**: `subprocess.Provider.Attach()`
  always returns an error because there is no terminal to connect to.
  Use tmux for interactive debugging.

- **Subprocess provider ignores startup hints**: ReadyPromptPrefix,
  ProcessNames, dialog dismissal, and all other startup hint features
  are tmux-specific. Subprocess sessions are fire-and-forget only.

- **Dialog dismissal is provider-specific**: The dialog dismissal logic
  in `AcceptStartupDialogs()` depends on peek + sendKeys, which are
  tmux-specific operations. Other providers either skip dialog handling
  or implement it differently.

- **Session metadata is not persistent across provider restarts**: For
  the tmux provider, metadata is stored as tmux environment variables
  on the session. If tmux itself crashes, all metadata is lost. The
  controller recovers by restarting agents (which recomputes
  fingerprints).

- **Nudge serialization is tmux-only**: The per-session nudge lock that
  prevents interleaving is implemented only in the tmux provider.
  Concurrent nudges to subprocess or k8s sessions may interleave.

- **roleEmoji map in tmux.go**: The tmux package contains a hardcoded
  `roleEmoji` map for status bar display, inherited from the Gas Town
  codebase. This is cosmetic only (tmux status bar) and does not affect
  any protocol behavior or dispatch logic. Scheduled for removal.

## See Also

- [Architecture glossary](./glossary.md) -- authoritative definitions
  of all Gas City terms used in this document
- [Health Patrol architecture](./health-patrol.md) -- the supervision
  model that depends on Agent Protocol for liveness detection, restart,
  and crash forensics
- [Config architecture](./config.md) -- override resolution chain that
  produces the `config.Agent` structs used to construct agents
- [Session Provider interface](../../internal/session/session.go) --
  the 16-method interface that all providers implement
- [Agent interface and StartupHints](../../internal/agent/agent.go) --
  the high-level agent abstraction with session naming
- [Config fingerprinting](../../internal/session/fingerprint.go) --
  SHA-256 hashing for drift detection
- [Overlay copying](../../internal/overlay/) -- recursive additive
  directory copy used during agent startup
