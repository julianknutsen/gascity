package k8s

import (
	"encoding/base64"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/steveyegge/gascity/internal/session"
)

// buildPod creates a pod manifest compatible with gc-session-k8s.
// Same labels, annotations, container names, volumes, and tmux-inside-pod
// pattern so mixed-mode migration works.
func buildPod(name string, cfg session.Config, p *Provider) *corev1.Pod {
	podName := SanitizeName(name)
	label := SanitizeLabel(name)
	agentName := cfg.Env["GC_AGENT"]
	if agentName == "" {
		agentName = "unknown"
	}
	agentLabel := SanitizeLabel(agentName)

	// Resolve pod-side working directory.
	// Controller resolves dirs relative to its cityPath; pods use /workspace.
	podWorkDir := "/workspace"
	ctrlCity := cfg.Env["GC_CITY"]
	if ctrlCity != "" && cfg.WorkDir != "" && cfg.WorkDir != ctrlCity {
		if rel, ok := strings.CutPrefix(cfg.WorkDir, ctrlCity+"/"); ok {
			podWorkDir = "/workspace/" + rel
		}
	}

	// Build the command the agent runs. Base64-encode to avoid quoting issues.
	agentCmd := cfg.Command
	if agentCmd == "" {
		agentCmd = "/bin/bash"
	}
	// Remap controller-side city path references to pod-side /workspace.
	// The controller expands {{.ConfigDir}} templates using its own city path
	// (e.g. /city/topologies/...) but pods have files at /workspace/....
	if ctrlCity != "" {
		agentCmd = strings.ReplaceAll(agentCmd, ctrlCity, "/workspace")
	}
	cmdB64 := base64.StdEncoding.EncodeToString([]byte(agentCmd))

	// Pod entrypoint: wait for workspace ready → pre_start → tmux → keepalive.
	var preStartCmds string
	for _, cmd := range cfg.PreStart {
		c := cmd
		if ctrlCity != "" {
			c = strings.ReplaceAll(c, ctrlCity, "/workspace")
		}
		preStartCmds += c + "; "
	}

	credCopy := `mkdir -p $HOME/.claude && cp -rL /tmp/claude-secret/. $HOME/.claude/ 2>/dev/null; mkdir -p /workspace/.gc; `
	wsWait := ""
	if !p.prebaked {
		wsWait = `while [ ! -f /workspace/.gc-workspace-ready ]; do sleep 0.5; done; `
	}
	tmuxCmd := fmt.Sprintf(
		"%s%s%sCMD=$(echo '%s' | base64 -d) && tmux new-session -d -s %s \"$CMD\" && sleep infinity",
		credCopy, wsWait, preStartCmds, cmdB64, tmuxSession,
	)

	// Build environment, remapping K8s-specific vars.
	env := buildPodEnv(cfg.Env, podWorkDir)

	// Build volume mounts for the main container.
	// When prebaked, skip the ws EmptyDir — it would shadow baked image content.
	var mainVolMounts []corev1.VolumeMount
	var volumes []corev1.Volume

	if !p.prebaked {
		mainVolMounts = append(mainVolMounts, corev1.VolumeMount{
			Name: "ws", MountPath: podWorkDir,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "ws", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
	}

	mainVolMounts = append(mainVolMounts, corev1.VolumeMount{
		Name: "claude-config", MountPath: "/tmp/claude-secret", ReadOnly: true,
	})
	volumes = append(volumes, corev1.Volume{
		Name: "claude-config", VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName: "claude-credentials",
				Optional:   boolPtr(true),
			},
		},
	})

	// If GC_CITY differs from work_dir, add a city volume (not needed when prebaked).
	if !p.prebaked && ctrlCity != "" && ctrlCity != cfg.WorkDir {
		mainVolMounts = append(mainVolMounts, corev1.VolumeMount{
			Name: "city", MountPath: ctrlCity,
		})
		volumes = append(volumes, corev1.Volume{
			Name:         "city",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		})
	}

	// Resources.
	resources := buildResources(p)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      podName,
			Namespace: p.namespace,
			Labels: map[string]string{
				"app":        "gc-agent",
				"gc-session": label,
				"gc-agent":   agentLabel,
			},
			Annotations: map[string]string{
				"gc-session-name": name,
			},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:            "agent",
				Image:           p.image,
				ImagePullPolicy: corev1.PullAlways,
				WorkingDir:      podWorkDir,
				Command:         []string{"/bin/sh", "-c"},
				Args:            []string{tmuxCmd},
				Env:             env,
				Stdin:           true,
				TTY:             true,
				Resources:       resources,
				VolumeMounts:    mainVolMounts,
			}},
			Volumes: volumes,
		},
	}

	// Add init container when staging is needed (skip when prebaked).
	if !p.prebaked && needsStaging(cfg, ctrlCity) {
		initVolMounts := []corev1.VolumeMount{
			{Name: "ws", MountPath: "/workspace"},
		}
		if ctrlCity != "" && ctrlCity != cfg.WorkDir {
			initVolMounts = append(initVolMounts, corev1.VolumeMount{
				Name: "city", MountPath: "/city-stage",
			})
		}
		pod.Spec.InitContainers = []corev1.Container{{
			Name:            "stage",
			Image:           p.image,
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"sh", "-c", "while [ ! -f /workspace/.gc-ready ]; do sleep 0.5; done"},
			VolumeMounts:    initVolMounts,
		}}
	}

	return pod
}

// buildPodEnv creates the env var list for the agent container.
// Removes controller-only vars and remaps K8s-specific ones.
func buildPodEnv(cfgEnv map[string]string, podWorkDir string) []corev1.EnvVar {
	// Start with cfg.Env, removing controller-only vars.
	skip := map[string]bool{
		"GC_BEADS":     true,
		"GC_SESSION":   true,
		"GC_EVENTS":    true,
		"GC_DOLT_HOST": true,
		"GC_DOLT_PORT": true,
	}

	var env []corev1.EnvVar
	for k, v := range cfgEnv {
		if skip[k] {
			continue
		}
		val := v
		// Remap GC_CITY and GC_DIR to pod paths.
		switch k {
		case "GC_CITY":
			val = "/workspace"
		case "GC_DIR":
			val = podWorkDir
		}
		env = append(env, corev1.EnvVar{Name: k, Value: val})
	}

	// Add tmux session env so agent's tmux provider uses the same session.
	env = append(env, corev1.EnvVar{Name: "GC_TMUX_SESSION", Value: tmuxSession})
	env = append(env, corev1.EnvVar{Name: "CLAUDE_CONFIG_DIR", Value: "/home/gcagent/.claude"})

	// Inject K8s Dolt discovery for agent-side bd init.
	// GC_DOLT_HOST/PORT are stripped (controller-only), so inject K8s-specific
	// defaults that point to the in-cluster Dolt service.
	envMap := make(map[string]bool, len(env))
	for _, e := range env {
		envMap[e.Name] = true
	}
	if !envMap["GC_K8S_DOLT_HOST"] {
		env = append(env, corev1.EnvVar{
			Name: "GC_K8S_DOLT_HOST", Value: "dolt.gc.svc.cluster.local",
		})
	}
	if !envMap["GC_K8S_DOLT_PORT"] {
		env = append(env, corev1.EnvVar{
			Name: "GC_K8S_DOLT_PORT", Value: "3307",
		})
	}

	return env
}

// needsStaging returns true if the session config requires file staging
// via init container.
func needsStaging(cfg session.Config, ctrlCity string) bool {
	if cfg.OverlayDir != "" {
		return true
	}
	if len(cfg.CopyFiles) > 0 {
		return true
	}
	// Rig agents have a work_dir subdirectory.
	if cfg.WorkDir != "" && cfg.WorkDir != ctrlCity {
		return true
	}
	return false
}

// buildResources creates resource requirements from the provider config.
func buildResources(p *Provider) corev1.ResourceRequirements {
	req := corev1.ResourceRequirements{}
	if p.cpuRequest != "" || p.memRequest != "" {
		req.Requests = corev1.ResourceList{}
		if p.cpuRequest != "" {
			req.Requests[corev1.ResourceCPU] = resource.MustParse(p.cpuRequest)
		}
		if p.memRequest != "" {
			req.Requests[corev1.ResourceMemory] = resource.MustParse(p.memRequest)
		}
	}
	if p.cpuLimit != "" || p.memLimit != "" {
		req.Limits = corev1.ResourceList{}
		if p.cpuLimit != "" {
			req.Limits[corev1.ResourceCPU] = resource.MustParse(p.cpuLimit)
		}
		if p.memLimit != "" {
			req.Limits[corev1.ResourceMemory] = resource.MustParse(p.memLimit)
		}
	}
	return req
}

func boolPtr(b bool) *bool { return &b }
