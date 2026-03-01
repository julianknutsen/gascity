# Three Stacks, One SDK

Demonstrates Gas City's pluggable provider architecture by running the
full Gas Town topology (8 roles, formulas, events) across 3 radically
different infrastructure stacks. **Same city.toml, different env vars.**

| # | Session | Beads | Events | Character |
|---|---------|-------|--------|-----------|
| 1 | Local tmux | bd (dolt) | File JSONL | Laptop dev |
| 2 | Docker containers | br (beads_rust) | File JSONL | Containerized |
| 3 | K8s pods (native client-go) | K8s dolt StatefulSet | K8s ConfigMaps | Production |

## Prerequisites

All combos need:
- `gc` binary in PATH
- `tmux` installed
- `ANTHROPIC_API_KEY` set (for Claude agents)

### Docker combo

```bash
# Build images (from gascity root):
make docker-base docker-agent
```

### K8s combo

```bash
# Deploy infrastructure to your cluster:
kubectl apply -f contrib/k8s/namespace.yaml
kubectl apply -f contrib/k8s/rbac.yaml
kubectl apply -f contrib/k8s/controller-rbac.yaml
kubectl apply -f contrib/k8s/dolt-statefulset.yaml
kubectl apply -f contrib/k8s/dolt-service.yaml

# Build all images (agent + controller):
make docker-base docker-agent docker-controller

# For kind clusters, `make docker-agent` auto-loads into the cluster.
# For remote registries:
docker tag gc-agent:latest your-registry/gc-agent:latest
docker push your-registry/gc-agent:latest
docker tag gc-controller:latest your-registry/gc-controller:latest
docker push your-registry/gc-controller:latest
```

## Running the demo

```bash
# Combo 1 — Local (tmux sessions, bd beads)
./contrib/demo/run-demo.sh local

# Combo 2 — Docker (containers, br beads)
./contrib/demo/run-demo.sh docker

# Combo 3 — Kubernetes (pods, dolt StatefulSet)
./contrib/demo/run-demo.sh k8s
```

Each invocation:
1. Cleans up any previous demo city
2. Runs `gc init --from examples/gastown`
3. Creates a demo rig (temp git repo if none provided)
4. Sets up a 4-pane tmux layout
5. Pauses for you to position your dashboard
6. Starts the controller in foreground mode

### Dispatching work

Once the controller is running and agents are alive:

```bash
# Sling a formula to the polecat pool:
gc sling polecat polecat-work --formula --nudge

# Or create individual beads and a convoy:
gc bead create "Review README.md" --type task --label pool:polecat
gc bead create "Fix typo in main.go" --type task --label pool:polecat
gc convoy create "demo-batch" <bead-id-1> <bead-id-2>
```

## Terminal layout

Local / Docker:
```
┌──────────────────────────┬──────────────────────────┐
│ 1: Controller            │ 2: Events Stream         │
│ gc start --foreground    │ gc events --watch         │
├──────────────────────────┼──────────────────────────┤
│ 3: Convoy Status         │ 4: Agent Peek            │
│ watch gc convoy list     │ peek-cycle.sh            │
└──────────────────────────┴──────────────────────────┘
```

K8s (controller runs in-cluster):
```
┌──────────────────────────┬──────────────────────────┐
│ 1: Controller (pod)      │ 2: Events Stream         │
│ deploy + logs --follow   │ gc events --watch         │
├──────────────────────────┼──────────────────────────┤
│ 3: Convoy Status         │ 4: Agent Peek            │
│ watch gc convoy list     │ peek-cycle.sh            │
└──────────────────────────┴──────────────────────────┘
```

## Recommended dashboards (second monitor)

| Combo | Dashboard | Notes |
|-------|-----------|-------|
| Local | `tmux list-sessions` | Lightweight — just shows active sessions |
| Docker | **lazydocker** (TUI) or **Portainer** (web, :9000) | Containers, logs, resource usage |
| K8s | **Lens** (desktop app) | Pods appearing, events tab, resource graphs |

## In-cluster controller (production K8s)

For production, run the controller inside the cluster instead of locally:

```bash
# Build all images:
make docker-base docker-agent docker-controller

# Apply controller RBAC:
kubectl apply -f contrib/k8s/controller-rbac.yaml

# Deploy (copies city dir into controller pod):
contrib/session-scripts/gc-controller-k8s deploy examples/gastown/

# Monitor:
contrib/session-scripts/gc-controller-k8s logs --follow
contrib/session-scripts/gc-controller-k8s status

# Teardown:
contrib/session-scripts/gc-controller-k8s stop
```

## Providing a real repo

```bash
./contrib/demo/run-demo.sh local /path/to/your/project
```

The repo is registered as a rig, and rig-scoped agents (witness, refinery,
polecat pool) are stamped for it.

## Troubleshooting

**Controller exits immediately:** Check `gc doctor` for common issues
(missing tmux, invalid config, port conflicts).

**Docker agents fail to start:** Ensure `gc-agent:latest` image is built
and contains tmux. Test with `docker run --rm gc-agent:latest tmux -V`.

**K8s pods stuck in Pending:** Check node resources, image pull policy,
and that the image exists in your registry. For kind clusters, ensure
`make docker-agent` loaded the image into the cluster.

**Events pane empty:** Events are written to `.gc/events.jsonl` (local),
K8s ConfigMaps (K8s combo), or not collected (Docker). Verify the provider
is set correctly.

**Agents not picking up work:** GUPP requires agents to poll their hook.
Check that prompts tell agents to run `gc hook` / `gc prime`.

**K8s beads failures:** The K8s combo requires the dolt StatefulSet to be
running. Check: `kubectl get pods -n gc -l app=dolt`.
