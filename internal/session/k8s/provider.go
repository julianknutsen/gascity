package k8s

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/steveyegge/gascity/internal/session"
)

// Compile-time interface check.
var _ session.Provider = (*Provider)(nil)

// Provider is a native Kubernetes session provider using client-go.
// Eliminates subprocess overhead by making direct API calls over reused
// HTTP/2 connections. Pod manifests are compatible with gc-session-k8s.
type Provider struct {
	ops        k8sOps
	namespace  string
	image      string
	k8sContext string
	cpuRequest string
	memRequest string
	cpuLimit   string
	memLimit   string
	prebaked   bool      // skip staging + init container for prebaked images
	stderr     io.Writer // warning output (default os.Stderr)
}

// NewProvider creates a K8s session provider.
// Configuration is read from environment variables (matching gc-session-k8s):
//   - GC_K8S_NAMESPACE — namespace (default: "gc")
//   - GC_K8S_IMAGE — container image (required for Start)
//   - GC_K8S_CONTEXT — kubectl context (default: current)
//   - GC_K8S_CPU_REQUEST, GC_K8S_MEM_REQUEST — resource requests
//   - GC_K8S_CPU_LIMIT, GC_K8S_MEM_LIMIT — resource limits
//
// Uses rest.InClusterConfig() when running in a pod, falls back to
// clientcmd.BuildConfigFromFlags() for local development.
func NewProvider() (*Provider, error) {
	namespace := envOrDefault("GC_K8S_NAMESPACE", "gc")
	image := os.Getenv("GC_K8S_IMAGE")
	k8sContext := os.Getenv("GC_K8S_CONTEXT")

	restConfig, err := buildRESTConfig(k8sContext)
	if err != nil {
		return nil, fmt.Errorf("building K8s config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating K8s clientset: %w", err)
	}

	return &Provider{
		ops: &realK8sOps{
			clientset:  clientset,
			restConfig: restConfig,
			namespace:  namespace,
		},
		namespace:  namespace,
		image:      image,
		k8sContext: k8sContext,
		cpuRequest: envOrDefault("GC_K8S_CPU_REQUEST", "500m"),
		memRequest: envOrDefault("GC_K8S_MEM_REQUEST", "1Gi"),
		cpuLimit:   envOrDefault("GC_K8S_CPU_LIMIT", "2"),
		memLimit:   envOrDefault("GC_K8S_MEM_LIMIT", "4Gi"),
		prebaked:   os.Getenv("GC_K8S_PREBAKED") == "true",
		stderr:     os.Stderr,
	}, nil
}

// newProviderWithOps creates a provider with a custom k8sOps (for testing).
func newProviderWithOps(ops k8sOps) *Provider {
	return &Provider{
		ops:        ops,
		namespace:  "test-ns",
		image:      "test-image:latest",
		cpuRequest: "500m",
		memRequest: "1Gi",
		cpuLimit:   "2",
		memLimit:   "4Gi",
		stderr:     io.Discard,
	}
}

// Start creates a new K8s pod running a tmux session with the agent command.
func (p *Provider) Start(name string, cfg session.Config) error {
	if p.image == "" {
		return fmt.Errorf("starting session %q: GC_K8S_IMAGE is required", name)
	}

	ctx := context.Background()
	podName := SanitizeName(name)
	label := SanitizeLabel(name)

	// Check for existing pod (any phase).
	existing, err := p.ops.listPods(ctx, "gc-session="+label, "")
	if err == nil && len(existing) > 0 {
		pod := &existing[0]
		if pod.Status.Phase == corev1.PodRunning {
			// Check if tmux is alive — stale pod detection.
			_, tmuxErr := p.ops.execInPod(ctx, pod.Name, "agent",
				[]string{"tmux", "has-session", "-t", tmuxSession}, nil)
			if tmuxErr == nil {
				return fmt.Errorf("session %q already exists (pod: %s)", name, pod.Name)
			}
			// Stale pod — tmux dead, recreate.
		}
		// Clean up existing pod.
		_ = p.ops.deletePod(ctx, pod.Name, 5)
		_ = waitForDeletion(ctx, p.ops, pod.Name, 30*time.Second)
	}

	// Build and create pod.
	pod := buildPod(name, cfg, p)
	_, err = p.ops.createPod(ctx, pod)
	if err != nil {
		return fmt.Errorf("creating pod for session %q: %w", name, err)
	}

	ctrlCity := cfg.Env["GC_CITY"]

	if !p.prebaked {
		// Stage files via init container if needed.
		if needsStaging(cfg, ctrlCity) {
			if err := stageFiles(ctx, p.ops, podName, cfg, ctrlCity, p.stderr); err != nil {
				return fmt.Errorf("staging files for session %q: %w", name, err)
			}
		}
	}

	// Wait for main container to be running.
	if err := waitForPodRunning(ctx, p.ops, podName, 120*time.Second); err != nil {
		return fmt.Errorf("waiting for pod %q: %w", podName, err)
	}

	if !p.prebaked {
		// Initialize the city inside the pod.
		if ctrlCity != "" {
			if err := initCityInPod(ctx, p.ops, podName, ctrlCity); err != nil {
				fmt.Fprintf(p.stderr, "gc: warning: initCityInPod for %s: %v\n", podName, err) //nolint:errcheck
			}
		}

		// Signal entrypoint to proceed.
		if _, err := p.ops.execInPod(ctx, podName, "agent",
			[]string{"touch", "/workspace/.gc-workspace-ready"}, nil); err != nil {
			fmt.Fprintf(p.stderr, "gc: warning: touch .gc-workspace-ready in %s: %v\n", podName, err) //nolint:errcheck
		}
	}

	// Initialize .beads/ in the pod (runs in both prebaked and non-prebaked paths).
	// Resolve pod-side working directory.
	podWorkDir := "/workspace"
	if ctrlCity != "" && cfg.WorkDir != "" && cfg.WorkDir != ctrlCity {
		if rel, ok := strings.CutPrefix(cfg.WorkDir, ctrlCity+"/"); ok {
			podWorkDir = "/workspace/" + rel
		}
	}
	if err := initBeadsInPod(ctx, p.ops, podName, cfg, podWorkDir); err != nil {
		fmt.Fprintf(p.stderr, "gc: warning: initBeadsInPod for %s: %v\n", podName, err) //nolint:errcheck
	}

	// Wait for tmux session.
	if err := waitForTmux(ctx, p.ops, podName, 60*time.Second); err != nil {
		return fmt.Errorf("waiting for tmux in pod %q: %w", podName, err)
	}

	// Enable pane logging for diagnostics.
	_, _ = p.ops.execInPod(ctx, podName, "agent",
		[]string{"tmux", "pipe-pane", "-t", tmuxSession, "-o", "cat >> /tmp/agent-output.log"}, nil)

	// Run session_setup commands inside the pod.
	for _, cmd := range cfg.SessionSetup {
		if cmd == "" {
			continue
		}
		_, _ = p.ops.execInPod(ctx, podName, "agent",
			[]string{"sh", "-c", cmd}, nil)
	}

	// Run session_setup_script.
	if cfg.SessionSetupScript != "" {
		script, err := os.ReadFile(cfg.SessionSetupScript)
		if err == nil {
			_, _ = p.ops.execInPod(ctx, podName, "agent",
				[]string{"sh"}, strings.NewReader(string(script)))
		}
	}

	return nil
}

// Stop deletes the pod for the named session. Idempotent.
func (p *Provider) Stop(name string) error {
	ctx := context.Background()
	label := SanitizeLabel(name)

	pods, err := p.ops.listPods(ctx, "gc-session="+label, "")
	if err != nil {
		return nil // best-effort
	}
	for i := range pods {
		_ = p.ops.deletePod(ctx, pods[i].Name, 5)
	}
	return nil
}

// Interrupt sends Ctrl-C to the tmux session inside the pod.
func (p *Provider) Interrupt(name string) error {
	ctx := context.Background()
	podName, err := p.findRunningPod(ctx, name)
	if err != nil {
		return nil // best-effort
	}
	_, _ = p.ops.execInPod(ctx, podName, "agent",
		[]string{"tmux", "send-keys", "-t", tmuxSession, "C-c"}, nil)
	return nil
}

// IsRunning reports whether the session has a running pod with a live tmux session.
func (p *Provider) IsRunning(name string) bool {
	ctx := context.Background()
	podName, err := p.findRunningPod(ctx, name)
	if err != nil {
		return false
	}
	// Pod Running + tmux session alive.
	_, err = p.ops.execInPod(ctx, podName, "agent",
		[]string{"tmux", "has-session", "-t", tmuxSession}, nil)
	return err == nil
}

// Attach shells out to kubectl exec -it for full TTY passthrough.
func (p *Provider) Attach(name string) error {
	ctx := context.Background()
	podName, err := p.findRunningPod(ctx, name)
	if err != nil {
		return fmt.Errorf("attach: no running pod for session %q", name)
	}

	args := []string{}
	if p.k8sContext != "" {
		args = append(args, "--context", p.k8sContext)
	}
	args = append(args, "-n", p.namespace, "exec", "-it", podName, "--",
		"tmux", "attach", "-t", tmuxSession)

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// ProcessAlive checks if the named processes are running inside the pod.
func (p *Provider) ProcessAlive(name string, processNames []string) bool {
	if len(processNames) == 0 {
		return true
	}
	ctx := context.Background()
	label := SanitizeLabel(name)

	pods, err := p.ops.listPods(ctx, "gc-session="+label, "")
	if err != nil || len(pods) == 0 {
		return false
	}
	pod := &pods[0]

	// Check deletionTimestamp — pod in graceful shutdown is not alive.
	if pod.DeletionTimestamp != nil {
		return false
	}
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}

	for _, pname := range processNames {
		_, err := p.ops.execInPod(ctx, pod.Name, "agent",
			[]string{"pgrep", "-f", pname}, nil)
		if err == nil {
			return true
		}
	}
	return false
}

// Nudge types a message into the tmux session followed by Enter.
func (p *Provider) Nudge(name, message string) error {
	ctx := context.Background()
	podName, err := p.findRunningPod(ctx, name)
	if err != nil {
		return nil // best-effort
	}
	_, _ = p.ops.execInPod(ctx, podName, "agent",
		[]string{"tmux", "send-keys", "-t", tmuxSession, message, "Enter"}, nil)
	return nil
}

// SendKeys sends bare keystrokes to the tmux session.
func (p *Provider) SendKeys(name string, keys ...string) error {
	ctx := context.Background()
	podName, err := p.findRunningPod(ctx, name)
	if err != nil {
		return nil // best-effort
	}
	args := []string{"tmux", "send-keys", "-t", tmuxSession}
	args = append(args, keys...)
	_, _ = p.ops.execInPod(ctx, podName, "agent", args, nil)
	return nil
}

// SetMeta stores a key-value pair in the tmux environment.
func (p *Provider) SetMeta(name, key, value string) error {
	ctx := context.Background()
	podName, err := p.findPod(ctx, name)
	if err != nil {
		return nil // best-effort
	}
	_, _ = p.ops.execInPod(ctx, podName, "agent",
		[]string{"tmux", "set-environment", "-t", tmuxSession, key, value}, nil)
	return nil
}

// GetMeta retrieves a metadata value from the tmux environment.
func (p *Provider) GetMeta(name, key string) (string, error) {
	ctx := context.Background()
	podName, err := p.findPod(ctx, name)
	if err != nil {
		return "", nil
	}
	output, err := p.ops.execInPod(ctx, podName, "agent",
		[]string{"tmux", "show-environment", "-t", tmuxSession, key}, nil)
	if err != nil {
		return "", nil
	}
	output = strings.TrimSpace(output)
	// tmux output: "KEY=VALUE" (set), "-KEY" (unset).
	if strings.HasPrefix(output, "-") {
		return "", nil // explicitly unset
	}
	if _, val, ok := strings.Cut(output, "="); ok {
		return val, nil
	}
	return "", nil
}

// RemoveMeta removes a metadata key from the tmux environment.
func (p *Provider) RemoveMeta(name, key string) error {
	ctx := context.Background()
	podName, err := p.findPod(ctx, name)
	if err != nil {
		return nil // best-effort
	}
	_, _ = p.ops.execInPod(ctx, podName, "agent",
		[]string{"tmux", "set-environment", "-t", tmuxSession, "-u", key}, nil)
	return nil
}

// Peek captures the last N lines of tmux pane output.
func (p *Provider) Peek(name string, lines int) (string, error) {
	ctx := context.Background()
	podName, err := p.findRunningPod(ctx, name)
	if err != nil {
		return "", nil
	}
	var cmd []string
	if lines > 0 {
		cmd = []string{"tmux", "capture-pane", "-t", tmuxSession, "-p", "-S", "-" + strconv.Itoa(lines)}
	} else {
		cmd = []string{"tmux", "capture-pane", "-t", tmuxSession, "-p", "-S", "-"}
	}
	output, err := p.ops.execInPod(ctx, podName, "agent", cmd, nil)
	if err != nil {
		return "", nil
	}
	return output, nil
}

// ListRunning returns names of all running sessions with the given prefix.
func (p *Provider) ListRunning(prefix string) ([]string, error) {
	ctx := context.Background()
	pods, err := p.ops.listPods(ctx, "app=gc-agent", "status.phase=Running")
	if err != nil {
		return nil, err
	}
	var names []string
	for i := range pods {
		pod := &pods[i]
		// Prefer annotation (raw name) over label (sanitized).
		name := pod.Annotations["gc-session-name"]
		if name == "" {
			name = pod.Labels["gc-session"]
		}
		if name == "" {
			continue
		}
		if prefix == "" || strings.HasPrefix(name, prefix) {
			names = append(names, name)
		}
	}
	return names, nil
}

// GetLastActivity returns the time of the last I/O in the tmux session.
func (p *Provider) GetLastActivity(name string) (time.Time, error) {
	ctx := context.Background()
	podName, err := p.findRunningPod(ctx, name)
	if err != nil {
		return time.Time{}, nil
	}
	output, err := p.ops.execInPod(ctx, podName, "agent",
		[]string{"tmux", "display-message", "-t", tmuxSession, "-p", "#{session_activity}"}, nil)
	if err != nil {
		return time.Time{}, nil
	}
	epoch := strings.TrimSpace(output)
	if epoch == "" {
		return time.Time{}, nil
	}
	secs, err := strconv.ParseInt(epoch, 10, 64)
	if err != nil {
		return time.Time{}, nil
	}
	return time.Unix(secs, 0), nil
}

// ClearScrollback clears the tmux scrollback buffer.
func (p *Provider) ClearScrollback(name string) error {
	ctx := context.Background()
	podName, err := p.findRunningPod(ctx, name)
	if err != nil {
		return nil // best-effort
	}
	_, _ = p.ops.execInPod(ctx, podName, "agent",
		[]string{"tmux", "clear-history", "-t", tmuxSession}, nil)
	return nil
}

// CopyTo copies a local file/directory into the pod via tar.
func (p *Provider) CopyTo(name, src, relDst string) error {
	ctx := context.Background()
	podName, err := p.findRunningPod(ctx, name)
	if err != nil {
		return nil // best-effort
	}
	dst := "/workspace"
	if relDst != "" {
		dst = "/workspace/" + relDst
	}
	return copyToPod(ctx, p.ops, podName, "agent", src, dst)
}

// --- Internal helpers ---

// findRunningPod finds a running pod by session label.
func (p *Provider) findRunningPod(ctx context.Context, name string) (string, error) {
	label := SanitizeLabel(name)
	pods, err := p.ops.listPods(ctx, "gc-session="+label, "status.phase=Running")
	if err != nil {
		return "", err
	}
	if len(pods) == 0 {
		return "", fmt.Errorf("no running pod for session %q", name)
	}
	return pods[0].Name, nil
}

// findPod finds a pod by session label (any phase).
func (p *Provider) findPod(ctx context.Context, name string) (string, error) {
	label := SanitizeLabel(name)
	pods, err := p.ops.listPods(ctx, "gc-session="+label, "")
	if err != nil {
		return "", err
	}
	if len(pods) == 0 {
		return "", fmt.Errorf("no pod for session %q", name)
	}
	return pods[0].Name, nil
}

// waitForDeletion waits for a pod to be deleted.
func waitForDeletion(ctx context.Context, ops k8sOps, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := ops.getPod(ctx, name)
		if err != nil {
			return nil // gone
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("pod %s not deleted after %s", name, timeout)
}

// waitForPodRunning waits for the pod to reach Running phase.
func waitForPodRunning(ctx context.Context, ops k8sOps, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pod, err := ops.getPod(ctx, name)
		if err != nil {
			time.Sleep(time.Second)
			continue
		}
		switch pod.Status.Phase {
		case corev1.PodRunning:
			return nil
		case corev1.PodFailed:
			return fmt.Errorf("pod %s failed", name)
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("pod %s not running after %s", name, timeout)
}

// waitForTmux waits for the tmux session to be available inside the pod.
func waitForTmux(ctx context.Context, ops k8sOps, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		_, err := ops.execInPod(ctx, name, "agent",
			[]string{"tmux", "has-session", "-t", tmuxSession}, nil)
		if err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("tmux session not ready in pod %s after %s", name, timeout)
}

// initCityInPod copies the city directory and runs gc init inside the pod.
func initCityInPod(ctx context.Context, ops k8sOps, podName, ctrlCity string) error {
	// Copy city dir (excluding .gc/) into the pod.
	if err := copyDirToPod(ctx, ops, podName, "agent", ctrlCity, "/tmp/city-src"); err != nil {
		return err
	}
	// Run gc init --from.
	_, err := ops.execInPod(ctx, podName, "agent",
		[]string{"gc", "init", "--from", "/tmp/city-src", "/workspace"}, nil)
	if err != nil {
		return err
	}
	// Clean up.
	_, _ = ops.execInPod(ctx, podName, "agent",
		[]string{"rm", "-rf", "/tmp/city-src"}, nil)
	return nil
}

// initBeadsInPod runs bd init --server inside the pod when Dolt env vars are
// present. This eliminates the need for every agent script to include bd init
// boilerplate. Runs in both prebaked and non-prebaked paths.
func initBeadsInPod(ctx context.Context, ops k8sOps, podName string, cfg session.Config, workDir string) error {
	// Determine Dolt host: prefer GC_K8S_DOLT_HOST, fall back to GC_DOLT_HOST,
	// then default K8s service address.
	doltHost := cfg.Env["GC_K8S_DOLT_HOST"]
	if doltHost == "" {
		doltHost = cfg.Env["GC_DOLT_HOST"]
	}
	if doltHost == "" {
		doltHost = "dolt.gc.svc.cluster.local"
	}
	doltPort := cfg.Env["GC_K8S_DOLT_PORT"]
	if doltPort == "" {
		doltPort = cfg.Env["GC_DOLT_PORT"]
	}
	if doltPort == "" {
		doltPort = "3307"
	}

	// Derive rig prefix from rig directory name: split on hyphens, first letter
	// of each part (e.g., "demo-repo" → "dr"). Same algorithm as gc-controller-k8s
	// and mock scripts.
	rigName := workDir
	if i := strings.LastIndex(rigName, "/"); i >= 0 {
		rigName = rigName[i+1:]
	}
	var prefix strings.Builder
	for _, part := range strings.Split(rigName, "-") {
		if len(part) > 0 {
			prefix.WriteByte(part[0])
		}
	}

	// Run: yes | bd init --server --server-host HOST --server-port PORT -p PREFIX --skip-hooks
	initCmd := fmt.Sprintf(
		"cd %s && yes | bd init --server --server-host %s --server-port %s -p %s --skip-hooks",
		workDir, doltHost, doltPort, prefix.String(),
	)
	_, err := ops.execInPod(ctx, podName, "agent",
		[]string{"sh", "-c", initCmd}, nil)
	return err
}

func buildRESTConfig(k8sContext string) (*rest.Config, error) {
	// Try in-cluster first.
	cfg, err := rest.InClusterConfig()
	if err == nil {
		return cfg, nil
	}
	// Fall back to kubeconfig.
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	if k8sContext != "" {
		overrides.CurrentContext = k8sContext
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
