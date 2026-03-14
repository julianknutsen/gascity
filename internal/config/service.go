package config

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gastownhall/gascity/internal/citylayout"
)

var validServiceName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// Service declares a workspace-owned HTTP service mounted under /svc/{name}.
type Service struct {
	// Name is the unique service identifier within a workspace.
	Name string `toml:"name" jsonschema:"required"`
	// Kind selects how the service is implemented.
	Kind string `toml:"kind,omitempty" jsonschema:"enum=workflow"`
	// PublishMode declares how the service is intended to be published.
	// v0 supports private services and direct reuse of the API listener.
	PublishMode string `toml:"publish_mode,omitempty" jsonschema:"enum=private,enum=direct"`
	// StateRoot overrides the managed service state root. Defaults to
	// .gc/services/{name}. The path must stay within .gc/services/.
	StateRoot string `toml:"state_root,omitempty"`
	// Workflow configures controller-owned workflow services.
	Workflow ServiceWorkflowConfig `toml:"workflow,omitempty"`
	// SourceDir records pack provenance for pack-stamped services.
	SourceDir string `toml:"-" json:"-"`
}

// KindOrDefault returns the normalized service kind.
func (s Service) KindOrDefault() string {
	if s.Kind == "" {
		return "workflow"
	}
	return s.Kind
}

// MountPathOrDefault returns the service mount path.
func (s Service) MountPathOrDefault() string {
	return "/svc/" + s.Name
}

// PublishModeOrDefault returns the normalized publish mode.
func (s Service) PublishModeOrDefault() string {
	if s.PublishMode == "" {
		return "private"
	}
	return s.PublishMode
}

// StateRootOrDefault returns the managed runtime root for the service.
func (s Service) StateRootOrDefault() string {
	if s.StateRoot != "" {
		return filepath.Clean(s.StateRoot)
	}
	return filepath.Join(citylayout.RuntimeServicesRoot, s.Name)
}

// ServiceWorkflowConfig configures controller-owned workflow services.
type ServiceWorkflowConfig struct {
	// Contract selects the built-in workflow handler.
	Contract string `toml:"contract,omitempty"`
}

// ValidateServices checks workspace service declarations for configuration
// errors that would prevent runtime activation.
func ValidateServices(services []Service) error {
	seen := make(map[string]bool, len(services))
	for i, svc := range services {
		if svc.Name == "" {
			return fmt.Errorf("service[%d]: name is required", i)
		}
		if !validServiceName.MatchString(svc.Name) {
			return fmt.Errorf("service %q: name must match [a-zA-Z0-9][a-zA-Z0-9_-]*", svc.Name)
		}
		if seen[svc.Name] {
			if svc.SourceDir != "" {
				return fmt.Errorf("service %q: duplicate name (from %q)", svc.Name, svc.SourceDir)
			}
			return fmt.Errorf("service %q: duplicate name", svc.Name)
		}
		seen[svc.Name] = true

		if svc.KindOrDefault() != "workflow" {
			return fmt.Errorf("service %q: kind must be \"workflow\", got %q", svc.Name, svc.Kind)
		}
		switch svc.PublishModeOrDefault() {
		case "private", "direct":
		default:
			return fmt.Errorf("service %q: publish_mode must be \"private\" or \"direct\", got %q", svc.Name, svc.PublishMode)
		}

		root := filepath.ToSlash(filepath.Clean(svc.StateRootOrDefault()))
		prefix := filepath.ToSlash(citylayout.RuntimeServicesRoot) + "/"
		if !strings.HasPrefix(root, prefix) {
			return fmt.Errorf("service %q: state_root must stay under %s/, got %q", svc.Name, filepath.ToSlash(citylayout.RuntimeServicesRoot), svc.StateRootOrDefault())
		}
		if strings.Contains(root, "../") || strings.HasSuffix(root, "/..") {
			return fmt.Errorf("service %q: state_root may not traverse upward, got %q", svc.Name, svc.StateRootOrDefault())
		}

		if svc.Workflow.Contract == "" {
			return fmt.Errorf("service %q: workflow.contract is required for workflow services", svc.Name)
		}
	}
	return nil
}
