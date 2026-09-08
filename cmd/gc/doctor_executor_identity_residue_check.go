package main

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gastownhall/gascity/internal/agent"
	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/doctor"
	"github.com/gastownhall/gascity/internal/fsys"
	"github.com/gastownhall/gascity/internal/graphroute"
	"github.com/gastownhall/gascity/internal/suspensionstate"
)

// executorIdentityResidueCheck is a gc doctor check that finds and clears
// stale executor-identity stamp residue left on beads: gc.session_name,
// gc.work_dir, and the legacy work_dir metadata keys can survive after a
// bead moves on from the executor that stamped them (design ref
// ga-cm2o5t.1, PR #6099).
type executorIdentityResidueCheck struct {
	cfg      *config.City
	cityPath string
	newStore func(string) (beads.Store, error)
}

func newExecutorIdentityResidueCheck(cfg *config.City, cityPath string, newStore func(string) (beads.Store, error)) *executorIdentityResidueCheck {
	return &executorIdentityResidueCheck{cfg: cfg, cityPath: cityPath, newStore: newStore}
}

func (c *executorIdentityResidueCheck) Name() string { return "executor-identity-residue" }

func (c *executorIdentityResidueCheck) CanFix() bool { return true }

type executorIdentityResidueFinding struct {
	label  string
	store  beads.Store
	beadID string
}

func (f executorIdentityResidueFinding) describe() string {
	return fmt.Sprintf("%s bead %s carries stale executor-identity stamp residue (%s)", f.label, f.beadID, beadmeta.SessionNameMetadataKey)
}

func (c *executorIdentityResidueCheck) Run(_ *doctor.CheckContext) *doctor.CheckResult {
	findings, skipped := c.collect()
	if len(findings) == 0 && len(skipped) == 0 {
		return okCheck(c.Name(), "no stale executor-identity stamp residue found")
	}
	details := make([]string, 0, len(findings)+len(skipped))
	for _, f := range findings {
		details = append(details, f.describe())
	}
	details = append(details, skipped...)
	sort.Strings(details)
	if len(findings) == 0 {
		return warnCheck(c.Name(),
			fmt.Sprintf("executor-identity-residue check skipped %d scope(s)", len(skipped)),
			"fix bead store access, then rerun gc doctor",
			details)
	}
	if len(skipped) > 0 {
		return warnCheck(c.Name(),
			fmt.Sprintf("%d bead(s) carry stale executor-identity stamp residue; %d scope(s) skipped", len(findings), len(skipped)),
			"run gc doctor --fix to clear the stale stamps, fix skipped store access, then rerun gc doctor",
			details)
	}
	return warnCheck(c.Name(),
		fmt.Sprintf("%d bead(s) carry stale executor-identity stamp residue", len(findings)),
		"run gc doctor --fix to clear the stale gc.session_name/gc.work_dir/work_dir stamps, then rerun gc doctor",
		details)
}

func (c *executorIdentityResidueCheck) Fix(_ *doctor.CheckContext) error {
	findings, skipped := c.collect()
	var errs []error
	for _, f := range findings {
		clearKVs := map[string]string{
			beadmeta.SessionNameMetadataKey:   "",
			beadmeta.WorkDirMetadataKey:       "",
			beadmeta.LegacyWorkDirMetadataKey: "",
		}
		if err := f.store.SetMetadataBatch(f.beadID, clearKVs); err != nil {
			errs = append(errs, fmt.Errorf("%s bead %s: clear executor-identity stamp: %w", f.label, f.beadID, err))
		}
	}
	if len(skipped) > 0 {
		errs = append(errs, fmt.Errorf("executor-identity-residue skipped %d scope(s): %s", len(skipped), strings.Join(skipped, "; ")))
	}
	return errors.Join(errs...)
}

func (c *executorIdentityResidueCheck) collect() (findings []executorIdentityResidueFinding, skipped []string) {
	scopes := []struct{ label, path string }{{"city", c.cityPath}}
	if c.cfg != nil {
		suspState, _ := loadSuspensionState(fsys.OSFS{}, c.cityPath)
		for _, rig := range c.cfg.Rigs {
			if suspensionstate.EffectiveRigSuspended(suspState, rig.Name, rig.EffectiveSuspendedOnStart()) || strings.TrimSpace(rig.Path) == "" {
				continue
			}
			scopes = append(scopes, struct{ label, path string }{"rig " + rig.Name, rig.Path})
		}
	}
	for _, sc := range scopes {
		if c.newStore == nil || strings.TrimSpace(sc.path) == "" {
			continue
		}
		store, err := c.newStore(sc.path)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s skipped: opening bead store: %v", sc.label, err))
			continue
		}
		scopeFindings, err := c.collectStoreFindings(store, sc.label)
		findings = append(findings, scopeFindings...)
		if err != nil {
			skipped = append(skipped, fmt.Sprintf("%s skipped: listing beads: %v", sc.label, err))
		}
	}
	return findings, skipped
}

func (c *executorIdentityResidueCheck) collectStoreFindings(store beads.Store, label string) ([]executorIdentityResidueFinding, error) {
	items, err := store.List(beads.ListQuery{AllowScan: true, IncludeClosed: true})
	if err != nil {
		return nil, err
	}
	var findings []executorIdentityResidueFinding
	for _, bd := range items {
		if !isExecutorIdentityStampStale(c.cfg, bd) {
			continue
		}
		findings = append(findings, executorIdentityResidueFinding{label: label, store: store, beadID: bd.ID})
	}
	return findings, nil
}

// isExecutorIdentityStampStale reports whether bd carries executor-identity
// stamp residue. It is not in_progress, not workflow topology (a run root,
// scope latch, or formula spec is never itself claimed — only its descendant
// steps are — so a completed step's visibility stamp copied onto the root
// must not read as residue even when it no longer matches the root's own
// gc.routed_to), gc.session_name is non-empty, and the session name the
// bead's CURRENT gc.routed_to would mint today — via agent.SessionNameFor,
// honoring any configured session_template — no longer matches the stored
// gc.session_name. The comparison forward-encodes from gc.routed_to on every
// call, never against a fixed snapshot and never via a best-effort reverse
// decode, so a bead whose stamp still matches what its current route would
// mint — including through a custom session_template — is not flagged.
func isExecutorIdentityStampStale(cfg *config.City, bd beads.Bead) bool {
	if bd.Status == "in_progress" {
		return false
	}
	if graphroute.IsWorkflowTopologyKind(bd.Metadata[beadmeta.KindMetadataKey]) {
		return false
	}
	sessionName := strings.TrimSpace(bd.Metadata[beadmeta.SessionNameMetadataKey])
	if sessionName == "" {
		return false
	}
	routedTo := strings.TrimSpace(bd.Metadata[beadmeta.RoutedToMetadataKey])
	var sessionTemplate string
	if cfg != nil {
		sessionTemplate = cfg.Workspace.SessionTemplate
	}
	expected := agent.SessionNameFor(cfg.EffectiveCityName(), routedTo, sessionTemplate)
	return expected != sessionName
}
