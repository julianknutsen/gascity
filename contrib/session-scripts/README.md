# Session Scripts

Community-maintained session provider scripts for Gas City's exec session
provider. These are real implementations we ship, but they have external
dependencies and aren't the same support tier as `gc` itself.

See [docs/exec-session-protocol.md](../../docs/exec-session-protocol.md)
for the protocol specification.

## Scripts

### gc-session-screen

GNU screen backend. Creates screen sessions, sends keystrokes for nudge
and interrupt, captures output via `hardcopy`, and stores metadata in
sidecar files.

**Dependencies:** `screen`, `jq`, `bash`

**Usage:**

```bash
export GC_SESSION=exec:/path/to/contrib/session-scripts/gc-session-screen
gc start my-city
```

**Parity with tmux provider:** The script implements the full 13-operation
protocol but does not yet include Gas Town theming (status bar colors,
role emoji, keybindings) or lifecycle features (remain-on-exit, auto-respawn,
zombie detection). See comments in the script header for the full gap list.
