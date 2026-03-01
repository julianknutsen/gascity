package automations

import "fmt"

// Override modifies a scanned automation's scheduling fields.
// Uses pointer fields to distinguish "not set" from "set to zero value."
// Mirrors config.AutomationOverride but lives in the automations package
// to avoid a circular dependency.
type Override struct {
	Name     string
	Rig      string
	Enabled  *bool
	Gate     *string
	Interval *string
	Schedule *string
	Check    *string
	On       *string
	Pool     *string
	Timeout  *string
}

// ApplyOverrides applies each override to the matching automation in aa.
// Matching is by name, optionally scoped by rig. Returns an error if an
// override targets a nonexistent automation (following the agent override
// pattern where unmatched targets are errors, not silent no-ops).
func ApplyOverrides(aa []Automation, overrides []Override) error {
	for i, ov := range overrides {
		if ov.Name == "" {
			return fmt.Errorf("automations.overrides[%d]: name is required", i)
		}
		found := false
		for j := range aa {
			if aa[j].Name != ov.Name {
				continue
			}
			if ov.Rig != "" && aa[j].Rig != ov.Rig {
				continue
			}
			applyOverride(&aa[j], &ov)
			found = true
		}
		if !found {
			if ov.Rig != "" {
				return fmt.Errorf("automations.overrides[%d]: automation %q (rig %q) not found", i, ov.Name, ov.Rig)
			}
			return fmt.Errorf("automations.overrides[%d]: automation %q not found", i, ov.Name)
		}
	}
	return nil
}

func applyOverride(a *Automation, ov *Override) {
	if ov.Enabled != nil {
		a.Enabled = ov.Enabled
	}
	if ov.Gate != nil {
		a.Gate = *ov.Gate
	}
	if ov.Interval != nil {
		a.Interval = *ov.Interval
	}
	if ov.Schedule != nil {
		a.Schedule = *ov.Schedule
	}
	if ov.Check != nil {
		a.Check = *ov.Check
	}
	if ov.On != nil {
		a.On = *ov.On
	}
	if ov.Pool != nil {
		a.Pool = *ov.Pool
	}
	if ov.Timeout != nil {
		a.Timeout = *ov.Timeout
	}
}
