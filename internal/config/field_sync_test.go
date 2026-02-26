package config

import (
	"reflect"
	"sort"
	"testing"
)

// TestAgentFieldSync verifies that Agent, AgentPatch, and AgentOverride all
// have the same set of overridable fields. When a new field is added to Agent,
// it must also be added to AgentPatch and AgentOverride (or explicitly excluded
// below). This prevents the common bug where a new config field works in
// city.toml but is silently ignored by patches and topology overrides.
//
// See CLAUDE.md "Adding agent config fields" for the convention.
func TestAgentFieldSync(t *testing.T) {
	// Fields that exist on Agent but are NOT overridable via patch/override.
	// Add to this list with a comment explaining why.
	excluded := map[string]string{
		"Name": "identity field, not overridable",
		// Provider-level fields: set during ResolveProvider, not typically
		// overridden per-rig. Agent-level overrides happen in the Agent
		// struct itself (which feeds into ResolveProvider).
		"Args":                   "provider field, set via ResolveProvider",
		"PromptMode":             "provider field, set via ResolveProvider",
		"PromptFlag":             "provider field, set via ResolveProvider",
		"ReadyDelayMs":           "provider field, set via ResolveProvider",
		"ReadyPromptPrefix":      "provider field, set via ResolveProvider",
		"ProcessNames":           "provider field, set via ResolveProvider",
		"EmitsPermissionWarning": "provider field, set via ResolveProvider",
		"WorkQuery":              "agent-specific, derived from name — not a patch concern",
	}

	// Fields on AgentOverride/AgentPatch that don't map 1:1 to Agent fields.
	// "Agent" is the targeting key on AgentOverride, "EnvRemove" is a
	// remove-only modifier that has no Agent equivalent.
	patchOnly := map[string]bool{
		"Agent":     true, // targeting key on AgentOverride
		"EnvRemove": true, // remove modifier, no Agent field
	}

	agentFields := structFields(reflect.TypeOf(Agent{}))
	patchFields := structFields(reflect.TypeOf(AgentPatch{}))
	overrideFields := structFields(reflect.TypeOf(AgentOverride{}))

	// Remove excluded fields from agent set.
	var expected []string
	for _, f := range agentFields {
		if _, ok := excluded[f]; !ok {
			expected = append(expected, f)
		}
	}
	sort.Strings(expected)

	// Check AgentPatch has all expected fields.
	patchSet := toSet(patchFields)
	for _, k := range patchOnly {
		_ = k // just documenting
	}
	var missingPatch []string
	for _, f := range expected {
		if !patchSet[f] {
			missingPatch = append(missingPatch, f)
		}
	}
	if len(missingPatch) > 0 {
		t.Errorf("AgentPatch missing fields that exist on Agent: %v\n"+
			"Add them to AgentPatch or add to the excluded map with justification.", missingPatch)
	}

	// Check AgentOverride has all expected fields.
	overrideSet := toSet(overrideFields)
	var missingOverride []string
	for _, f := range expected {
		if !overrideSet[f] {
			missingOverride = append(missingOverride, f)
		}
	}
	if len(missingOverride) > 0 {
		t.Errorf("AgentOverride missing fields that exist on Agent: %v\n"+
			"Add them to AgentOverride or add to the excluded map with justification.", missingOverride)
	}

	// Check for extra fields on Patch/Override that aren't on Agent or patchOnly.
	agentSet := toSet(agentFields)
	for _, f := range patchFields {
		if !agentSet[f] && !patchOnly[f] {
			t.Errorf("AgentPatch has field %q not found on Agent or patchOnly exclusion list", f)
		}
	}
	for _, f := range overrideFields {
		if !agentSet[f] && !patchOnly[f] {
			t.Errorf("AgentOverride has field %q not found on Agent or patchOnly exclusion list", f)
		}
	}
}

func structFields(t reflect.Type) []string {
	var names []string
	for i := 0; i < t.NumField(); i++ {
		names = append(names, t.Field(i).Name)
	}
	return names
}

func toSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
