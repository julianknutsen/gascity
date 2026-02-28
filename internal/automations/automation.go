// Package automations provides parsing, scanning, and gate evaluation for Gas City
// automations. Automations live inside formula directories as automations/<name>/automation.toml
// and inherit the existing 4-layer formula resolution.
package automations

import (
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
)

// Automation is a parsed automation definition from an automation.toml file.
type Automation struct {
	// Name is derived from the subdirectory name (not from TOML).
	Name string `toml:"-"`
	// Description explains what this automation does.
	Description string `toml:"description,omitempty"`
	// Formula is the formula name to dispatch when the gate opens.
	Formula string `toml:"formula"`
	// Gate is the gate type: "cooldown", "cron", "condition", or "manual".
	Gate string `toml:"gate"`
	// Interval is the minimum time between runs (for cooldown gates). Go duration string.
	Interval string `toml:"interval,omitempty"`
	// Schedule is a cron-like expression (for cron gates).
	Schedule string `toml:"schedule,omitempty"`
	// Check is a shell command that returns exit 0 when the formula should run (for condition gates).
	Check string `toml:"check,omitempty"`
	// On is the event type to match (for event gates). E.g., "bead.closed".
	On string `toml:"on,omitempty"`
	// Pool is the target agent/pool for dispatching the wisp.
	Pool string `toml:"pool,omitempty"`
	// Enabled controls whether the automation is active. Defaults to true.
	Enabled *bool `toml:"enabled,omitempty"`
	// Source is the absolute file path to automation.toml (set by scanner, not from TOML).
	Source string `toml:"-"`
	// Rig is the rig name this automation is scoped to. Empty for city-level automations.
	// Set by the scanning caller, not from TOML.
	Rig string `toml:"-"`
}

// ScopedName returns a rig-qualified key for label scoping.
// City-level: "dolt-health". Rig-level: "dolt-health:rig:demo-repo".
func (a *Automation) ScopedName() string {
	if a.Rig == "" {
		return a.Name
	}
	return a.Name + ":rig:" + a.Rig
}

// automationFile wraps the TOML structure with an [automation] header.
type automationFile struct {
	Automation Automation `toml:"automation"`
}

// IsEnabled reports whether the automation is enabled. Defaults to true if not set.
func (a *Automation) IsEnabled() bool {
	if a.Enabled == nil {
		return true
	}
	return *a.Enabled
}

// Parse decodes TOML data into an Automation.
func Parse(data []byte) (Automation, error) {
	var af automationFile
	if _, err := toml.Decode(string(data), &af); err != nil {
		return Automation{}, fmt.Errorf("parsing automation: %w", err)
	}
	return af.Automation, nil
}

// Validate checks an Automation for structural correctness based on its gate type.
func Validate(a Automation) error {
	if a.Formula == "" {
		return fmt.Errorf("automation %q: formula is required", a.Name)
	}
	switch a.Gate {
	case "cooldown":
		if a.Interval == "" {
			return fmt.Errorf("automation %q: cooldown gate requires interval", a.Name)
		}
		if _, err := time.ParseDuration(a.Interval); err != nil {
			return fmt.Errorf("automation %q: invalid interval %q: %w", a.Name, a.Interval, err)
		}
	case "cron":
		if a.Schedule == "" {
			return fmt.Errorf("automation %q: cron gate requires schedule", a.Name)
		}
	case "condition":
		if a.Check == "" {
			return fmt.Errorf("automation %q: condition gate requires check command", a.Name)
		}
	case "event":
		if a.On == "" {
			return fmt.Errorf("automation %q: event gate requires on (event type)", a.Name)
		}
	case "manual":
		// No additional fields required.
	case "":
		return fmt.Errorf("automation %q: gate is required", a.Name)
	default:
		return fmt.Errorf("automation %q: unknown gate type %q", a.Name, a.Gate)
	}
	return nil
}
