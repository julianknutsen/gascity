# Worktree Isolation Roadmap

## Current state (what's implemented)

Gas City has basic worktree isolation:

- `isolation = "worktree"` on agents creates per-agent git worktrees
- Pool agents each get separate worktrees (worker-1, worker-2, etc.)
- Worktrees live under `.gc/worktrees/<rig>/<agent>/`
- Branch naming: `gc/<agent>-<base36-nanos>`
- `.beads/redirect` files point worktrees to shared bead stores
- `createAgentWorktree` is idempotent (reuses existing worktrees)
- `gc stop` calls `cleanupWorktrees` which destroys all worktrees

## The problem: worktrees must persist across restarts

Gas Town distinguishes **pause** vs **permanent shutdown**:

| Gas Town command | Sessions | Worktrees | Beads | Gas City equivalent |
|------------------|----------|-----------|-------|---------------------|
| `gt down` (pause) | Killed | **Preserved** | Preserved | `gc stop` |
| `gt up` (resume) | Re-created | **Auto-discovered** | Read | `gc start` |
| `gt shutdown` | Killed | **Removed** | Preserved | `gc stop --clean` (future) |

**The current `gc stop` does `shutdown` behavior (removes worktrees).
It should do `down` behavior (preserve worktrees) by default.**

### Why worktrees must persist

1. **Crash recovery.** Agent crashes mid-task. Its worktree has uncommitted
   changes representing hours of work. `gc stop && gc start` must not
   destroy that work.

2. **Context continuity.** A restarted agent picks up where it left off.
   Its worktree has the right branch checked out, local changes present,
   bead hook pointing to the current task.

3. **Pool scaling.** Pool scales down from 5 → 2 agents. The 3 scaled-down
   worktrees may have in-flight work. Destroying them immediately loses
   that work. (Gas Town uses drain timeouts for this.)

4. **NDI (Nondeterministic Idempotence).** Sessions come and go; the work
   survives. Worktrees + beads are the persistent substrate. Multiple
   independent observers can reconcile the same state.

### How Gas Town handles restart discovery

Gas Town uses **filesystem discovery** (ZFC: no status files):

1. On `gt up`, scan `polecats/<name>/` directories for existing worktrees
2. Scan tmux for existing sessions
3. Cross-reference with beads agent registry (`hook_bead` field)
4. If worktree exists + bead has work assigned → reuse worktree, start session
5. If worktree exists + no work assigned → available for new assignment
6. If session exists but worktree gone → kill orphan session

Key code: `ReconcilePool()` in `gastown/internal/polecat/manager.go`

## Fix needed: make `gc stop` preserve worktrees

### Change 1: `gc stop` preserves worktrees (default)

Remove the `cleanupWorktrees` call from `cmdStop`. Worktrees survive
between `gc stop` and `gc start`.

### Change 2: `gc stop --clean` removes worktrees

Add a `--clean` flag that triggers the destructive cleanup path.
This is the "I'm done, tear everything down" operation.

### Change 3: `gc start` reuses existing worktrees (already works)

`createAgentWorktree` is already idempotent — if the directory exists,
it reads the current branch and returns. No code change needed.

### Change 4: Worktree safety check before cleanup

Before removing a worktree (in `--clean` mode or pool drain), check
for uncommitted changes:

```go
func hasUncommittedWork(wtPath string) bool {
    g := git.New(wtPath)
    // git status --porcelain → non-empty means dirty
    // git log @{u}..HEAD → unpushed commits
}
```

If dirty, warn on stderr instead of silently destroying work.

## Roadmap: what to build when

### Now (post-commit fix)

- [ ] Remove `cleanupWorktrees` from `gc stop` (preserve by default)
- [ ] Add `--clean` flag to `gc stop` for explicit cleanup
- [ ] Add `hasUncommittedWork()` safety check to `internal/git`

### Tutorial 03 (Ralph Loop)

- [ ] Auto-respawn on crash (tmux pane-died hook)
- [ ] Worktree persists across agent restarts within a running city
- [ ] Agent discovers its worktree path from `GC_DIR` env var

### Tutorial 04 (Agent Team)

- [ ] Cross-rig worktrees (crew pattern)
- [ ] Identity preservation across rigs (env vars + git config)
- [ ] `gc worktree add/remove/list` commands for crew management

### Tutorial 05a (Formulas)

- [ ] Merge-back flow: formula step that merges worktree branch → main
- [ ] Post-merge worktree cleanup (only after confirmed merge)

### Tutorial 05b (Health Patrol)

- [ ] Zombie detection: tmux session alive but agent process dead
- [ ] Stale hook scanning: dead agent + hooked bead → unhook
- [ ] **Worktree safety check before unhook** (check for partial work)
- [ ] PID tracking for orphan process cleanup
- [ ] Mass death detection (multiple agents dying rapidly)

### Tutorial 05d (Full Orchestration)

- [ ] Bare repo pattern (`.repo.git` + worktrees) for efficiency
- [ ] Full worktree lifecycle: create → work → merge → cleanup
- [ ] Name pool recycling (release polecat names after cleanup)

## Key design patterns from Gas Town

### 1. Filesystem as source of truth (ZFC)

No worktree registry files. Discovery by scanning directories +
`git worktree list`. Consistent with CLAUDE.md's "No status files"
rule.

### 2. Beads track work assignments, not worktree state

The bead's `hook` field says "agent X is working on task Y."
The worktree is an implementation detail. If the worktree is gone
but the hook exists, something is wrong → flag for recovery.

### 3. Two-phase cleanup

1. **Soft stop** (`gt down` / `gc stop`): kill sessions, keep worktrees
2. **Hard stop** (`gt shutdown` / `gc stop --clean`): kill sessions,
   verify no partial work, remove worktrees, prune git

### 4. Stale hook = dead session + hooked bead

Before unhooking:
1. Check if agent session is alive (tmux + process check)
2. Check if worktree has uncommitted changes
3. If dead + clean → unhook bead (status → open, reassignable)
4. If dead + dirty → warn, don't unhook (manual recovery needed)

### 5. Startup beacon for context restoration

When a session restarts in an existing worktree, the startup command
includes a "beacon" — context about what work was assigned. This lets
the agent resume without losing track of its current task.

In Gas City, this maps to prompt templates reading from beads:
the agent's prompt includes its current hook status.

## Gas Town reference files

| Concept | Gas Town file | Key function |
|---------|---------------|--------------|
| Worktree preserved on down | internal/cmd/down.go | runDown() |
| Auto-discovery on up | internal/polecat/manager.go:1289 | ReconcilePool() |
| Session stale check | internal/polecat/session_manager.go:448 | Start() |
| Agent bead registry | internal/polecat/manager.go:1621 | loadPolecatState() |
| Crew worktree (permanent) | internal/cmd/worktree.go:96 | runWorktree() |
| Zombie detection | internal/doctor/zombie_check.go | isSessionProcessDead() |
| Stale hook scan | internal/deacon/stale_hooks.go:75 | scanStaleHooks() |
| Worktree safety check | internal/deacon/stale_hooks.go:191 | checkUncommittedWork() |
| PID tracking | internal/session/pidtrack.go | WritePID() / CheckPID() |
| Startup beacon | internal/session/startup.go | FormatStartupBeacon() |
