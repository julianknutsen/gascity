# K8s Session Provider — Known Issues & Backlog

Robustness issues identified during initial implementation review.
Items marked FIXED have been addressed.

## P0 — Will break in normal use

### 1. YAML injection in `start` handler — FIXED

Pod manifest is now generated as JSON via `jq` — all values are properly
escaped. No string interpolation into YAML.

### 2. `nudge` writes to dead-letter file — FIXED

Switched to tmux-inside-pod architecture. The pod runs tmux as its
session manager; nudge sends keystrokes via `kubectl exec -- tmux
send-keys`. Same semantics as the local tmux provider.

### 3. Pod name / label value sanitization — FIXED

Added `sanitize_label()` helper. All label queries go through it.
Pod names and label values are sanitized consistently.

## P1 — Fragile under real conditions

### 4. `peek` returns logs, not terminal output — FIXED

Switched to tmux-inside-pod. `peek` now uses `kubectl exec -- tmux
capture-pane -p`, which returns real terminal scrollback content.
Same semantics as the local tmux provider.

### 5. `start` returns before pod is schedulable — FIXED

`start` now calls `kubectl wait --for=condition=Ready` with a 120s
timeout after `kubectl apply`. Reports failure with phase info if
the pod doesn't reach Running.

### 6. `process-alive` race with pod termination

If the main process exits, pod enters Completed state. `kubectl exec`
fails on non-Running pods. `get_pod_name_by_label` filters to Running,
so `process-alive` returns "false" (correct). Low priority.

## P2 — Phase 1 acceptable, fix later

### 7. No `get-last-activity` support — FIXED

Now queries tmux `#{session_activity}` via `kubectl exec` and converts
the unix timestamp to RFC3339. Health patrol can detect idle agents.

### 8. `clear-scrollback` — FIXED

Now delegates to `kubectl exec -- tmux clear-history`. Works with
tmux-inside-pod architecture.
