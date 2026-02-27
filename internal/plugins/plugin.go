// Package plugins provides parsing, scanning, and gate evaluation for Gas City
// plugins. Plugins live inside formula directories as plugins/<name>/plugin.toml
// and inherit the existing 4-layer formula resolution.
package plugins

import (
	"fmt"
	"time"

	"github.com/BurntSushi/toml"
)

// Plugin is a parsed plugin definition from a plugin.toml file.
type Plugin struct {
	// Name is derived from the subdirectory name (not from TOML).
	Name string `toml:"-"`
	// Description explains what this plugin does.
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
	// Enabled controls whether the plugin is active. Defaults to true.
	Enabled *bool `toml:"enabled,omitempty"`
	// Source is the absolute file path to plugin.toml (set by scanner, not from TOML).
	Source string `toml:"-"`
}

// pluginFile wraps the TOML structure with a [plugin] header.
type pluginFile struct {
	Plugin Plugin `toml:"plugin"`
}

// IsEnabled reports whether the plugin is enabled. Defaults to true if not set.
func (p *Plugin) IsEnabled() bool {
	if p.Enabled == nil {
		return true
	}
	return *p.Enabled
}

// Parse decodes TOML data into a Plugin.
func Parse(data []byte) (Plugin, error) {
	var pf pluginFile
	if _, err := toml.Decode(string(data), &pf); err != nil {
		return Plugin{}, fmt.Errorf("parsing plugin: %w", err)
	}
	return pf.Plugin, nil
}

// Validate checks a Plugin for structural correctness based on its gate type.
func Validate(p Plugin) error {
	if p.Formula == "" {
		return fmt.Errorf("plugin %q: formula is required", p.Name)
	}
	switch p.Gate {
	case "cooldown":
		if p.Interval == "" {
			return fmt.Errorf("plugin %q: cooldown gate requires interval", p.Name)
		}
		if _, err := time.ParseDuration(p.Interval); err != nil {
			return fmt.Errorf("plugin %q: invalid interval %q: %w", p.Name, p.Interval, err)
		}
	case "cron":
		if p.Schedule == "" {
			return fmt.Errorf("plugin %q: cron gate requires schedule", p.Name)
		}
	case "condition":
		if p.Check == "" {
			return fmt.Errorf("plugin %q: condition gate requires check command", p.Name)
		}
	case "event":
		if p.On == "" {
			return fmt.Errorf("plugin %q: event gate requires on (event type)", p.Name)
		}
	case "manual":
		// No additional fields required.
	case "":
		return fmt.Errorf("plugin %q: gate is required", p.Name)
	default:
		return fmt.Errorf("plugin %q: unknown gate type %q", p.Name, p.Gate)
	}
	return nil
}
