# Messaging

> Last verified against code: 2026-03-01

## Summary

Messaging is a Layer 2-4 derived mechanism that provides inter-agent
communication without introducing new primitives. Mail is composed
from the Bead Store (`TaskStore.Create(bead{type:"message"})`), and
nudge is composed from the Agent Protocol
(`session.Provider.Nudge()`). No new infrastructure is needed — messaging
is a thin composition layer proving the primitives are sufficient.

## Key Concepts

- **Mail**: A message bead — a bead with `Type="message"`, `From` set
  to the sender, `Assignee` set to the recipient, and `Title`
  containing the body text. Open mail beads are unread; closed beads
  are read or archived.

- **Inbox**: The set of open message beads assigned to a recipient.
  Queried by filtering `store.List()` for `Type="message"`,
  `Status="open"`, `Assignee=recipient`.

- **Archive**: Closing a message bead without reading it. Idempotent
  via `ErrAlreadyArchived`.

- **Nudge**: Text sent directly to an agent's session to wake or
  redirect it. Delivered via `session.Provider.Nudge()`. Configured
  per-agent in `Agent.Nudge`. Not persisted — fire-and-forget.

- **Provider**: The pluggable mail backend interface. Two
  implementations: beadmail (default, backed by `beads.Store`) and
  exec (user-supplied script).

## Architecture

```
                    ┌─────────────┐
                    │ gc mail CLI │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │ mail.Provider│
                    └──────┬──────┘
                    ┌──────┴──────┐
              ┌─────▼─────┐ ┌────▼────┐
              │  beadmail  │ │  exec   │
              │ (default)  │ │ (script)│
              └─────┬──────┘ └─────────┘
                    │
              ┌─────▼──────┐
              │ beads.Store │
              └────────────┘
```

### Data Flow

**Sending a message (beadmail path):**

1. `gc mail send agent-1 -m "hello"` invokes `Provider.Send("sender", "agent-1", "hello")`
2. beadmail calls `store.Create(Bead{Title:"hello", Type:"message", Assignee:"agent-1", From:"sender"})`
3. Store assigns ID, sets Status="open", returns the bead
4. beadmail converts to `mail.Message` and returns

**Checking inbox:**

1. `gc mail inbox` invokes `Provider.Inbox("agent-1")`
2. beadmail calls `store.List()` and filters for `Type="message"`, `Status="open"`, `Assignee="agent-1"`
3. Returns matching messages

**Reading a message:**

1. `Provider.Read(id)` retrieves the bead via `store.Get(id)`
2. If not already closed, calls `store.Close(id)` (marks as read)
3. Returns the message

### Key Types

- **`mail.Provider`** — interface with Send, Inbox, Read, Archive,
  Check methods. Defined in `internal/mail/mail.go`.
- **`mail.Message`** — ID, From, To, Body, CreatedAt. The transport
  struct returned by all Provider methods.
- **`beadmail.Provider`** — default implementation backed by
  `beads.Store`. Defined in `internal/mail/beadmail/beadmail.go`.
- **`mail.ErrAlreadyArchived`** — sentinel error for idempotent
  archive calls.

## Invariants

1. **Messages are beads.** Every message has a corresponding bead with
   `Type="message"`. No separate message storage exists.
2. **Inbox returns only open messages.** Closed (read/archived) beads
   are excluded from inbox results.
3. **Archive is idempotent.** Archiving an already-archived message
   returns `ErrAlreadyArchived`, not a generic error.
4. **Check does not mutate state.** Unlike Read, Check returns messages
   without closing them. Used by hooks for non-destructive inspection.
5. **Nudge is fire-and-forget.** There is no delivery guarantee,
   persistence, or retry for nudges. If the session is not running,
   the nudge is lost.

## Interactions

| Depends on | How |
|---|---|
| `internal/beads` | beadmail stores messages as beads |
| `internal/session` | Nudge delivered via Provider.Nudge() |

| Depended on by | How |
|---|---|
| `cmd/gc/cmd_mail.go` | CLI commands: send, inbox, read, archive |
| `cmd/gc/cmd_hook.go` | Hook checks for unread mail via Check() |
| Agent prompts | Templates reference `gc mail` commands |

## Code Map

- `internal/mail/mail.go` — Provider interface, Message struct, ErrAlreadyArchived
- `internal/mail/fake.go` — test double
- `internal/mail/fake_conformance_test.go` — conformance tests for fakes
- `internal/mail/beadmail/beadmail.go` — bead-backed implementation
- `internal/mail/exec/` — script-based mail provider
- `internal/mail/mailtest/` — test helpers
- `cmd/gc/cmd_mail.go` — CLI commands

## Configuration

```toml
[mail]
provider = "beadmail"   # default; or "exec" for script-based
```

The exec provider runs a user-supplied script for each mail operation,
allowing integration with external messaging systems.

## Testing

- `internal/mail/fake_conformance_test.go` — verifies the fake
  satisfies the Provider contract
- `internal/mail/beadmail/` — unit tests for bead-backed provider
- `test/integration/mail_test.go` — integration tests with real beads

## Known Limitations

- **beadmail.Inbox scans all beads.** Uses `store.List()` with
  client-side filtering. No server-side query for type + status +
  assignee. Acceptable for current scale.
- **No delivery confirmation.** Neither mail nor nudge provides
  read receipts or delivery guarantees.
- **No threading.** Messages are flat — no reply chains or
  conversation grouping.

## See Also

- [Bead Store](beads.md) — messages are stored as beads; understanding
  bead lifecycle explains mail lifecycle
- [Agent Protocol](agent-protocol.md) — Nudge() delivery mechanism
- [Glossary](glossary.md) — authoritative definitions of mail, nudge,
  and related terms
