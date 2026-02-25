# Gas City Configuration

Schema for city.toml — the top-level configuration file for a Gas City instance.

> **Auto-generated** — do not edit. Run `go run ./cmd/genschema` to regenerate.

## City

City is the top-level configuration for a Gas City instance.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `workspace` | Workspace | **yes** |  | Workspace holds city-level metadata (name, default provider). |
| `providers` | map[string]ProviderSpec |  |  | Providers defines named provider presets for agent startup. |
| `agents` | []Agent | **yes** |  | Agents lists all configured agents in this city. |
| `rigs` | []Rig |  |  | Rigs lists external projects registered in the city. |
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

## PoolConfig

PoolConfig defines elastic pool parameters for an agent.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `min` | integer |  |  | Min is the minimum number of pool instances. Defaults to 0. |
| `max` | integer |  |  | Max is the maximum number of pool instances. Defaults to 0. |
| `check` | string |  | `echo 1` | Check is a shell command whose output determines desired pool size. Defaults to "echo 1". |
| `drain_timeout` | string |  | `5m` | DrainTimeout is the maximum time to wait for a pool instance to drain. Defaults to "5m". |

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

## Workspace

Workspace holds city-level metadata and optional defaults that apply to all agents unless overridden per-agent.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `name` | string | **yes** |  | Name is the human-readable name for this city. |
| `provider` | string |  |  | Provider is the default provider name used by agents that don't specify one. |
| `start_command` | string |  |  | StartCommand overrides the provider's command for all agents. |

