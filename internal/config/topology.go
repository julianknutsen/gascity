package config

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"slices"
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
// with topology fields set, it loads the topology directories, stamps agents
// with dir = rig.Name, resolves prompt_template paths relative to the
// topology directory, and appends the agents to the city config.
//
// Overrides from the rig are applied to the stamped agents (after all
// topologies for the rig are expanded). All expansion happens before
// validation — downstream sees a flat City struct.
//
// rigFormulaDirs is populated with per-rig topology formula directories
// (Layer 3). cityRoot is the city directory (parent of city.toml), used
// for path resolution.
func ExpandTopologies(cfg *City, fs fsys.FS, cityRoot string, rigFormulaDirs map[string][]string) error {
	var expanded []Agent
	for i := range cfg.Rigs {
		rig := &cfg.Rigs[i]
		topoRefs := EffectiveRigTopologies(*rig)
		if len(topoRefs) == 0 {
			continue
		}

		var rigAgents []Agent
		for _, ref := range topoRefs {
			topoDir := resolveConfigPath(ref, cityRoot, cityRoot)
			topoPath := filepath.Join(topoDir, topologyFile)

			agents, providers, formulaDirs, sharedDirs, cityAgents, err := loadTopology(fs, topoPath, topoDir, cityRoot, rig.Name, nil)
			if err != nil {
				return fmt.Errorf("rig %q topology %q: %w", rig.Name, ref, err)
			}

			// Accumulate shared dirs (deduped later).
			cfg.TopologySharedDirs = appendUnique(cfg.TopologySharedDirs, sharedDirs...)

			// If city_agents is set, exclude city-scoped agents from rig expansion.
			if len(cityAgents) > 0 {
				citySet := setFromSlice(cityAgents)
				agents = filterAgentsByName(agents, citySet, false)
			}

			// Record rig topology formula dirs (Layer 3).
			if rigFormulaDirs != nil {
				rigFormulaDirs[rig.Name] = append(rigFormulaDirs[rig.Name], formulaDirs...)
			}

			rigAgents = append(rigAgents, agents...)

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
		}

		// Check for duplicate agent names across topologies for this rig.
		if err := checkTopologyAgentCollisions(rigAgents, rig.Name); err != nil {
			return err
		}

		// Apply per-rig overrides after all topologies for this rig.
		if err := applyOverrides(rigAgents, rig.Overrides, rig.Name); err != nil {
			return fmt.Errorf("rig %q: %w", rig.Name, err)
		}

		expanded = append(expanded, rigAgents...)
	}
	cfg.Agents = append(cfg.Agents, expanded...)
	return nil
}

// ExpandCityTopology loads the city-level topology from workspace.topology.
// City topology agents are stamped with dir="" (city-scoped) and prepended
// to the agent list. Returns the resolved formula dir from the topology
// (empty if none). cityRoot is the city directory.
//
// Deprecated: Use ExpandCityTopologies for composable multi-topology support.
func ExpandCityTopology(cfg *City, fs fsys.FS, cityRoot string) (string, error) {
	dirs, err := ExpandCityTopologies(cfg, fs, cityRoot)
	if err != nil {
		return "", err
	}
	if len(dirs) == 0 {
		return "", nil
	}
	return dirs[0], nil
}

// ExpandCityTopologies loads all city-level topologies (from both
// workspace.topology and workspace.topologies). City topology agents are
// stamped with dir="" (city-scoped) and prepended to the agent list.
// Returns the resolved formula dirs (one per topology that has formulas).
// cityRoot is the city directory.
func ExpandCityTopologies(cfg *City, fs fsys.FS, cityRoot string) ([]string, error) {
	topos := EffectiveCityTopologies(cfg.Workspace)
	if len(topos) == 0 {
		return nil, nil
	}

	var allAgents []Agent
	var formulaDirs []string

	for _, ref := range topos {
		topoDir := resolveConfigPath(ref, cityRoot, cityRoot)
		topoPath := filepath.Join(topoDir, topologyFile)

		agents, providers, topoDirs, sharedDirs, cityAgents, err := loadTopology(fs, topoPath, topoDir, cityRoot, "", nil)
		if err != nil {
			return nil, fmt.Errorf("city topology %q: %w", ref, err)
		}

		// Accumulate shared dirs (deduped).
		cfg.TopologySharedDirs = appendUnique(cfg.TopologySharedDirs, sharedDirs...)

		// If city_agents is set, keep only city-scoped agents.
		if len(cityAgents) > 0 {
			citySet := setFromSlice(cityAgents)
			agents = filterAgentsByName(agents, citySet, true)
		}

		allAgents = append(allAgents, agents...)
		formulaDirs = append(formulaDirs, topoDirs...)

		// Merge topology providers (additive, first wins).
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
	}

	// Check for duplicate agent names across city topologies.
	if err := checkTopologyAgentCollisions(allAgents, ""); err != nil {
		return nil, err
	}

	// City topology agents go at the front (before user-defined agents).
	cfg.Agents = append(allAgents, cfg.Agents...)

	return formulaDirs, nil
}

// ComputeFormulaLayers builds the FormulaLayers from the resolved formula
// directories. Each layer slice is ordered lowest→highest priority.
//
// Parameters:
//   - cityTopoFormulas: formula dirs from city topologies (Layer 1), nil if none
//   - cityLocalFormulas: formula dir from city [formulas] section (Layer 2), "" if none
//   - rigTopoFormulas: map[rigName][]formulaDirs from rig topologies (Layer 3)
//   - rigs: rig configs (for rig-local FormulasDir, Layer 4)
//   - cityRoot: city directory for resolving relative paths
func ComputeFormulaLayers(cityTopoFormulas []string, cityLocalFormulas string, rigTopoFormulas map[string][]string, rigs []Rig, cityRoot string) FormulaLayers {
	fl := FormulaLayers{
		Rigs: make(map[string][]string),
	}

	// City layers (apply to city-scoped agents and as base for all rigs).
	var cityLayers []string
	cityLayers = append(cityLayers, cityTopoFormulas...)
	if cityLocalFormulas != "" {
		cityLayers = append(cityLayers, cityLocalFormulas)
	}
	fl.City = cityLayers

	// Per-rig layers: city layers + rig topology + rig local.
	for _, r := range rigs {
		layers := make([]string, len(cityLayers))
		copy(layers, cityLayers)
		if fds, ok := rigTopoFormulas[r.Name]; ok {
			layers = append(layers, fds...)
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

// checkTopologyAgentCollisions detects duplicate agent names within
// topology-expanded agents and returns an error with provenance (which
// topology directories defined the conflicting agents). rigName is used
// for the error message context; pass "" for city-scoped agents.
func checkTopologyAgentCollisions(agents []Agent, rigName string) error {
	// Map agent name → list of source directories that defined it.
	sources := make(map[string][]string)
	for _, a := range agents {
		src := a.SourceDir
		if src == "" {
			continue // inline agents have no SourceDir
		}
		existing := sources[a.Name]
		if !slices.Contains(existing, src) {
			sources[a.Name] = append(existing, src)
		}
	}
	for name, dirs := range sources {
		if len(dirs) < 2 {
			continue
		}
		scope := "city"
		if rigName != "" {
			scope = fmt.Sprintf("rig %q", rigName)
		}
		return fmt.Errorf("%s: topologies define duplicate agent %q:\n  - %s\nrename one agent in its topology.toml, or use separate rigs",
			scope, name, strings.Join(dirs, "\n  - "))
	}
	return nil
}

// loadTopology loads a topology.toml, validates metadata, and returns the
// agent list with dir stamped and paths adjusted, the resolved formula
// directories, shared template directories, and the city_agents list (nil
// if not configured).
//
// The seen set tracks visited topology directories for cycle detection.
// Pass nil for the initial call; it will be initialized automatically.
// Includes are processed recursively: included agents come first (base
// layer), then the parent's own agents (override layer).
func loadTopology(fs fsys.FS, topoPath, topoDir, cityRoot, rigName string, seen map[string]bool) ([]Agent, map[string]ProviderSpec, []string, []string, []string, error) {
	// Initialize seen set on first call.
	if seen == nil {
		seen = make(map[string]bool)
	}

	// Cycle detection: resolve to absolute path for reliable comparison.
	absTopoDir, err := filepath.Abs(topoDir)
	if err != nil {
		absTopoDir = topoDir
	}
	if seen[absTopoDir] {
		return nil, nil, nil, nil, nil, fmt.Errorf("cycle detected: topology %q already visited", topoDir)
	}
	seen[absTopoDir] = true

	data, err := fs.ReadFile(topoPath)
	if err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("loading %s: %w", topologyFile, err)
	}

	var tc topologyConfig
	if _, err := toml.Decode(string(data), &tc); err != nil {
		return nil, nil, nil, nil, nil, fmt.Errorf("parsing %s: %w", topologyFile, err)
	}

	if err := validateTopologyMeta(&tc.Topology); err != nil {
		return nil, nil, nil, nil, nil, err
	}

	// Process includes: accumulate base-layer agents, providers, formula
	// dirs, and city_agents from included topologies.
	var includedAgents []Agent
	var includedFormulaDirs []string
	var includedSharedDirs []string
	includedProviders := make(map[string]ProviderSpec)
	cityAgentsSet := make(map[string]bool)

	for _, inc := range tc.Topology.Includes {
		var incTopoDir string
		if isRemoteInclude(inc) {
			source, subpath, ref := parseRemoteInclude(inc)
			cacheDir, err := fetchRemoteInclude(source, ref, cityRoot)
			if err != nil {
				return nil, nil, nil, nil, nil, fmt.Errorf("include %q: %w", inc, err)
			}
			incTopoDir = cacheDir
			if subpath != "" {
				incTopoDir = filepath.Join(cacheDir, subpath)
			}
		} else {
			incTopoDir = resolveConfigPath(inc, topoDir, cityRoot)
		}

		incTopoPath := filepath.Join(incTopoDir, topologyFile)
		incAgents, incProviders, incFormulaDirs, incSharedDirs, incCityAgents, err := loadTopology(
			fs, incTopoPath, incTopoDir, cityRoot, rigName, seen)
		if err != nil {
			return nil, nil, nil, nil, nil, fmt.Errorf("include %q: %w", inc, err)
		}

		includedAgents = append(includedAgents, incAgents...)
		includedFormulaDirs = append(includedFormulaDirs, incFormulaDirs...)
		includedSharedDirs = append(includedSharedDirs, incSharedDirs...)

		// Merge providers: included first, no overwrite.
		for name, spec := range incProviders {
			if _, exists := includedProviders[name]; !exists {
				includedProviders[name] = spec
			}
		}

		// Union city_agents.
		for _, ca := range incCityAgents {
			cityAgentsSet[ca] = true
		}
	}

	// Validate city_agents against all agents (included + parent).
	allAgentNames := make(map[string]bool, len(includedAgents)+len(tc.Agents))
	for _, a := range includedAgents {
		allAgentNames[a.Name] = true
	}
	for _, a := range tc.Agents {
		allAgentNames[a.Name] = true
	}

	// Build city_agents union: included first (sorted for determinism),
	// then parent in declaration order, deduplicated.
	var cityAgents []string
	parentCityAgents := make(map[string]bool, len(tc.Topology.CityAgents))
	for _, ca := range tc.Topology.CityAgents {
		cityAgentsSet[ca] = true
		parentCityAgents[ca] = true
	}
	// Collect included-only entries (sorted for determinism).
	for ca := range cityAgentsSet {
		if !parentCityAgents[ca] {
			cityAgents = append(cityAgents, ca)
		}
	}
	sort.Strings(cityAgents)
	// Append parent entries in declaration order, skipping duplicates.
	deduped := make(map[string]bool, len(tc.Topology.CityAgents))
	for _, ca := range tc.Topology.CityAgents {
		if !deduped[ca] {
			deduped[ca] = true
			cityAgents = append(cityAgents, ca)
		}
	}

	// Validate all city_agents against the full agent set.
	for _, ca := range cityAgents {
		if !allAgentNames[ca] {
			return nil, nil, nil, nil, nil, fmt.Errorf("city_agents: agent %q not found in topology", ca)
		}
	}

	// Stamp parent agents: set dir = rigName (unless already set), adjust paths.
	agents := make([]Agent, len(tc.Agents))
	copy(agents, tc.Agents)
	for i := range agents {
		if agents[i].Dir == "" {
			agents[i].Dir = rigName
		}
		// Track where this agent's config was defined.
		agents[i].SourceDir = topoDir
		// Resolve prompt_template paths relative to topology directory.
		if agents[i].PromptTemplate != "" {
			agents[i].PromptTemplate = adjustFragmentPath(
				agents[i].PromptTemplate, topoDir, cityRoot)
		}
		// Resolve session_setup_script paths relative to topology directory.
		if agents[i].SessionSetupScript != "" {
			agents[i].SessionSetupScript = adjustFragmentPath(
				agents[i].SessionSetupScript, topoDir, cityRoot)
		}
		// Resolve overlay_dir paths relative to topology directory.
		if agents[i].OverlayDir != "" {
			agents[i].OverlayDir = adjustFragmentPath(
				agents[i].OverlayDir, topoDir, cityRoot)
		}
	}

	// Merge: included agents first (base), then parent agents (override).
	includedAgents = append(includedAgents, agents...)

	// Merge providers: parent wins over included.
	mergedProviders := includedProviders
	for name, spec := range tc.Providers {
		mergedProviders[name] = spec
	}

	// Resolve parent formula directory relative to topology directory.
	var formulaDirs []string
	formulaDirs = append(formulaDirs, includedFormulaDirs...)
	if tc.Formulas.Dir != "" {
		formulaDirs = append(formulaDirs, resolveConfigPath(tc.Formulas.Dir, topoDir, cityRoot))
	}

	// Collect prompts/shared/ directory if it exists.
	var sharedDirs []string
	sharedDirs = append(sharedDirs, includedSharedDirs...)
	sharedDir := filepath.Join(topoDir, "prompts", "shared")
	if _, err := fs.Stat(sharedDir); err == nil {
		sharedDirs = append(sharedDirs, sharedDir)
	}

	return includedAgents, mergedProviders, formulaDirs, sharedDirs, cityAgents, nil
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

// appendUnique appends items to dst, skipping any already present.
func appendUnique(dst []string, items ...string) []string {
	seen := setFromSlice(dst)
	for _, item := range items {
		if !seen[item] {
			dst = append(dst, item)
			seen[item] = true
		}
	}
	return dst
}

// setFromSlice builds a set from a string slice.
func setFromSlice(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}

// filterAgentsByName filters agents by name membership in the set.
// If keep is true, only agents whose Name is in the set are kept.
// If keep is false, agents whose Name is in the set are excluded.
func filterAgentsByName(agents []Agent, nameSet map[string]bool, keep bool) []Agent {
	var result []Agent
	for _, a := range agents {
		if nameSet[a.Name] == keep {
			result = append(result, a)
		}
	}
	return result
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
	if len(ov.PreStart) > 0 {
		a.PreStart = append([]string(nil), ov.PreStart...)
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
	if len(ov.SessionSetup) > 0 {
		a.SessionSetup = append([]string(nil), ov.SessionSetup...)
	}
	if ov.SessionSetupScript != nil {
		a.SessionSetupScript = *ov.SessionSetupScript
	}
	if ov.OverlayDir != nil {
		a.OverlayDir = *ov.OverlayDir
	}
	if ov.DefaultSlingFormula != nil {
		a.DefaultSlingFormula = *ov.DefaultSlingFormula
	}
	if len(ov.InjectFragments) > 0 {
		a.InjectFragments = append([]string(nil), ov.InjectFragments...)
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
// Handles all four topology fields: workspace.topology, workspace.topologies,
// rig.topology, and rig.topologies. If a reference matches a key in
// cfg.Topologies, it is rewritten to the local cache directory path.
// Local path references pass through unchanged.
// Called after merge + patches, before expansion.
func resolveNamedTopologies(cfg *City, cityRoot string) {
	if len(cfg.Topologies) == 0 {
		return
	}
	// City singular.
	if cfg.Workspace.Topology != "" {
		if src, ok := cfg.Topologies[cfg.Workspace.Topology]; ok {
			cfg.Workspace.Topology = TopologyCachePath(cityRoot, cfg.Workspace.Topology, src)
		}
	}
	// City plural.
	for i, ref := range cfg.Workspace.CityTopologies {
		if src, ok := cfg.Topologies[ref]; ok {
			cfg.Workspace.CityTopologies[i] = TopologyCachePath(cityRoot, ref, src)
		}
	}
	// Rig singular + plural.
	for i := range cfg.Rigs {
		if cfg.Rigs[i].Topology != "" {
			if src, ok := cfg.Topologies[cfg.Rigs[i].Topology]; ok {
				cfg.Rigs[i].Topology = TopologyCachePath(cityRoot, cfg.Rigs[i].Topology, src)
			}
		}
		for j, ref := range cfg.Rigs[i].RigTopologies {
			if src, ok := cfg.Topologies[ref]; ok {
				cfg.Rigs[i].RigTopologies[j] = TopologyCachePath(cityRoot, ref, src)
			}
		}
	}
}

// EffectiveCityTopologies returns the resolved list of city-level topology
// paths. If both singular Topology and plural CityTopologies are set,
// the singular is prepended. Returns nil if neither is set.
func EffectiveCityTopologies(ws Workspace) []string {
	var result []string
	if ws.Topology != "" {
		result = append(result, ws.Topology)
	}
	result = append(result, ws.CityTopologies...)
	return result
}

// EffectiveRigTopologies returns the resolved list of topology paths for
// a rig. If both singular Topology and plural RigTopologies are set,
// the singular is prepended. Returns nil if neither is set.
func EffectiveRigTopologies(rig Rig) []string {
	var result []string
	if rig.Topology != "" {
		result = append(result, rig.Topology)
	}
	result = append(result, rig.RigTopologies...)
	return result
}

// HasTopologyRigs reports whether any rig in the config uses a topology.
func HasTopologyRigs(rigs []Rig) bool {
	for _, r := range rigs {
		if r.Topology != "" || len(r.RigTopologies) > 0 {
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
		topoRefs := EffectiveRigTopologies(r)
		if len(topoRefs) == 0 {
			continue
		}
		var summaries []string
		for _, ref := range topoRefs {
			summaries = append(summaries, topologySummaryOne(fs, ref, cityRoot))
		}
		result[r.Name] = strings.Join(summaries, "; ")
	}
	return result
}

// topologySummaryOne builds a summary string for a single topology reference.
func topologySummaryOne(fs fsys.FS, ref, cityRoot string) string {
	topoDir := resolveConfigPath(ref, cityRoot, cityRoot)
	topoPath := filepath.Join(topoDir, topologyFile)
	data, err := fs.ReadFile(topoPath)
	if err != nil {
		return ref + " (unreadable)"
	}
	var tc topologyConfig
	if _, err := toml.Decode(string(data), &tc); err != nil {
		return ref + " (parse error)"
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
	return strings.Join(parts, " ")
}
