# Mail Roadmap

Tracks the full Gastown mail feature set and when we expect to need each
piece. Nothing here is speculative — every feature exists in Gastown
production. The question is ordering.

## Phase 1 — Tutorial 05: Basic Mail

Minimum viable mail. Human ↔ agent conversation, agents check inbox in loop.

| Feature | Notes |
|---------|-------|
| Mail = bead with type "message" | Core storage model |
| `gc mail send <to> "<body>"` | Create message bead |
| `gc mail inbox [agent]` | List unread messages |
| `gc mail read <id>` | Show message, mark as read |
| `from` / `to` fields | Via assignee + labels |
| Unread tracking | open = unread, closed = read |
| Implicit "human" sender/recipient | CLI defaults |
| Validate recipient exists | Error on nonexistent agent |
| `prompts/loop-mail.md` | Loop with inbox check step |

## Phase 2 — Agent-to-Agent Coordination

When agents need to talk to each other to resolve issues.

| Feature | Why deferred |
|---------|-------------|
| Agent → agent mail | Needs multiple agents with distinct roles |
| `--subject` flag | Short messages don't need subject vs body split |
| Reply-to / threading | Needs conversation patterns to justify |
| `gc mail reply <id> "<body>"` | Sugar for send with thread linkage |

## Phase 3 — Message Lifecycle

When inbox bloat becomes a problem.

| Feature | Why deferred |
|---------|-------------|
| Wisps (ephemeral, default) | Needs patrol/cleanup to auto-purge |
| `--permanent` flag | Only meaningful once wisps are default |
| Pinned messages | Needs context cycling to justify |
| `gc mail archive` | Needs enough messages to warrant cleanup |
| `gc mail delete` | Needs archive first |
| `gc mail mark-read` / `mark-unread` | Read without closing |
| Stale message archival | Needs session restart awareness |

## Phase 4 — Priority & Urgency

When health patrol exists and can act on priority.

| Feature | Why deferred |
|---------|-------------|
| Priority tiers (urgent/high/normal/low) | Needs health patrol to interrupt |
| `--urgent` flag | Needs priority to exist |
| Nudge on send | Needs nudge infrastructure |
| Idle-aware notification | Needs tmux idle detection |
| Nudge enqueue for busy agents | Needs nudge queue |
| Priority-stratified inbox check | Needs `gc mail check --inject` |

## Phase 5 — Routing & Groups

When multi-project and team topologies exist.

| Feature | Why deferred |
|---------|-------------|
| Queue messages (claiming) | Needs ephemeral worker pools |
| `gc mail claim` / `gc mail release` | Queue consumer commands |
| Announce/channel (broadcast) | Needs subscriber concept |
| @group expansion (@town, @rig) | Needs project scoping |
| CC recipients | Needs multi-recipient patterns |
| List addresses (fan-out) | Needs messaging.json config |

## Phase 6 — Delivery Guarantees

When reliability matters at scale.

| Feature | Why deferred |
|---------|-------------|
| Two-phase delivery (pending → acked) | Needs delivery tracking |
| Idempotent ack with timestamp reuse | Needs two-phase first |
| Bounded concurrent acks | Optimization for scale |
| `--no-notify` / suppress notify | Needs nudge infrastructure |
| DND / muted agents | Needs health config |

## Gastown Features We May Never Need

These exist in Gastown but may not apply to Gas City's model.

| Feature | Reason |
|---------|--------|
| Legacy JSONL storage | Gas City is beads-only |
| Crew-specific inbox paths | No hardcoded roles |
| `gt mail search` | Nice to have, not essential |
| Message type field (task/scavenge/notification/reply) | May not need structured types |
