# Gas City Demo

Two demo modes: the original **Three Stacks** provider demo, and a
**Four-Act Recording** that showcases provider pluggability, topology
comparison, Wasteland federation, and hyperscale K8s.

## Quick start

```bash
# Original provider demo (interactive):
./contrib/demo/run-demo.sh local

# Full 4-act recording:
./contrib/demo/demo-recording.sh all

# Individual acts:
./contrib/demo/demo-recording.sh act1   # Provider pluggability
./contrib/demo/demo-recording.sh act2   # Topology comparison
./contrib/demo/demo-recording.sh act4   # 100-agent hyperscale
```

## Four-Act Recording

Structure: 4 independent act scripts + a top-level orchestrator with
narration pause points between acts. Each act can be recorded separately.

### Act 1: Provider Pluggability

**Script**: `act1-providers.sh`

Runs gastown topology on all 3 provider combos. Same `city.toml`,
different env vars.

| # | Session | Beads | Events |
|---|---------|-------|--------|
| 1 | Local tmux | bd (dolt) | File JSONL |
| 2 | Docker containers | br (beads_rust) | File JSONL |
| 3 | K8s pods (native client-go) | Dolt StatefulSet | K8s ConfigMaps |

Key visual: identical events stream across all 3 combos. Local uses
`.gc/events.jsonl`, K8s uses ConfigMaps — same `gc events --watch`
command, different storage backends.

### Act 2: Topology Comparison

**Script**: `act2-topologies.sh`

Runs gastown (hierarchical) then swarm (flat peer) on local tmux. Shows
different orchestration shapes on the same SDK.

- **Gastown**: mayor -> deacon -> polecat pool (formula dispatch)
- **Swarm**: flat coder pool (self-organizing peers, no formulas)

### Act 3: Wasteland Auto-Claim

**Script**: `act3-wasteland.sh`

**Prerequisite**: Real `wl` binary installed and a Wasteland instance running.

Shows the wasteland-feeder automation chain:
1. Poll fires, `wl sync` + `wl browse` finds open inference items
2. Auto-claim via `wl claim`
3. `gc sling` dispatches to polecat pool
4. Polecat spawns, picks up work, runs inference
5. Bead closed, polecat exits

### Act 4: 100-Agent Hyperscale

**Script**: `act4-hyperscale.sh`

**Prerequisite**: K8s cluster with `gc` namespace.

Spawns 100 worker pods on K8s. The visual: a wall of pods materializing,
events streaming, work completing.

3-pane tmux layout:
```
+------------------------+------------------------+
| Controller logs        | Pod watch              |
|                        | kubectl get pods -w    |
+------------------------+------------------------+
| Progress: 42/100 complete                       |
+-------------------------------------------------+
```

Options:
- `GC_HYPERSCALE_MOCK=true` — uses shell mock instead of Claude (no API cost)
- `ACT4_TIMEOUT=300` — auto-teardown seconds

## Three Stacks Demo (Original)

Demonstrates Gas City's pluggable provider architecture by running the
full Gas Town topology (8 roles, formulas, events) across 3 radically
different infrastructure stacks. **Same city.toml, different env vars.**

| # | Session | Beads | Events | Character |
|---|---------|-------|--------|-----------|
| 1 | Local tmux | bd (dolt) | File JSONL | Laptop dev |
| 2 | Docker containers | br (beads_rust) | File JSONL | Containerized |
| 3 | K8s pods (native client-go) | K8s dolt StatefulSet | K8s ConfigMaps | Production |

### Prerequisites

All combos need:
- `gc` binary in PATH
- `tmux` installed
- `ANTHROPIC_API_KEY` set (for Claude agents)

#### Docker combo

```bash
make docker-base docker-agent
```

#### K8s combo

```bash
kubectl apply -f contrib/k8s/namespace.yaml
kubectl apply -f contrib/k8s/rbac.yaml
kubectl apply -f contrib/k8s/controller-rbac.yaml
kubectl apply -f contrib/k8s/dolt-statefulset.yaml
kubectl apply -f contrib/k8s/dolt-service.yaml

gc build-image examples/gastown --tag gc-agent:latest
make docker-controller
```

### Running

```bash
# With flags:
./contrib/demo/run-demo.sh local
./contrib/demo/run-demo.sh --quick local         # auto-dispatch + auto-teardown
./contrib/demo/run-demo.sh --topology examples/swarm local  # different topology
./contrib/demo/run-demo.sh --quick --topology examples/swarm docker
```

### Dispatching work

```bash
gc sling polecat polecat-work --formula --nudge
```

### Terminal layout

```
+----------------------------+----------------------------+
| 1: Controller              | 2: Events Stream           |
| gc start --foreground      | gc events --watch           |
+----------------------------+----------------------------+
| 3: Mail Traffic            | 4: Agent Peek              |
| watch gc mail list       | peek-cycle.sh              |
+----------------------------+----------------------------+
```

## Helper scripts

| Script | Purpose |
|--------|---------|
| `narrate.sh` | Source for `narrate()`, `pause()`, `step()`, `countdown()` |
| `progress.sh` | Live bead completion counter for a pool label |
| `seed-hyperscale.sh` | Seeds N work beads for the hyperscale pool |
| `peek-cycle.sh` | "Security camera" view cycling through agent sessions |

## Hyperscale example

The `examples/hyperscale/` topology provides a minimal single-pool config
for the 100-agent demo:

```
examples/hyperscale/
  city.toml                                  # K8s session, minimal resources
  topologies/hyperscale/
    topology.toml                            # worker pool, max=100
    prompts/worker.md.tmpl                   # pick up bead, close it, exit
```

Build the prebaked image:
```bash
gc build-image examples/hyperscale --tag gc-hyperscale:latest
```

## Troubleshooting

**Controller exits immediately:** Check `gc doctor` for common issues.

**Docker agents fail to start:** Ensure `gc-agent:latest` image is built.

**K8s pods stuck in Pending:** Check node resources, image pull policy.

**Events pane empty:** Verify the provider env vars are set correctly.

**Act 3 (Wasteland) fails:** Ensure `wl` binary is installed and Wasteland
instance is reachable. Set `WL_BIN=/path/to/wl` if not in PATH.

**Act 4 (Hyperscale) too expensive:** Set `GC_HYPERSCALE_MOCK=true` to
avoid Claude API costs — workers use shell commands instead.
