# Capability Progression

Internal reference — what each tutorial unlocks and what it requires.

| Tut | Problem | Config added | Prompts | Infrastructure used |
|-----|---------|--------------|---------|---------------------|
| 01 | Context loss kills progress | `[workspace]`, `[[agents]]` | one-shot | beads, session, reconciler |
| 02 | Named agents for different jobs | Multiple `[[agents]]` | mayor, worker | agent hook (assign to named agent) |
| 03 | Hand-feeding tasks one at a time | — | loop | hook (atomic self-claim via ready queue) |
| 04 | One agent too slow | More `[[agents]]` entries | — | — (just config + existing hooks) |
| 06 | Restart from scratch on multi-step work | `[formulas]` | — | formula parser, molecules (bead DAG) |
| 07 | Reusable formulas with specific context | `gc mol create --on` | — | attached molecules, Store.Update |
| 08 | Need more hands when work piles up | `[[pools]]`, `scale_check` | polecat | pool manager, scale_check shell eval |
| 05b | Agents die silently | `[daemon]`, `[agents.health]` | — | health patrol, restart |
| 05c | Manual maintenance chores | `[plugins]` | — | plugin gates, event bus |
| 05d | Multi-repo orchestration | `[projects.*]`, `scope` | — | project scoping, agent replication |
