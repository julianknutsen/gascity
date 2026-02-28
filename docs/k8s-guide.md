# Kubernetes Deployment Guide

Run Gas City agents as Kubernetes pods with Dolt as a K8s StatefulSet.
The `gc` controller runs on your laptop and manages agent pods via
`kubectl`. No new Go code required — agent sessions use the exec session
provider with the `gc-session-k8s` script.

Each agent pod runs tmux inside the container, giving identical session
semantics to the local tmux provider — real terminal scrollback, keystroke
nudge, interactive attach.

## Architecture

```
┌─── Your Laptop ───────────────┐     ┌─── K8s Cluster ──────────────┐
│                                │     │                               │
│  gc controller                 │     │  ┌─────────────────────┐     │
│    │                           │     │  │ Dolt StatefulSet     │     │
│    ├─ kubectl apply (start)    │────>│  │  port 3307           │     │
│    ├─ kubectl delete (stop)    │     │  │  PVC: 10Gi           │     │
│    ├─ kubectl exec tmux (ops)  │     │  └─────────────────────┘     │
│    │                           │     │            │                  │
│  gc-session-k8s script         │     │  ┌─────────┴───────────┐     │
│    translates session ops      │     │  │    dolt Service      │     │
│    to kubectl + tmux commands  │     │  │    ClusterIP:3307    │     │
│                                │     │  └─────────┬───────────┘     │
└────────────────────────────────┘     │            │                  │
                                       │  ┌─────────┴───────────┐     │
                                       │  │ Agent Pod (mayor)    │     │
                                       │  │   └─ tmux → claude   │     │
                                       │  │ Agent Pod (coder-1)  │     │
                                       │  │   └─ tmux → claude   │     │
                                       │  └─────────────────────┘     │
                                       └───────────────────────────────┘
```

## Prerequisites

- `kubectl` configured with cluster access
- A container registry accessible from the cluster
- `jq` installed locally

## Setup

### 1. Create the namespace and Dolt server

```bash
kubectl apply -f contrib/k8s/namespace.yaml
kubectl apply -f contrib/k8s/dolt-statefulset.yaml
kubectl apply -f contrib/k8s/dolt-service.yaml
kubectl apply -f contrib/k8s/rbac.yaml

# Wait for Dolt to be ready.
kubectl -n gc wait --for=condition=Ready pod -l app=dolt --timeout=120s
```

### 2. Build and push the agent image

```bash
# Build the gc binary first.
go build -o gc ./cmd/gc

# Build the agent image.
docker build -f contrib/k8s/Dockerfile.agent -t myregistry/gc-agent:latest .

# Push to your registry.
docker push myregistry/gc-agent:latest
```

### 3. Configure your city

Copy `contrib/k8s/example-city.toml` to your city directory and customize:

```bash
mkdir -p ~/my-city
cp contrib/k8s/example-city.toml ~/my-city/city.toml
```

### 4. Set environment and start

```bash
export GC_SESSION=exec:$(pwd)/contrib/session-scripts/gc-session-k8s
export GC_K8S_IMAGE=myregistry/gc-agent:latest
export GC_DOLT_HOST=dolt.gc.svc.cluster.local
export GC_DOLT_PORT=3307

gc init ~/my-city
gc start ~/my-city
```

## Configuration

The `gc-session-k8s` script reads these environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `GC_K8S_NAMESPACE` | `gc` | K8s namespace for agent pods |
| `GC_K8S_IMAGE` | (required) | Container image for agents |
| `GC_K8S_CONTEXT` | current | kubectl context to use |
| `GC_K8S_DOLT_HOST` | `dolt.gc.svc.cluster.local` | Dolt service DNS |
| `GC_K8S_DOLT_PORT` | `3307` | Dolt service port |
| `GC_K8S_CPU_REQUEST` | `500m` | Pod CPU request |
| `GC_K8S_MEM_REQUEST` | `1Gi` | Pod memory request |
| `GC_K8S_CPU_LIMIT` | `2` | Pod CPU limit |
| `GC_K8S_MEM_LIMIT` | `4Gi` | Pod memory limit |

## How it works

Each agent pod starts tmux with the agent command inside it. All session
operations go through `kubectl exec -- tmux ...`, giving the same
semantics as the local tmux provider.

| Session Op | Implementation |
|------------|----------------|
| `start` | `kubectl apply` pod, entrypoint runs `tmux new-session -d -s main '<cmd>'` |
| `stop` | `kubectl delete pod -l gc-session=<name>` |
| `is-running` | `kubectl get pod` → check phase=Running |
| `interrupt` | `kubectl exec -- tmux send-keys C-c` |
| `attach` | `kubectl exec -it -- tmux attach -t main` |
| `nudge` | `kubectl exec -- tmux send-keys '<text>' Enter` |
| `peek` | `kubectl exec -- tmux capture-pane -p -S -<lines>` |
| `clear-scrollback` | `kubectl exec -- tmux clear-history` |
| `get-last-activity` | `kubectl exec -- tmux display-message '#{session_activity}'` → RFC3339 |
| `set-meta` | `kubectl annotate pod` |
| `get-meta` | `kubectl get pod -o jsonpath` |
| `list-running` | `kubectl get pods -l app=gc-agent` |

## Events Provider

Store events as Kubernetes ConfigMaps instead of a local JSONL file.
Each event becomes a labeled ConfigMap; sequence numbers use a dedicated
counter ConfigMap with compare-and-swap updates.

### Setup

```bash
export GC_EVENTS=exec:$(pwd)/contrib/events-scripts/gc-events-k8s
```

This can be combined with the K8s session provider — both share the same
namespace and context variables.

### Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `GC_K8S_NAMESPACE` | `gc` | K8s namespace for event ConfigMaps |
| `GC_K8S_CONTEXT` | current | kubectl context to use |

### How it works

| Events Op | kubectl Command |
|-----------|-----------------|
| `ensure-running` | Check/create `gc-events-seq` counter ConfigMap |
| `record` | CAS increment seq counter + create `gc-evt-NNNN` ConfigMap |
| `list` | `kubectl get configmaps -l gc/component=event` + jq filter |
| `latest-seq` | `kubectl get configmap gc-events-seq -o jsonpath` |
| `watch` | `kubectl get configmaps --watch` + jq stream filter |

### ConfigMap layout

Events use two types of ConfigMaps:

- **`gc-events-seq`** — tracks the latest sequence number
- **`gc-evt-0000000042`** — one per event, labeled with `gc/type` and
  `gc/actor` for efficient label-selector queries

### Troubleshooting

```bash
# Check event ConfigMaps
kubectl -n gc get configmaps -l gc/component=event

# View a specific event
kubectl -n gc get configmap gc-evt-0000000001 -o jsonpath='{.data.event}' | jq .

# Check current sequence number
kubectl -n gc get configmap gc-events-seq -o jsonpath='{.data.seq}'
```

## Phase 1 Limitations

| Limitation | Workaround |
|---|---|
| No worktree isolation | Use `pre_start` scripts for worktree setup, or clone repo in Dockerfile |
| No `overlay_dir` | Bake config files into the agent image |
| No formula symlinks | Skip formulas in Phase 1 |
| Controller on laptop | Fine for Phase 1; in-cluster is future work |

`session_setup` commands and `session_setup_script` are supported — they
execute inside the pod via `kubectl exec` after the pod reaches Ready.

## Troubleshooting

### Check Dolt connectivity from inside the cluster

```bash
kubectl -n gc run -it --rm test-dolt --image=dolthub/dolt:latest -- \
  dolt sql --host dolt.gc.svc.cluster.local --port 3307 --user root --no-tls \
  -q "SELECT 1"
```

### Check agent pod logs

```bash
kubectl -n gc logs -l app=gc-agent --tail=50
```

### Attach to an agent's tmux session

```bash
kubectl -n gc exec -it <pod-name> -- tmux attach -t main
```

### Check pod status

```bash
kubectl -n gc get pods -l app=gc-agent -o wide
```

### Agent pod won't start

Verify the image is pullable and the command is correct:

```bash
kubectl -n gc describe pod <pod-name>
kubectl -n gc logs <pod-name>
```
