# Tutorial 01 — Hello, Gas City

Let's say that you're using Claude Code on a significant feature implementation.
You've described the feature, pointed the agent at the right files, and it's
making progress. Then — mid-flight — the context window fills up. The session is
either over or you're at the mercy of the compactation to save the important
details. This is the fundamental problem with AI coding agents: their memory is
the context window, and context windows are finite.

Gas City fixes this with **beads** — tracked work units that persist outside the
agent. When the agent uses beads to record what's done and what's left, running
out of context is no longer catastrophic. In fact, it can be beneficial to clear
out the context before it rots. A fresh session queries beads and picks up right
where the last one left off. The state is in the store, not in the agent's head.

This tutorial builds the simplest possible Gas City orchestration: a named agent
(the "Mayor") to capture the work, an anonymous coding agent to build it and
the beads system that ties it all together.

---

## Starting your city

First, start by installing Gas City. We do that on macOS with Home Brew:

```shell
$ brew install gascity
```

A city is a particular set of rules for how your orchestration works and a set
of projects configured for that orchestration. The configuration for a city is
stored in a folder on your computer set aside for that purpose. You can define
multiple cities with multiple configurations, but we'll start with just one for
now. By convention, each city goes into your home folder. Initialize one like
so:

```shell
$ gc init ~/bright-lights
Welcome to Gas City!
Initialized city "bright-lights" with default mayor agent.
```

This creates the city directory with everything you need: a `.gc/` runtime
directory, a `rigs/` directory for projects, prompt templates, and a
`city.toml` with a default mayor agent configured.

Now start the city to launch the mayor:

```shell
$ cd ~/bright-lights
$ gc start
City started.
```

Starting a city uses the configuration to ensure that you have the agents you
need to do your work. You can update the configuration at any time, stop and
restart your city for new configurations to take effect. If you specify no
configuration, you'll get the default which is what we'll use for the rest of
this tutorial.

## Adding a project

To associate a project (called a "rig") with a city, you add it from within
the city directory:

```shell
$ gc rig add ~/projects/tower-of-hanoi
Adding rig 'tower-of-hanoi'...
  Detected git repo at ~/projects/tower-of-hanoi
  Initialized beads database
Rig added.

$ gc rig list

Rigs in /Users/csells/bright-lights:

  bright-lights (HQ):
    Prefix: brightlights
    Beads:  initialized

  tower-of-hanoi:
    Path:   /Users/csells/projects/tower-of-hanoi
    Prefix: toh
    Beads:  initialized
```

The "gc rig" command needs to know which city you're talking about. The easiest
way to specify that is to be in the configuration folder of the city itself, but
you can also specify the city manually via the `--city` argment.

Because we're getting the default GC configuration, we have only a single agent
-- the Mayor -- which is who you'll talk to for planning and coordination.

## Create a Task

To give an agent something to do, you'll want to create a bead that represents a
task. You can do that manually or use the mayor to do that work. Creating a bead
manually looks like this:

```shell
# create the bead manually
$ gc bd create "build a Tower of Hanoi app"
Created bead: gc-1  (status: open)

# list the beads ready to work on
$ gc bd ready
ID    STATUS   ASSIGNEE   TITLE
gc-1  open     —          Build a Tower of Hanoi app
```

A new bead starts with a status of `open` — available for claiming. No assignee
yet.

```
$ gc bd show gc-1
ID:       gc-1
Status:   open
Type:     task
Title:    Build a Tower of Hanoi app
Rig:      tower-of-hanoi
Created:  2026-02-16 10:30:00
Assignee: —
```

> **Two things to notice:**
>
> 1. The bead has an ID (`gc-1`). Every bead in this city gets a unique ID.
> 2. The bead is stored on disk — not in the agent's context window. Agents come
>    and go. Beads persist.

We created this bead via the `gc` CLI. If you'd rather have a conversation
instead of remember the CLI args, you talk to the Mayor instead:

```shell
$ gc agent attach mayor
Attaching to agent 'mayor' (tmux session: bright-lights/mayor)...

╭────────────────────────────────────────╮
│ ✻ Welcome to Claude Code!              │
│   /help for help                       │
│                                        │
│   cwd: ~/projects/tower-of-hanoi       │
╰────────────────────────────────────────╯

You: Mr. Mayor, can you create a bead to build a Tower of Hanoi app? thanks!

Mayor: Sure! I'll create that bead for you.

  $ gc bd create "Build a Tower of Hanoi app"
  Created bead: gc-1  (status: open)

Done — gc-1 is in the backlog and ready for a crew member to pick up.

You: Can you list the ready beads?

Mayor: Of course.

  $ gc bd ready
  ID    STATUS   TITLE
  gc-1  open     Build a Tower of Hanoi app

Just the one bead in the backlog right now.
```

The act of "attaching" to the mayor via `gc agent attach` brings up the single
instance of that agent running in a tmux session. By using tmux, the mayor's
session is long-lived — it persists even if you close your terminal. You can
detach from it any time with `Ctrl-b d` and reattach later with `gc agent attach
mayor`. This is how Gas City keeps agents running in the background while you do
other things.

---

## Let's get to work!

Now let's use a CLI coding agent to pick up that work for our rig. Detach from
the mayor's session (`Ctrl-b d`) and start a coding agent in the rig directory.
It's a good practice to run your GC agents in a tmux session; in future
tutorials, we'll see the benefits of that.

Because you've added the rig to the "bright-lights" city, any agent that
supports the AGENTS.md (most of them) rules file (or CLAUDE.md for Claude Code)
has already been configured with the information it needs to understand tasks
expressed as beads:

```shell
$ tmux new -s tower-worker
$ cd ~/projects/tower-of-hanoi
$ codex   # or claude, gemini, etc.

Codex (research preview)
model: gpt5.3-codex

You: Can you check what beads are ready?

Codex: Checking...

  $ gc bd ready
  ID    STATUS   TITLE
  gc-1  open     Build a Tower of Hanoi app

There's one bead ready — gc-1, "Build a Tower of Hanoi app." Want me to
pick it up?

You: Yes, please!

Codex: On it. Let me start by setting up the project structure...
```

You can watch it build your app, or detach from the tmux session (`Ctrl-b d`)
and let it cook.

Check the bead status from another terminal any time you like:

```shell
$ gc bd list
ID    STATUS   ASSIGNEE                TITLE
gc-1  active   tower-of-hanoi-codex    Build a Tower of Hanoi app
```

When the agent finishes, it closes the bead:

```shell
$ gc bd close gc-1
Closed bead: gc-1

$ gc bd list
ID    STATUS   ASSIGNEE                TITLE
gc-1  closed   tower-of-hanoi-codex    Build a Tower of Hanoi app
```

That's it. The coding agent has now built your app. The bead records that the
work happened, who did it, and when it closed.

---

## Starting and stopping

When you're done for the day, stop the city:

```shell
$ gc stop
City stopped.
```

To resume later, start it again:

```shell
$ gc start
City started.
```

If you run `gc start` in a directory that isn't a city yet, it auto-initializes
one for you — so `gc start` in an empty directory is equivalent to `gc init`
followed by `gc start`.

---

## What You Learned

This tutorial used four of Gas City's five primitives:

| Primitive              | What You Used It For                                           |
| ---------------------- | -------------------------------------------------------------- |
| **Config**             | Default city configuration — one mayor, beads backend          |
| **Agent Protocol**     | `gc init` / `gc start` / `gc stop` / `gc agent attach` — managed the mayor |
| **Task Store (Beads)** | `gc bd create` / `gc bd list` — tracked the work               |
| **Prompt Templates**   | `gc prime` — gave agents their behavioral prompts at startup   |

The remaining primitive (Event Bus) isn't needed yet. It shows up when you have
multiple agents that need to observe each other. That's
[Tutorial 04](04-agent-team.md).

---

## What's Next

At this point, you've got yourself a working orchestration system. You can use
the mayor to create beads and hand them off to a coding agent on demand.

But right now you're doing the routing manually — you told codex to check beads
yourself. Ideally we'd like the agent to know it has outstanding work and to get
to it without any nudging from us.

In [Tutorial 02 — Named Crew](02-named-crew.md), you'll register named agents
on your rigs so the mayor can route work to them and they'll get to work as soon
as they're started.

---

## Spec Changes Needed

> This section tracks DX decisions from the tutorial that need to flow back
> into `gas-city-spec.md`. Don't delete until the spec is updated.

- **City-as-directory model.** A city is a folder (`~/bright-lights`), not a
  config file embedded in a project repo. `gc init <path>` creates a city at
  that directory; `gc start` boots it (auto-initing if needed). The spec
  currently assumes workspace.toml inside the project.

- **`gc rig add <path>`** — new command to associate a project with a city.
  Creates rig infrastructure (beads, routes). Not in the spec at all.

- **`gc rig list`** — new command to list rigs in a city. Not in the spec.

- **`gc agent attach <name>`** — starts or reattaches to a named agent's tmux
  session. Spec has `gc agent start` but not `attach`.

- **Default agent naming: `<rig-name>-<process-name>`** — when a coding agent
  picks up a bead without an explicit agent name, the assignee defaults to
  `<rig-name>-<cli-process-name>` (e.g. `tower-of-hanoi-codex`). Not in spec.

- **Mayor as overseer, not worker.** The default config creates a mayor whose
  role is planning and coordination, not coding. Workers are separate agents
  started in rig directories. Spec doesn't distinguish mayor from worker role.

- **`gc prime [agent-name]`** — new command that outputs the agent's behavioral
  prompt. Used via Claude Code's `--settings` flag: `gc init` writes a
  canonical `hooks/claude-settings.json` containing a SessionStart hook that
  calls `gc prime`. When `gc start` launches Claude Code agents, it passes
  `--settings <city>/hooks/claude-settings.json`. No AGENTS.md or CLAUDE.md
  is written into rigs — the prompt flows through `--settings` at launch time.

- **`gc init` / `gc start` semantics.** `gc init [path]` creates a complete
  city (like `git init`). `gc start [path]` boots it, auto-initing if needed.
  Spec doesn't distinguish init from start.

- **`gc bd claim` is implicit.** Agents pick up beads by working on them; the
  `open → active` transition happens internally. No explicit `gc bd claim`
  command needed in the basic flow.

- **City discovery via `.gc/` walk-up.** Commands find the city by walking up
  from cwd looking for a `.gc/` directory. No `--city` flag needed in the
  common case.

- **Tutorial reordering.** The original plan had Tutorial 02 = Ralph loop.
  New order: 02 = Named Crew + routing, 03 = Ralph Loop. The Ralph loop
  requires routing as a prerequisite so that beads land on the agent's hook
  and the loop just clears context and picks up the next hooked bead.

- **"Crew" terminology.** Named agents assigned to rigs. New concept not in
  spec. Relates to the existing agent config but adds rig-scoped naming.
