# Shareable Topologies

A practical guide for creating and consuming shareable topologies as
pluggable features in Gas City.

## What is a shareable topology?

A topology is a directory containing a `topology.toml` file and any
supporting assets (prompt templates, scripts, formulas). It defines a
reusable set of agents that can be composed into any city.

```
my-topology/
├── topology.toml        # agent definitions + metadata
├── prompts/
│   └── worker.md        # prompt templates (auto-resolved)
├── scripts/
│   └── setup.sh         # session setup scripts
└── formulas/            # optional formula directory
    └── code-review.toml
```

Topologies are self-contained: they carry everything their agents need.
Paths in `topology.toml` (prompt_template, session_setup_script,
overlay_dir) resolve relative to the topology directory, so the
topology works regardless of where it's referenced from.

Topologies can be:
- **Local directories** — referenced by relative or absolute path
- **Remote git repos** — fetched and cached via `[[topologies]]` source

## Creating a shareable topology

### topology.toml format

```toml
[topology]
name = "code-review"
version = "1.0.0"
schema = 1

[[agents]]
name = "reviewer"
prompt_template = "prompts/reviewer.md"
provider = "claude"

[agents.pool]
min = 0
max = 3

[[agents]]
name = "summarizer"
prompt_template = "prompts/summarizer.md"
provider = "claude"
```

Required metadata fields:
- **name** — identifier for the topology
- **schema** — format version (currently `1`)

Optional metadata:
- **version** — semver string for tracking
- **requires_gc** — minimum gc version
- **city_agents** — agent names that should be city-scoped (see below)

### Prompt templates and scripts

Reference prompts and scripts using paths relative to the topology
directory:

```toml
[[agents]]
name = "reviewer"
prompt_template = "prompts/reviewer.md"
session_setup_script = "scripts/setup.sh"
overlay_dir = "overlays/reviewer"
```

During expansion, Gas City rewrites these paths to absolute paths so
they work regardless of which city references the topology.

### Including formulas

Add a `[formulas]` section to include a formula directory:

```toml
[formulas]
dir = "formulas"
```

Formula directories participate in the layered formula resolution
system. Topology formulas are lower priority than city-local or
rig-local formulas, so consumers can override specific formulas.

### Including providers

Topologies can define provider presets that their agents depend on:

```toml
[providers.claude]
start_command = "claude --dangerously-skip-permissions"
```

Provider definitions merge additively — existing city providers are
not overwritten. This means the consumer's provider config takes
precedence.

### Dual-scope topologies (city_agents)

Some topologies define agents that should run at city scope (not per-rig)
alongside agents that run per-rig. Use `city_agents` to declare which
agents are city-scoped:

```toml
[topology]
name = "gastown"
schema = 1
city_agents = ["mayor", "deacon"]

[[agents]]
name = "mayor"
prompt_template = "prompts/mayor.md"

[[agents]]
name = "deacon"
prompt_template = "prompts/deacon.md"

[[agents]]
name = "polecat"
prompt_template = "prompts/polecat.md"

[agents.pool]
min = 0
max = 5
```

When this topology is referenced from both `workspace.topology` and a
rig's `topology`:
- City expansion keeps only `mayor` and `deacon` (dir="")
- Rig expansion keeps only `polecat` (dir=rig name)

## Consuming a shareable topology

### Local reference

Reference a topology directory by path in your `city.toml`:

```toml
# City-level (agents get dir="")
[workspace]
topology = "topologies/base"

# Or multiple city topologies
[workspace]
topologies = ["topologies/base", "topologies/monitoring"]

# Rig-level (agents get dir=rig name)
[[rigs]]
name = "my-project"
path = "/home/user/my-project"
topology = "topologies/gastown"

# Or multiple rig topologies
[[rigs]]
name = "my-project"
path = "/home/user/my-project"
topologies = ["topologies/base", "topologies/review"]
```

Relative paths resolve against the city directory (where `city.toml`
lives).

### Remote reference

Define named topology sources and reference them by name:

```toml
[topologies.gastown]
source = "https://github.com/example/gastown-topology.git"
ref = "v1.0.0"
path = "topology"  # subdirectory within the repo

[[rigs]]
name = "my-project"
path = "/home/user/my-project"
topology = "gastown"
```

Remote topologies are fetched once and cached in `.gc/topology-cache/`.
The cache key includes the source URL, ref, and path.

### Customizing topology agents

Use per-rig overrides to customize agents from a topology without
modifying the topology itself:

```toml
[[rigs]]
name = "my-project"
path = "/home/user/my-project"
topology = "topologies/gastown"

[[rigs.overrides]]
agent = "polecat"
provider = "gemini"
idle_timeout = "30m"

[rigs.overrides.env]
CUSTOM_VAR = "value"

[rigs.overrides.pool]
max = 10
```

Override fields (all optional):
- **provider** — change the agent's provider
- **suspended** — suspend/unsuspend the agent
- **idle_timeout** — change idle timeout
- **prompt_template** — replace the prompt template
- **start_command** — change the start command
- **nudge** — change the nudge text
- **env** / **env_remove** — add/remove environment variables
- **pool** — override pool settings (min, max, check, drain_timeout)
- **pre_start** — replace pre-start commands
- **session_setup** / **session_setup_script** — replace session setup
- **overlay_dir** — replace overlay directory
- **install_agent_hooks** — replace agent hook installation list

For city-level customization, use patches:

```toml
[[patches.agents]]
name = "mayor"
provider = "gemini"
```

## Handling name collisions

When two topologies define an agent with the same name and both apply to
the same scope (same rig, or both city-level), Gas City reports an error
with provenance:

```
rig "myrig": topologies define duplicate agent "worker":
  - topologies/base
  - topologies/extras
rename one agent in its topology.toml, or use separate rigs
```

### Resolution strategies

1. **Rename in topology.toml** — if you control the topology, change one
   agent's name to be unique.

2. **Use separate rigs** — apply each topology to a different rig. Since
   agent uniqueness is scoped to `(dir, name)`, the same agent name in
   different rigs is valid.

3. **Split the topology** — extract the conflicting agent into its own
   topology so you can choose which version to include.

## Example: composing three topologies

```toml
[workspace]
name = "full-stack-city"
provider = "claude"

# City-level topology for orchestration
[workspace]
topology = "topologies/orchestration"

# Remote topology source
[topologies.code-review]
source = "https://github.com/example/review-topology.git"
ref = "main"

# Provider presets
[providers.claude]
start_command = "claude --dangerously-skip-permissions"

[providers.gemini]
start_command = "gemini-cli"

# Rig with two composed topologies
[[rigs]]
name = "backend"
path = "/home/user/backend"
topologies = ["topologies/base-agents", "code-review"]

# Override the reviewer to use gemini
[[rigs.overrides]]
agent = "reviewer"
provider = "gemini"

# Second rig with just the base topology
[[rigs]]
name = "frontend"
path = "/home/user/frontend"
topology = "topologies/base-agents"
```

This city composes:
- **orchestration** topology at city scope (dir="")
- **base-agents** topology on both rigs
- **code-review** topology only on the backend rig
- Per-rig overrides to customize the reviewer agent
