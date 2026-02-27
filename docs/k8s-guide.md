# Kubernetes Deployment Guide

Run Gas City agents as Kubernetes pods with Dolt as a K8s StatefulSet.
The `gc` controller runs on your laptop and manages agent pods via
`kubectl`. No new Go code required — agent sessions use the exec session
provider with the `gc-session-k8s` script.

## Architecture

```
┌─── Your Laptop ───────────────┐     ┌─── K8s Cluster ──────────────┐
│                                │     │                               │
│  gc controller                 │     │  ┌─────────────────────┐     │
│    │                           │     │  │ Dolt StatefulSet     │     │
│    ├─ kubectl apply (start)    │────>│  │  port 3307           │     │
│    ├─ kubectl delete (stop)    │     │  │  PVC: 10Gi           │     │
│    ├─ kubectl exec (nudge)     │     │  └─────────────────────┘     │
│    └─ kubectl logs (peek)      │     │            │                  │
│                                │     │  ┌─────────┴───────────┐     │
│  gc-session-k8s script         │     │  │    dolt Service      │     │
│    translates session ops      │     │  │    ClusterIP:3307    │     │
│    to kubectl commands         │     │  └─────────┬───────────┘     │
│                                │     │            │                  │
└────────────────────────────────┘     │  ┌─────────┴───────────┐     │
                                       │  │  Agent Pod (mayor)   │     │
                                       │  │  Agent Pod (coder-1) │     │
                                       │  │  Agent Pod (coder-2) │     │
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

The exec session provider calls `gc-session-k8s` for each session operation.
The script translates these to `kubectl` commands:

| Session Op | kubectl Command |
|------------|-----------------|
| `start` | `kubectl apply -f -` (pod manifest) |
| `stop` | `kubectl delete pod -l gc-session=<name>` |
| `is-running` | `kubectl get pod` → check phase=Running |
| `interrupt` | `kubectl exec -- kill -INT 1` |
| `attach` | `kubectl exec -it -- /bin/bash` |
| `nudge` | `kubectl exec -i -- sh -c 'cat >> /tmp/gc-nudge'` |
| `peek` | `kubectl logs --tail=<n>` |
| `set-meta` | `kubectl annotate pod` |
| `get-meta` | `kubectl get pod -o jsonpath` |
| `list-running` | `kubectl get pods -l app=gc-agent` |

## Phase 1 Limitations

| Limitation | Workaround |
|---|---|
| No worktree isolation | Use `isolation = "none"`, clone repo in Dockerfile |
| No `overlay_dir` | Bake config files into the agent image |
| No `session_setup` | Use container entrypoint instead |
| No formula symlinks | Skip formulas in Phase 1 |
| No `get-last-activity` | Returns empty (graceful degradation) |
| Controller on laptop | Fine for Phase 1; in-cluster is future work |

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
