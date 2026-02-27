# Exec Session Provider — Script Protocol

Gas City's exec session provider delegates each `session.Provider` operation
to a user-supplied script. This allows any terminal multiplexer or process
manager to be used as a session backend without writing Go code.

## Usage

Set the `GC_SESSION` environment variable to `exec:<script>`:

```bash
# Absolute path
export GC_SESSION=exec:/path/to/gc-session-screen

# PATH lookup
export GC_SESSION=exec:gc-session-screen
```

## Calling Convention

The script receives the operation name as its first argument:

```
<script> <operation> <session-name> [args...]
```

No shell invocation — the script is exec'd directly.

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success |
| 1 | Failure (stderr contains error message) |
| 2 | Unknown operation (treated as success — forward compatible) |

Exit code 2 is the forward-compatibility mechanism. When Gas City adds new
operations in the future, old scripts return exit 2 and the provider treats
it as a no-op success. Scripts only need to implement the operations they
care about.

## Operations

| Operation | Invocation | Stdin | Stdout |
|-----------|-----------|-------|--------|
| `start` | `script start <name>` | JSON config | — |
| `stop` | `script stop <name>` | — | — |
| `interrupt` | `script interrupt <name>` | — | — |
| `is-running` | `script is-running <name>` | — | `true` or `false` |
| `attach` | `script attach <name>` | tty passthrough | tty passthrough |
| `process-alive` | `script process-alive <name>` | process names (1/line) | `true` or `false` |
| `nudge` | `script nudge <name>` | message text | — |
| `set-meta` | `script set-meta <name> <key>` | value on stdin | — |
| `get-meta` | `script get-meta <name> <key>` | — | value (empty = not set) |
| `remove-meta` | `script remove-meta <name> <key>` | — | — |
| `peek` | `script peek <name> <lines>` | — | captured text |
| `list-running` | `script list-running <prefix>` | — | one name per line |
| `get-last-activity` | `script get-last-activity <name>` | — | RFC3339 or empty |

### Start Config (JSON on stdin)

The `start` operation receives a JSON object on stdin:

```json
{
  "work_dir": "/path/to/working/directory",
  "command": "claude --dangerously-skip-permissions",
  "env": {"GC_AGENT": "mayor", "GC_CITY": "/home/user/bright-lights"},
  "process_names": ["claude", "node"],
  "nudge": "initial prompt text"
}
```

All fields are optional (omitted when empty).

### Conventions

- **stdin for values**: `set-meta`, `nudge`, and `start` pass data on stdin
  to avoid shell quoting and argument length limits.
- **stdout for results**: `is-running`, `process-alive` return `true`/`false`.
  `get-meta` returns the value or empty for unset. `list-running` returns one
  name per line.
- **Idempotent stop**: `stop` must succeed (exit 0) even if the session
  doesn't exist.
- **Best-effort interrupt/nudge**: Return 0 even if the session doesn't exist.
- **Empty = unsupported**: `get-last-activity` returning empty stdout means
  the backend doesn't support activity tracking (zero time in Go).

## Writing Your Own Script

1. Start with the `gc-session-screen` script as a template.
2. Implement the operations your backend supports.
3. Return exit 2 for operations you don't support.
4. Test with `GC_SESSION=exec:./your-script gc start <city>`.

### Minimal script (start/stop/is-running only)

```bash
#!/bin/sh
op="$1"
name="$2"
case "$op" in
  start)     cat > /dev/null; my-mux new "$name" ;;
  stop)      my-mux kill "$name" 2>/dev/null; exit 0 ;;
  is-running) my-mux list | grep -q "^${name}$" && echo true || echo false ;;
  *)         exit 2 ;;
esac
```

## Environment Variables

Scripts can use `GC_EXEC_STATE_DIR` (if set) as a directory for sidecar
state files (metadata, wrappers). If not set, scripts should use a
reasonable default under `$TMPDIR` or `/tmp`.

## Shipped Scripts

- `gc-session-screen` — GNU screen implementation. Dependencies: `screen`,
  `jq`, `bash`.
