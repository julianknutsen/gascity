# Beads Scripts

Community-maintained bead store provider scripts for Gas City's exec beads
provider. These are reference implementations that wrap external bead stores.

See [docs/exec-beads-protocol.md](../../docs/exec-beads-protocol.md)
for the protocol specification.

## Scripts

### gc-beads-br

beads_rust (`br`) backend. Wraps the `br` CLI to provide full bead store
functionality with SQLite + JSONL backing.

**Dependencies:** `br` (beads_rust), `jq`, `bash`

**Usage:**

```bash
export GC_BEADS=exec:/path/to/contrib/beads-scripts/gc-beads-br
gc start my-city
```

Or in `city.toml`:

```toml
[beads]
provider = "exec:/path/to/contrib/beads-scripts/gc-beads-br"
```

**Label conventions:**

| Convention | Purpose |
|-----------|---------|
| `parent:<id>` | Tracks parent-child relationships (Children operation) |
| `meta:<key>=<value>` | Stores metadata (SetMetadata operation) |
| `needs:<step-id>` | Tracks step dependencies within molecules |

**Lifecycle operations:**

| Operation | Behavior |
|-----------|----------|
| `ensure-ready` | Exit 2 (br uses embedded SQLite, always ready) |
| `shutdown` | Exit 2 (no server process to stop) |

**Other optional operations:**

- `mol-cook` — composed in Go by `exec.Store` using Create calls; script
  returns exit 2 (not applicable)
- `init` — not needed; run `br init` separately if required
- `config-set` — not applicable
- `purge` — not supported; use `br` CLI directly for cleanup
