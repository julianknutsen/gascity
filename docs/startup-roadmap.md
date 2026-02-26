# Startup Roadmap: Gastown Steps Not Yet in Gas City

Created 2026-02-23 after implementing the 5-step startup sequence
(ensureFreshSession, waitForCommand, acceptBypass, waitForReady,
verifySurvived) in `internal/session/tmux/adapter.go`.

Gastown's `StartSession()` in `internal/session/lifecycle.go` has 15
steps. We implemented 5. This file tracks what's missing and when to
add each piece.

## Source reference

Gastown file: `/data/projects/gastown/internal/session/lifecycle.go`
Gas City file: `/data/projects/gascity/internal/session/tmux/adapter.go`

---

## Tutorial 02-03: Settings & Slash Commands

### EnsureSettingsForRole (gastown step 2)

- **What:** Installs provider-specific settings files (Claude's
  `settings.json`, OpenCode plugins) and provisions slash commands
  into the working directory.
- **Gastown code:** `runtime.EnsureSettingsForRole(settingsDir, workDir, role, runtimeConfig)`
  in `internal/runtime/runtime.go` lines 46-77.
- **Why we need it:** When agents need slash commands and hooks.
  Without it, agent starts with default settings instead of Gas
  City-specific ones.
- **When:** Tutorial 02 (named crew with hooks) or 03 (agent loop).

---

## Tutorial 04: Theme & Multi-Agent UX

### ConfigureGasTownSession (gastown step 9)

- **What:** Applies color theme, status bar format (rig/worker/role),
  dynamic status, mouse mode, and key bindings (mail, feed, agents,
  cycle-sessions). The entire tmux UX layer.
- **Gastown code:** `t.ConfigureGasTownSession(sessionID, theme, rig, agent, role)`
  in `internal/tmux/tmux.go` lines 1920-1948.
- **Suboperations:**
  - `ApplyTheme` — color scheme
  - `SetStatusFormat` — status bar with rig/worker/role info
  - `SetDynamicStatus` — live status updates
  - `SetMailClickBinding` — click to read mail
  - `SetFeedBinding` — feed key binding
  - `SetAgentsBinding` — agents panel binding
  - `SetCycleBindings` — cycle between agent panes
  - `EnableMouseMode` — mouse support
- **Why we need it:** Navigation with multiple agents. Without it,
  sessions are plain tmux — functional but hard to tell apart and
  no quick-nav between agents.
- **When:** Tutorial 04 (agent team, multiple agents).
- **Note:** We already extracted `internal/session/tmux/theme.go` with
  `AssignTheme`, `GetThemeByName`, `ThemeStyle`. The tmux operations
  themselves are in `tmux.go`. Wiring is the remaining work.

---

## Tutorial 05b: Health Monitoring (package deal)

These four steps are interdependent. They arrive together.

### SetRemainOnExit (gastown step 7)

- **What:** `tmux set-option -t <session> remain-on-exit on`. Pane
  stays visible after agent process dies instead of being destroyed.
- **Gastown code:** `t.SetRemainOnExit(sessionID, true)` — tmux.go
  lines 2024-2035.
- **Why we need it:** Without it, crashed agent's pane vanishes — you
  lose error output, and respawn-pane has nothing to respawn into.
- **Prerequisite for:** SetAutoRespawnHook.

### SetEnvironment post-create (gastown step 8)

- **What:** After session creation, calls `tmux set-environment` for
  GT_ROLE, GT_RIG, GT_AGENT, GT_PROCESS_NAMES, GT_TOWN_ROOT, etc.
  This is SEPARATE from `-e` flags on `new-session`.
- **Gastown code:** Loop over `config.AgentEnv()` +
  `mergeRuntimeLivenessEnv()` calling `t.SetEnvironment(session, k, v)`
  — lifecycle.go step 8, tmux.go lines 1451-1455.
- **Critical subtlety:** `-e` flags set env for the INITIAL shell
  process only. `set-environment` sets SESSION-level env inherited by
  ALL future processes — including respawned panes. When auto-respawn
  restarts an agent, it gets `set-environment` vars but NOT the
  original `-e` flags.
- **Why we need it:** Prerequisite for auto-respawn to work correctly.
  Without it, respawned agents lose GC_AGENT and other identity vars.
- **Prerequisite for:** SetAutoRespawnHook (respawned process needs env).

### SetAutoRespawnHook (gastown step 11)

- **What:** Sets tmux `pane-died` hook: sleep 3s -> `respawn-pane -k`
  -> re-enable remain-on-exit (because respawn-pane resets it to off).
  This is the "let it crash" / Erlang supervisor mechanism.
- **Gastown code:** `t.SetAutoRespawnHook(sessionID)` — tmux.go lines
  2368-2403.
- **Hook command:** `run-shell "sleep 3 && tmux respawn-pane -k -t '<session>' && tmux set-option -t '<session>' remain-on-exit on"`
- **Dependencies:** Requires SetRemainOnExit + SetEnvironment.
- **PATCH-010 reference:** Fixes Deacon crash loop.
- **Why we need it:** Dead agents stay dead without it. For daemon mode
  with unattended agents, this is critical.

### TrackSessionPID (gastown step 15)

- **What:** Captures pane PID + process start time, writes to
  `.runtime/pids/<session>.pid`. Defense-in-depth for orphan cleanup.
- **Gastown code:** `TrackSessionPID(townRoot, sessionID, t)` —
  `internal/session/pidtrack.go` lines 36-56.
- **Why we need it:** If tmux itself dies or KillSession fails, the
  controller can find and kill orphaned processes by PID.
- **When:** Arrives with health monitoring / daemon mode.

---

## Not needed (permanent exclusions or handled differently)

| Gastown step | Why not needed |
|---|---|
| Step 1: ResolveRoleAgentConfig | Done in CLI via `config.ResolveProvider()` |
| Steps 3-5: Build command + env | Done in `agent.managed.Start()` + `cmd_start.go` |
| Step 13: SleepForReadyDelay | Handled inside `WaitForRuntimeReady` fallback |
| Step 4: ConfigDirEnv prepend | Gas City uses `-e` flags; revisit with respawn |

---

## Implementation notes

When implementing the 05b health monitoring cluster:

1. **Order matters:** SetRemainOnExit FIRST, then SetEnvironment,
   then SetAutoRespawnHook. Respawn depends on the other two.

2. **SetEnvironment must include GC_AGENT** (our equivalent of
   GT_ROLE/GT_AGENT) so respawned processes know their identity.

3. **The `-e` flags become insufficient** once respawn exists. We'll
   need to either:
   - Add post-create `set-environment` calls (gastown approach), OR
   - Bake env into the respawn command itself
   The gastown approach is cleaner.

4. **KillSessionWithProcesses** (gastown uses this instead of plain
   KillSession for cleanup) — kills descendant processes before
   killing the session. Important when agents spawn child processes.
   Currently we use simple KillSession.

5. **Add these to startOps interface** so they remain unit-testable
   via fakeStartOps.
