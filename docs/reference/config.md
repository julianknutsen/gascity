# Gas City Configuration

Schema for city.toml — the top-level configuration file for a Gas City instance.

> **Auto-generated** — do not edit. Run `go run ./cmd/genschema` to regenerate.

## City

City is the top-level configuration for a Gas City instance.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `include` | []string |  |  | Include lists config fragment files to merge into this config. Processed by LoadWithIncludes; not recursive (fragments cannot include). |
| `workspace` | Workspace | **yes** |  | Workspace holds city-level metadata (name, default provider). |
| `providers` | map[string]ProviderSpec |  |  | Providers defines named provider presets for agent startup. |
| `topologies` | map[string]TopologySource |  |  | Topologies defines named remote topology sources fetched via git. |
| `agents` | []Agent | **yes** |  | Agents lists all configured agents in this city. |
| `rigs` | []Rig |  |  | Rigs lists external projects registered in the city. |
| `patches` | Patches |  |  | Patches holds targeted modifications applied after fragment merge. |
| `beads` | BeadsConfig |  |  | Beads configures the bead store backend. |
| `dolt` | DoltConfig |  |  | Dolt configures optional dolt server connection overrides. |
| `formulas` | FormulasConfig |  |  | Formulas configures formula directory settings. |
| `daemon` | DaemonConfig |  |  | Daemon configures controller daemon settings. |

## Agent

Agent defines a configured agent in the city.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | **yes** |  | Name is the unique identifier for this agent. |
| `dir` | string |  |  | Dir is the working directory for the agent session. |
| `suspended` | boolean |  |  | Suspended prevents the reconciler from spawning this agent. Toggle with gc agent suspend/resume. |
| `isolation` | string |  | `none` | Isolation controls filesystem isolation: "none" (default) or "worktree". Enum: `none`, `worktree` |
| `prompt_template` | string |  |  | PromptTemplate is the path to this agent's prompt template file. |
| `nudge` | string |  |  | Nudge is text typed into the agent's tmux session after startup. Used for CLI agents that don't accept command-line prompts. |
| `provider` | string |  |  | Provider names the provider preset to use for this agent. |
| `start_command` | string |  |  | StartCommand overrides the provider's command for this agent. |
| `args` | []string |  |  | Args overrides the provider's default arguments. |
| `prompt_mode` | string |  | `arg` | PromptMode controls how prompts are delivered: "arg", "flag", or "none". Enum: `arg`, `flag`, `none` |
| `prompt_flag` | string |  |  | PromptFlag is the CLI flag used to pass prompts when prompt_mode is "flag". |
| `ready_delay_ms` | integer |  |  | ReadyDelayMs is milliseconds to wait after launch before considering the agent ready. |
| `ready_prompt_prefix` | string |  |  | ReadyPromptPrefix is the string prefix that indicates the agent is ready for input. |
| `process_names` | []string |  |  | ProcessNames lists process names to look for when checking if the agent is running. |
| `emits_permission_warning` | boolean |  |  | EmitsPermissionWarning indicates whether the agent emits permission prompts that should be suppressed. |
| `env` | map[string]string |  |  | Env sets additional environment variables for the agent process. |
| `pool` | PoolConfig |  |  | Pool configures elastic pool behavior. When set, the agent becomes a pool. |
| `work_query` | string |  |  | WorkQuery is the command to find available work for this agent. Used by gc hook and available in prompt templates as {{ .WorkQuery }}. Default for fixed agents: "bd ready --assignee=<qualified-name>" Default for pool agents: "bd ready --label=pool:<qualified-name> --limit=1" |
| `sling_query` | string |  |  | SlingQuery is the command template to route a bead to this agent/pool. Used by gc sling to make a bead visible to the target's work_query. The placeholder {} is replaced with the bead ID at runtime. Default for fixed agents: "bd update {} --assignee=<qualified-name>" Default for pool agents: "bd update {} --label=pool:<qualified-name>" Pool agents must set both sling_query and work_query, or neither. |
| `idle_timeout` | string |  |  | IdleTimeout is the maximum time an agent session can be inactive before the controller kills and restarts it. Empty (default) disables idle checking. Example: "15m", "1h". |
| `install_agent_hooks` | []string |  |  | InstallAgentHooks overrides workspace-level install_agent_hooks for this agent. When set, replaces (not adds to) the workspace default. |
| `hooks_installed` | boolean |  |  | HooksInstalled overrides automatic hook detection. Set to true when hooks are manually installed (e.g., merged into the project's own hook config) and auto-installation via install_agent_hooks is not desired, but the agent should still be treated as hook-enabled for startup behavior (no prime instruction in beacon, no delayed nudge). |
| `session_setup` | []string |  |  | SessionSetup is a list of shell commands run after session creation. Each command is a Go text/template string expanded with session context ({{.Session}}, {{.Agent}}, {{.Rig}}, {{.CityRoot}}, {{.CityName}}, {{.WorkDir}}). Commands run in gc's process (not inside the agent session) via sh -c. |
| `session_setup_script` | string |  |  | SessionSetupScript is a path to a script run after session_setup commands. Relative paths resolve against the city directory. The script receives context via env vars (GC_SESSION plus existing GC_* vars). |

## AgentOverride

AgentOverride modifies a topology-stamped agent for a specific rig.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `agent` | string | **yes** |  | Agent is the name of the topology agent to override (required). |
| `dir` | string |  |  | Dir overrides the stamped dir (default: rig name). |
| `suspended` | boolean |  |  | Suspended sets the agent's suspended state. |
| `pool` | PoolOverride |  |  | Pool overrides pool configuration fields. |
| `env` | map[string]string |  |  | Env adds or overrides environment variables. |
| `env_remove` | []string |  |  | EnvRemove lists env var keys to remove. |
| `isolation` | string |  |  | Isolation overrides the isolation mode. |
| `prompt_template` | string |  |  | PromptTemplate overrides the prompt template path. |
| `provider` | string |  |  | Provider overrides the provider name. |
| `start_command` | string |  |  | StartCommand overrides the start command. |
| `nudge` | string |  |  | Nudge overrides the nudge text. |
| `idle_timeout` | string |  |  | IdleTimeout overrides the idle timeout duration. |
| `install_agent_hooks` | []string |  |  | InstallAgentHooks overrides the agent's install_agent_hooks list. |
| `hooks_installed` | boolean |  |  | HooksInstalled overrides automatic hook detection. |
| `session_setup` | []string |  |  | SessionSetup overrides the agent's session_setup commands. |
| `session_setup_script` | string |  |  | SessionSetupScript overrides the agent's session_setup_script path. |

## AgentPatch

AgentPatch modifies an existing agent identified by (Dir, Name).

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `dir` | string | **yes** |  | Dir is the targeting key (required with Name). |
| `name` | string | **yes** |  | Name is the targeting key (required). |
| `suspended` | boolean |  |  | Suspended overrides the agent's suspended state. |
| `pool` | PoolOverride |  |  | Pool overrides pool configuration fields. |
| `env` | map[string]string |  |  | Env adds or overrides environment variables. |
| `env_remove` | []string |  |  | EnvRemove lists env var keys to remove after merging. |
| `isolation` | string |  |  | Isolation overrides the isolation mode. |
| `prompt_template` | string |  |  | PromptTemplate overrides the prompt template path. |
| `provider` | string |  |  | Provider overrides the provider name. |
| `start_command` | string |  |  | StartCommand overrides the start command. |
| `nudge` | string |  |  | Nudge overrides the nudge text. |
| `idle_timeout` | string |  |  | IdleTimeout overrides the idle timeout duration. |
| `install_agent_hooks` | []string |  |  | InstallAgentHooks overrides the agent's install_agent_hooks list. |
| `hooks_installed` | boolean |  |  | HooksInstalled overrides automatic hook detection. |
| `session_setup` | []string |  |  | SessionSetup overrides the agent's session_setup commands. |
| `session_setup_script` | string |  |  | SessionSetupScript overrides the agent's session_setup_script path. |

## BeadsConfig

BeadsConfig holds bead store settings.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `provider` | string |  | `bd` | Provider selects the bead store backend: "bd" (default) or "file". Enum: `bd`, `file` |

## DaemonConfig

DaemonConfig holds controller daemon settings.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `patrol_interval` | string |  | `30s` | PatrolInterval is the health patrol interval as a Go duration string. Defaults to "30s". |
| `max_restarts` | integer |  | `5` | MaxRestarts is the maximum number of agent restarts within RestartWindow before the agent is quarantined. 0 means unlimited (no crash loop detection). Defaults to 5. |
| `restart_window` | string |  | `1h` | RestartWindow is the sliding time window for counting restarts, as a Go duration string. Defaults to "1h". |
| `shutdown_timeout` | string |  | `5s` | ShutdownTimeout is the time to wait after sending Ctrl-C before force-killing agents during shutdown. Set to "0s" for immediate kill. Defaults to "5s". |

## DoltConfig

DoltConfig holds optional dolt server overrides.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `port` | integer |  | `3307` | Port is the dolt server port. Defaults to 3307. |
| `host` | string |  | `localhost` | Host is the dolt server hostname. Defaults to localhost. |

## FormulasConfig

FormulasConfig holds formula directory settings.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `dir` | string |  | `.gc/formulas` | Dir is the path to the formulas directory. Defaults to ".gc/formulas". |
| `periodic` | []PeriodicFormula |  |  | Periodic lists formulas that the deacon dispatches on a schedule. Each entry names a formula and its gate condition (cooldown, cron, etc.). |

## Patches

Patches holds all patch blocks from composition.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `agents` | []AgentPatch |  |  | Agents targets agents by (dir, name). |
| `rigs` | []RigPatch |  |  | Rigs targets rigs by name. |
| `providers` | []ProviderPatch |  |  | Providers targets providers by name. |

## PeriodicFormula

PeriodicFormula registers a formula for periodic dispatch by the deacon.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `formula` | string | **yes** |  | Formula is the formula name (must exist in the formulas directory). |
| `gate` | string | **yes** |  | Gate is the gate type: "cooldown", "cron", "condition", or "event". Enum: `cooldown`, `cron`, `condition`, `event` |
| `interval` | string |  |  | Interval is the minimum time between runs (for cooldown gates). Go duration string. |
| `schedule` | string |  |  | Schedule is a cron expression (for cron gates). |
| `check` | string |  |  | Check is a shell command that returns exit 0 when the formula should run (for condition gates). |
| `pool` | string |  |  | Pool is the target agent qualified name for dispatching the wisp. The deacon labels the wisp with pool:<value> to match the target agent's EffectiveWorkQuery. Example: "dog" targets pool:dog. |

## PoolConfig

PoolConfig defines elastic pool parameters for an agent.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `min` | integer |  |  | Min is the minimum number of pool instances. Defaults to 0. |
| `max` | integer |  |  | Max is the maximum number of pool instances. Defaults to 0. |
| `check` | string |  | `echo 1` | Check is a shell command whose output determines desired pool size. Defaults to "echo 1". |
| `drain_timeout` | string |  | `5m` | DrainTimeout is the maximum time to wait for a pool instance to drain. Defaults to "5m". |

## PoolOverride

PoolOverride modifies pool configuration fields.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `min` | integer |  |  | Min overrides pool minimum instances. |
| `max` | integer |  |  | Max overrides pool maximum instances. |
| `check` | string |  |  | Check overrides the pool check command. |
| `drain_timeout` | string |  |  | DrainTimeout overrides the drain timeout. |

## ProviderPatch

ProviderPatch modifies an existing provider identified by Name.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | **yes** |  | Name is the targeting key (required). |
| `command` | string |  |  | Command overrides the provider command. |
| `args` | []string |  |  | Args overrides the provider args. |
| `prompt_mode` | string |  |  | PromptMode overrides prompt delivery mode. |
| `prompt_flag` | string |  |  | PromptFlag overrides the prompt flag. |
| `ready_delay_ms` | integer |  |  | ReadyDelayMs overrides the ready delay. |
| `env` | map[string]string |  |  | Env adds or overrides environment variables. |
| `env_remove` | []string |  |  | EnvRemove lists env var keys to remove. |
| `_replace` | boolean |  |  | Replace replaces the entire provider block instead of deep-merging. |

## ProviderSpec

ProviderSpec defines a named provider's startup parameters.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `display_name` | string |  |  | DisplayName is the human-readable name shown in UI and logs. |
| `command` | string |  |  | Command is the executable to run for this provider. |
| `args` | []string |  |  | Args are default command-line arguments passed to the provider. |
| `prompt_mode` | string |  | `arg` | PromptMode controls how prompts are delivered: "arg", "flag", or "none". Enum: `arg`, `flag`, `none` |
| `prompt_flag` | string |  |  | PromptFlag is the CLI flag used when prompt_mode is "flag" (e.g. "--prompt"). |
| `ready_delay_ms` | integer |  |  | ReadyDelayMs is milliseconds to wait after launch before the provider is considered ready. |
| `ready_prompt_prefix` | string |  |  | ReadyPromptPrefix is the string prefix that indicates the provider is ready for input. |
| `process_names` | []string |  |  | ProcessNames lists process names to look for when checking if the provider is running. |
| `emits_permission_warning` | boolean |  |  | EmitsPermissionWarning indicates whether the provider emits permission prompts. |
| `env` | map[string]string |  |  | Env sets additional environment variables for the provider process. |

## Rig

Rig defines an external project registered in the city.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | **yes** |  | Name is the unique identifier for this rig. |
| `path` | string | **yes** |  | Path is the absolute filesystem path to the rig's repository. |
| `prefix` | string |  |  | Prefix overrides the auto-derived bead ID prefix for this rig. |
| `suspended` | boolean |  |  | Suspended prevents the reconciler from spawning agents in this rig. Toggle with gc rig suspend/resume. |
| `topology` | string |  |  | Topology is the path to a topology directory to stamp agents from. Relative paths resolve against the declaring file's directory. |
| `formulas_dir` | string |  |  | FormulasDir is a rig-local formula directory (Layer 4). Overrides topology formulas for this rig by filename. |
| `overrides` | []AgentOverride |  |  | Overrides are per-agent patches applied after topology expansion. |

## RigPatch

RigPatch modifies an existing rig identified by Name.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | **yes** |  | Name is the targeting key (required). |
| `path` | string |  |  | Path overrides the rig's filesystem path. |
| `prefix` | string |  |  | Prefix overrides the bead ID prefix. |
| `suspended` | boolean |  |  | Suspended overrides the rig's suspended state. |

## TopologySource

TopologySource defines a remote topology repository.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `source` | string | **yes** |  | Source is the git repository URL. |
| `ref` | string |  |  | Ref is the git ref to checkout (branch, tag, or commit). Defaults to HEAD. |
| `path` | string |  |  | Path is a subdirectory within the repo containing the topology files. |

## Workspace

Workspace holds city-level metadata and optional defaults that apply to all agents unless overridden per-agent.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | **yes** |  | Name is the human-readable name for this city. |
| `provider` | string |  |  | Provider is the default provider name used by agents that don't specify one. |
| `start_command` | string |  |  | StartCommand overrides the provider's command for all agents. |
| `suspended` | boolean |  |  | Suspended controls whether the city is suspended. When true, all agents are effectively suspended: the reconciler won't spawn them, and gc hook/prime return empty. Inherits downward — individual agent/rig suspended fields are checked independently. |
| `session_template` | string |  |  | SessionTemplate is a Go text/template string for session naming. Available variables: .City, .Agent (sanitized), .Dir, .Name. Default (empty): "gc-{{.City}}-{{.Agent}}". |
| `install_agent_hooks` | []string |  |  | InstallAgentHooks lists provider names whose hooks should be installed into agent working directories. Agent-level overrides workspace-level (replace, not additive). Supported: "claude", "gemini", "opencode", "copilot". |
| `topology` | string |  |  | Topology is the path to a city-level topology directory. Stamps agents with dir="" (city-scoped). Resolved like rig topologies. |

