package config

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/steveyegge/gascity/internal/fsys"
)

// topologyFile is the expected filename inside a topology directory.
const topologyFile = "topology.toml"

// currentTopologySchema is the supported topology schema version.
const currentTopologySchema = 1

// topologyConfig is the TOML structure of a topology.toml file.
// It has a [topology] metadata header and agent definitions.
type topologyConfig struct {
	Topology  TopologyMeta            `toml:"topology"`
	Agents    []Agent                 `toml:"agents"`
	Providers map[string]ProviderSpec `toml:"providers,omitempty"`
	Formulas  FormulasConfig          `toml:"formulas,omitempty"`
}

// ExpandTopologies resolves topology references on all rigs. For each rig
// with a topology field set, it loads the topology directory, stamps agents
// with dir = rig.Name, resolves prompt_template paths relative to the
// topology directory, and appends the agents to the city config.
//
// Overrides from the rig are applied to the stamped agents. All expansion
// happens before validation — downstream sees a flat City struct.
//
// rigFormulaDirs is populated with per-rig topology formula directories
// (Layer 3). cityRoot is the city directory (parent of city.toml), used
// for path resolution.
func ExpandTopologies(cfg *City, fs fsys.FS, cityRoot string, rigFormulaDirs map[string]string) error {
	var expanded []Agent
	for i := range cfg.Rigs {
		rig := &cfg.Rigs[i]
		if rig.Topology == "" {
			continue
		}

		topoDir := resolveConfigPath(rig.Topology, cityRoot, cityRoot)
		topoPath := filepath.Join(topoDir, topologyFile)

		agents, providers, formulaDir, periodic, err := loadTopology(fs, topoPath, topoDir, cityRoot, rig.Name)
		if err != nil {
			return fmt.Errorf("rig %q topology %q: %w", rig.Name, rig.Topology, err)
		}

		// Record rig topology formula dir (Layer 3).
		if formulaDir != "" && rigFormulaDirs != nil {
			rigFormulaDirs[rig.Name] = formulaDir
		}

		// Apply per-rig overrides.
		if err := applyOverrides(agents, rig.Overrides, rig.Name); err != nil {
			return fmt.Errorf("rig %q: %w", rig.Name, err)
		}

		expanded = append(expanded, agents...)

		// Merge topology providers into city (additive, no overwrite).
		if len(providers) > 0 {
			if cfg.Providers == nil {
				cfg.Providers = make(map[string]ProviderSpec)
			}
			for name, spec := range providers {
				if _, exists := cfg.Providers[name]; !exists {
					cfg.Providers[name] = spec
				}
			}
		}

		// Merge topology periodic formulas (city-level wins by formula name).
		cfg.Formulas.Periodic = mergePeriodicFormulas(cfg.Formulas.Periodic, periodic)
	}
	cfg.Agents = append(cfg.Agents, expanded...)
	return nil
}

// ExpandCityTopology loads the city-level topology from workspace.topology.
// City topology agents are stamped with dir="" (city-scoped) and prepended
// to the agent list. Returns the resolved formula dir from the topology
// (empty if none). cityRoot is the city directory.
func ExpandCityTopology(cfg *City, fs fsys.FS, cityRoot string) (string, error) {
	if cfg.Workspace.Topology == "" {
		return "", nil
	}

	topoDir := resolveConfigPath(cfg.Workspace.Topology, cityRoot, cityRoot)
	topoPath := filepath.Join(topoDir, topologyFile)

	agents, providers, formulaDir, periodic, err := loadTopology(fs, topoPath, topoDir, cityRoot, "")
	if err != nil {
		return "", fmt.Errorf("city topology %q: %w", cfg.Workspace.Topology, err)
	}

	// City topology agents go at the front (before user-defined agents).
	cfg.Agents = append(agents, cfg.Agents...)

	// Merge topology providers (additive, no overwrite).
	if len(providers) > 0 {
		if cfg.Providers == nil {
			cfg.Providers = make(map[string]ProviderSpec)
		}
		for name, spec := range providers {
			if _, exists := cfg.Providers[name]; !exists {
				cfg.Providers[name] = spec
			}
		}
	}

	// Merge topology periodic formulas (city-level wins by formula name).
	cfg.Formulas.Periodic = mergePeriodicFormulas(cfg.Formulas.Periodic, periodic)

	return formulaDir, nil
}

// ComputeFormulaLayers builds the FormulaLayers from the resolved formula
// directories. Each layer slice is ordered lowest→highest priority.
//
// Parameters:
//   - cityTopoFormulas: formula dir from city topology (Layer 1), "" if none
//   - cityLocalFormulas: formula dir from city [formulas] section (Layer 2), "" if none
//   - rigTopoFormulas: map[rigName]formulaDir from rig topologies (Layer 3)
//   - rigs: rig configs (for rig-local FormulasDir, Layer 4)
//   - cityRoot: city directory for resolving relative paths
func ComputeFormulaLayers(cityTopoFormulas, cityLocalFormulas string, rigTopoFormulas map[string]string, rigs []Rig, cityRoot string) FormulaLayers {
	fl := FormulaLayers{
		Rigs: make(map[string][]string),
	}

	// City layers (apply to city-scoped agents and as base for all rigs).
	var cityLayers []string
	if cityTopoFormulas != "" {
		cityLayers = append(cityLayers, cityTopoFormulas)
	}
	if cityLocalFormulas != "" {
		cityLayers = append(cityLayers, cityLocalFormulas)
	}
	fl.City = cityLayers

	// Per-rig layers: city layers + rig topology + rig local.
	for _, r := range rigs {
		layers := make([]string, len(cityLayers))
		copy(layers, cityLayers)
		if fd, ok := rigTopoFormulas[r.Name]; ok && fd != "" {
			layers = append(layers, fd)
		}
		if r.FormulasDir != "" {
			rigLocalDir := resolveConfigPath(r.FormulasDir, cityRoot, cityRoot)
			layers = append(layers, rigLocalDir)
		}
		if len(layers) > 0 {
			fl.Rigs[r.Name] = layers
		}
	}

	return fl
}

// loadTopology loads a topology.toml, validates metadata, and returns the
// agent list with dir stamped and paths adjusted, along with the resolved
// formula directory (empty if not configured).
func loadTopology(fs fsys.FS, topoPath, topoDir, cityRoot, rigName string) ([]Agent, map[string]ProviderSpec, string, []PeriodicFormula, error) {
	data, err := fs.ReadFile(topoPath)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("loading %s: %w", topologyFile, err)
	}

	var tc topologyConfig
	if _, err := toml.Decode(string(data), &tc); err != nil {
		return nil, nil, "", nil, fmt.Errorf("parsing %s: %w", topologyFile, err)
	}

	if err := validateTopologyMeta(&tc.Topology); err != nil {
		return nil, nil, "", nil, err
	}

	// Stamp agents: set dir = rigName (unless already set), adjust paths.
	agents := make([]Agent, len(tc.Agents))
	copy(agents, tc.Agents)
	for i := range agents {
		if agents[i].Dir == "" {
			agents[i].Dir = rigName
		}
		// Resolve prompt_template paths relative to topology directory.
		if agents[i].PromptTemplate != "" {
			agents[i].PromptTemplate = adjustFragmentPath(
				agents[i].PromptTemplate, topoDir, cityRoot)
		}
	}

	// Resolve formula directory relative to topology directory.
	var formulaDir string
	if tc.Formulas.Dir != "" {
		formulaDir = resolveConfigPath(tc.Formulas.Dir, topoDir, cityRoot)
	}

	return agents, tc.Providers, formulaDir, tc.Formulas.Periodic, nil
}

// validateTopologyMeta checks the [topology] header for required fields
// and schema compatibility.
func validateTopologyMeta(meta *TopologyMeta) error {
	if meta.Name == "" {
		return fmt.Errorf("[topology] name is required")
	}
	if meta.Schema == 0 {
		return fmt.Errorf("[topology] schema is required")
	}
	if meta.Schema > currentTopologySchema {
		return fmt.Errorf("[topology] schema %d not supported (max %d)", meta.Schema, currentTopologySchema)
	}
	return nil
}

// mergePeriodicFormulas merges new periodic formulas into existing ones.
// If a formula with the same name already exists, the existing one wins.
// This lets city-level config override topology defaults (e.g. changing
// interval from "24h" to "8h" by redeclaring the formula in city.toml).
func mergePeriodicFormulas(existing, incoming []PeriodicFormula) []PeriodicFormula {
	if len(incoming) == 0 {
		return existing
	}
	// Index existing formula names.
	seen := make(map[string]bool, len(existing))
	for _, pf := range existing {
		seen[pf.Formula] = true
	}
	for _, pf := range incoming {
		if !seen[pf.Formula] {
			existing = append(existing, pf)
			seen[pf.Formula] = true
		}
	}
	return existing
}

// applyOverrides applies per-rig overrides to topology-stamped agents.
// Each override targets an agent by name within the topology.
func applyOverrides(agents []Agent, overrides []AgentOverride, _ string) error {
	for i, ov := range overrides {
		if ov.Agent == "" {
			return fmt.Errorf("overrides[%d]: agent name is required", i)
		}
		found := false
		for j := range agents {
			if agents[j].Name == ov.Agent {
				applyAgentOverride(&agents[j], &ov)
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("overrides[%d]: agent %q not found in topology", i, ov.Agent)
		}
	}
	return nil
}

// applyAgentOverride applies a single override to an agent.
func applyAgentOverride(a *Agent, ov *AgentOverride) {
	if ov.Dir != nil {
		a.Dir = *ov.Dir
	}
	if ov.Suspended != nil {
		a.Suspended = *ov.Suspended
	}
	if ov.Isolation != nil {
		a.Isolation = *ov.Isolation
	}
	if ov.PromptTemplate != nil {
		a.PromptTemplate = *ov.PromptTemplate
	}
	if ov.Provider != nil {
		a.Provider = *ov.Provider
	}
	if ov.StartCommand != nil {
		a.StartCommand = *ov.StartCommand
	}
	if ov.Nudge != nil {
		a.Nudge = *ov.Nudge
	}
	if ov.IdleTimeout != nil {
		a.IdleTimeout = *ov.IdleTimeout
	}
	if len(ov.InstallAgentHooks) > 0 {
		a.InstallAgentHooks = append([]string(nil), ov.InstallAgentHooks...)
	}
	if ov.HooksInstalled != nil {
		a.HooksInstalled = ov.HooksInstalled
	}
	// Env: additive merge.
	if len(ov.Env) > 0 {
		if a.Env == nil {
			a.Env = make(map[string]string, len(ov.Env))
		}
		for k, v := range ov.Env {
			a.Env[k] = v
		}
	}
	for _, k := range ov.EnvRemove {
		delete(a.Env, k)
	}
	// Pool: sub-field patching.
	if ov.Pool != nil {
		applyPoolOverride(a, ov.Pool)
	}
}

// TopologyContentHash computes a SHA-256 hash of all files in a topology
// directory. The hash is deterministic (sorted filenames). Returns empty
// string if the directory cannot be read.
func TopologyContentHash(fs fsys.FS, topoDir string) string {
	entries, err := fs.ReadDir(topoDir)
	if err != nil {
		return ""
	}

	// Collect all file paths (non-recursive for now).
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		paths = append(paths, e.Name())
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, name := range paths {
		data, err := fs.ReadFile(filepath.Join(topoDir, name))
		if err != nil {
			continue
		}
		h.Write([]byte(name)) //nolint:errcheck // hash.Write never errors
		h.Write([]byte{0})    //nolint:errcheck // hash.Write never errors
		h.Write(data)         //nolint:errcheck // hash.Write never errors
		h.Write([]byte{0})    //nolint:errcheck // hash.Write never errors
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// TopologyContentHashRecursive computes a SHA-256 hash of all files in a
// topology directory, recursively descending into subdirectories. File
// paths are sorted for determinism and include the relative path from
// topoDir.
func TopologyContentHashRecursive(fs fsys.FS, topoDir string) string {
	var paths []string
	collectFiles(fs, topoDir, "", &paths)
	sort.Strings(paths)

	h := sha256.New()
	for _, relPath := range paths {
		data, err := fs.ReadFile(filepath.Join(topoDir, relPath))
		if err != nil {
			continue
		}
		h.Write([]byte(relPath)) //nolint:errcheck // hash.Write never errors
		h.Write([]byte{0})       //nolint:errcheck // hash.Write never errors
		h.Write(data)            //nolint:errcheck // hash.Write never errors
		h.Write([]byte{0})       //nolint:errcheck // hash.Write never errors
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// collectFiles recursively collects file paths relative to base.
func collectFiles(fs fsys.FS, base, prefix string, out *[]string) {
	dir := base
	if prefix != "" {
		dir = filepath.Join(base, prefix)
	}
	entries, err := fs.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		rel := e.Name()
		if prefix != "" {
			rel = prefix + "/" + e.Name()
		}
		if e.IsDir() {
			collectFiles(fs, base, rel, out)
		} else {
			*out = append(*out, rel)
		}
	}
}

// resolveNamedTopologies translates named topology references to cache paths.
// If a rig's Topology matches a key in cfg.Topologies, it is rewritten to
// the local cache directory path. Local path references pass through unchanged.
// Called after merge + patches, before ExpandTopologies.
func resolveNamedTopologies(cfg *City, cityRoot string) {
	if len(cfg.Topologies) == 0 {
		return
	}
	for i := range cfg.Rigs {
		if cfg.Rigs[i].Topology == "" {
			continue
		}
		if src, ok := cfg.Topologies[cfg.Rigs[i].Topology]; ok {
			cfg.Rigs[i].Topology = TopologyCachePath(cityRoot, cfg.Rigs[i].Topology, src)
		}
	}
}

// HasTopologyRigs reports whether any rig in the config uses a topology.
func HasTopologyRigs(rigs []Rig) bool {
	for _, r := range rigs {
		if r.Topology != "" {
			return true
		}
	}
	return false
}

// TopologySummary returns a string summarizing topology usage per rig
// (for provenance/config show output). Only includes rigs with topologies.
func TopologySummary(cfg *City, fs fsys.FS, cityRoot string) map[string]string {
	result := make(map[string]string)
	for _, r := range cfg.Rigs {
		if r.Topology == "" {
			continue
		}
		topoDir := resolveConfigPath(r.Topology, cityRoot, cityRoot)

		// Build summary: "topology-name vX.Y.Z (hash[:12])"
		topoPath := filepath.Join(topoDir, topologyFile)
		data, err := fs.ReadFile(topoPath)
		if err != nil {
			result[r.Name] = r.Topology + " (unreadable)"
			continue
		}
		var tc topologyConfig
		if _, err := toml.Decode(string(data), &tc); err != nil {
			result[r.Name] = r.Topology + " (parse error)"
			continue
		}
		hash := TopologyContentHashRecursive(fs, topoDir)
		short := hash
		if len(short) > 12 {
			short = short[:12]
		}
		var parts []string
		parts = append(parts, tc.Topology.Name)
		if tc.Topology.Version != "" {
			parts = append(parts, tc.Topology.Version)
		}
		parts = append(parts, "("+short+")")
		result[r.Name] = strings.Join(parts, " ")
	}
	return result
}
