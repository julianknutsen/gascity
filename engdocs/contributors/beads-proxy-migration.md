# Beads storage migration runbook

## Approved migration contract

Fresh managed-local scopes use Beads `proxied-server` by default. Existing
direct (`server`) scopes are never converted automatically. The four migration
flows below apply only to an existing **bd-owned**, stopped `server`,
`shared-server`, or `proxied-server` workspace. Stop all writers before running
one, and use `--dry-run` first.

`bd dolt stop` stops a bd-owned Dolt process; it is not a `gc stop` substitute.
For a legacy GC-owned workspace, complete the explicit ownership handoff in
`ga-p9iuv.30.1` (currently blocked) before using a bd migration command.

The journaled contract described below — the `.beads/dolt-mode-migration.json`
phases and the fail-closed checks built on them — ships with the beads
`release/1.3.0` line, which is not yet merged to beads `main`. Builds without
it, including the beads module this repo currently pins in `go.mod`
(`v1.1.1-0.20260805093327`) and the installed `8de373f05`, perform the same
mode flip as a single unjournaled `metadata.json` plus sidecar rewrite (plus
the shared-YAML write for shared roots), protected by an exclusive migrate lock
(a second concurrent `bd migrate` is refused) and by idempotent re-invocation
(re-running a finished migration reports the mode is already set and exits
successfully). Confirm your bd build's version floor before relying on
journal-based fault recovery. bd marks all four `bd migrate` subcommands
`[EXPERIMENTAL]` on both builds.

### Server and proxied-server

```sh
bd dolt stop
bd migrate from-server-to-proxied-server --dry-run
bd migrate from-server-to-proxied-server
bd dolt stop
```

Stop the Dolt process before the dry-run as well; migration validates live
ownership and refuses while a server or proxy is running. The forward command
advances through `prepared`, `target_configured`, `old_controls_retired`,
`verified`, and `committed` in `.beads/dolt-mode-migration.json`. It sets
`metadata.json:dolt_mode` to `proxied-server`, writes
`.beads/proxied_server_client_info.json`, and for shared roots writes
`.beads/config.yaml:dolt.shared-server: false`. Reverse commands restore
`dolt_mode: server`, re-enable shared YAML when selected, and remove the
sidecar. Advancing to `committed` removes the journal. Retry the same command
after a fault; never delete the journal or start writers until verification
succeeds.

To roll back a completed server-to-proxied migration, stop the bd-owned process
and run the matching reverse command:

```sh
bd dolt stop
bd migrate from-proxied-server-to-server --dry-run
bd migrate from-proxied-server-to-server
```

For a shared-server root, use the corresponding pair:
`from-shared-server-to-proxied-server` and
`from-proxied-server-to-shared-server`. The direct commands are the escape
hatch for operators who need a managed SQL server. Embedded mode has no
in-place flip; re-provision it explicitly.

### Shared-server and proxied-server

```sh
bd dolt stop
bd migrate from-shared-server-to-proxied-server --dry-run
bd migrate from-shared-server-to-proxied-server
bd dolt stop
```

On a shared root, `bd dolt stop` is not project-local. The first stop above
stops the shared Dolt server for every project sharing it — bd attaches that
same warning to `bd migrate from-shared-server-to-proxied-server` itself — and
the trailing stop targets a proxy keyed by that same shared root. Coordinate
with the other projects on the root before either stop.

To roll back a completed shared-server-to-proxied migration, stop the bd-owned
process and run the matching reverse command:

```sh
bd dolt stop
bd migrate from-proxied-server-to-shared-server --dry-run
bd migrate from-proxied-server-to-shared-server
```

This `bd dolt stop` has the same blast radius: it targets the proxy rooted at
the shared Dolt directory, so it affects every project proxied at that root.

Managed-local mode owns the proxy and child Dolt lifecycle. External TCP or
Unix endpoints are owner-managed; in-place migration refuses them. A migration
journal records each checkpoint, retries repair incomplete work, and a second
successful invocation is idempotent. The journal is absent before a migration
starts and again after one commits; migration creates it before making its
first change. While a migration is in flight, a malformed or unreadable
journal, or sidecar/metadata/topology state that contradicts the journal,
fails closed without mutating the workspace. Before a forward migration
begins, a stray proxied sidecar with no journal is refused rather than
overwritten; for the reverse commands that sidecar is the required starting
state.

The sidecar identifies the proxied root; `metadata.json` stores the mode and
workspace YAML stores shared-server topology. Proxy/server controls and logs
remain inside their owning roots. Verify a pre-existing sentinel bead and its
dependency (`bd show <id> --json`, then `bd dep list <id> --json` and check the
expected blocker) after migration. External TCP/Unix endpoints are in-place
migration refusals; they remain operator-managed. Embedded scopes require
explicit re-provisioning. Migration does not promise Git remotes or backups;
RC2 readiness is a separate gate.

Storage selection is explicit. A normal city start or `gc init` must not move
an existing direct or server-backed Beads workspace, and must preserve its
metadata, sentinel files, ownership, and migration checkpoint.

## Fresh city

Create a new city with the normal command:

```sh
gc init ~/my-city
```

`gc init` does not convert an existing Beads store. The release/RC version is
the binary selected by the operator (verify with `gc version`); do not run a
newer binary against a workspace until its version floor is approved.

## Safety checks

- Startup with unchanged configuration is a no-op.
- Configuration changes require an explicit migration intent and durable
  checkpoint; they are never inferred from provider availability.
- Missing metadata after initialization, malformed or unreadable metadata, and
  a `.beads/dolt-mode-migration.json` journal that is unreadable or that
  contradicts the on-disk sidecar, metadata, or topology fail closed rather
  than creating a new store. A journal left on disk by a faulted run is not a
  fail-closed state at any phase: it is the resume record, and rerunning the
  same command repairs the migration from that checkpoint. A missing
  `.beads/dolt-mode-migration.json` is expected both before a migration starts
  and after one commits: migration creates it as its initial checkpoint and
  removes it at the `committed` phase.
- Managed-local topology owns the child server lifecycle. External TCP and
  Unix topologies do not: they reconnect to the configured endpoint and never
  adopt or restart the external server.

The migration intent and guard invariants are exercised by
`TestDeriveStartupIntentIsANoOpWhenEveryIdentityMatches`,
`TestDeriveStartupIntentMigratesOnConfigurationChangeAlone`, and
`TestAcquireMigrationGuardRejectsNoncanonicalOrSymlinkedCityDirectory`.
