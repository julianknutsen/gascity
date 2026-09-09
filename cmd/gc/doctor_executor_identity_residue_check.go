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
// ga-cm2o5t.1, PR #6099; amended scope ga-6af29d decision 1).
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
	// keys is the set of metadata keys the triggering condition(s) actually
	// named. Fix() clears exactly these keys -- never a fixed superset --
	// so a trigger that did not fire can never lose state it never
	// inspected.
	keys []string
}

func (f executorIdentityResidueFinding) describe() string {
	return fmt.Sprintf("%s bead %s carries stale executor-identity stamp residue (%s)", f.label, f.beadID, strings.Join(f.keys, ", "))
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
		if len(f.keys) == 0 {
			continue
		}
		clearKVs := make(map[string]string, len(f.keys))
		for _, key := range f.keys {
			clearKVs[key] = ""
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
	items, err := store.List(beads.ListQuery{Status: "open", AllowScan: true, Live: true})
	if err != nil {
		return nil, err
	}
	routeIdentities := buildExecutorRouteIdentityIndex(items)
	var findings []executorIdentityResidueFinding
	for _, bd := range items {
		keys := staleExecutorIdentityStampKeys(c.cfg, bd, routeIdentities)
		if len(keys) == 0 {
			continue
		}
		findings = append(findings, executorIdentityResidueFinding{label: label, store: store, beadID: bd.ID, keys: keys})
	}
	return findings, nil
}

// executorRouteIdentityIndex maps a route to the full set of executor
// identities that legitimately act for it. The base session name a plain
// SessionNameFor(route) encoding would produce is only one member of that
// set when the route runs a pool — each pool slot mints its own concrete
// session name (sessionBeadIdentifier semantics) while still belonging to
// the same route (retiredSessionFallbackRoute semantics). Built fresh from
// the same open, live-scanned beads on every check run, never persisted, so
// it always reflects the pool's current membership.
type executorRouteIdentityIndex map[string]map[string]struct{}

// buildExecutorRouteIdentityIndex indexes items's open session beads by
// route (retiredSessionFallbackRoute) and identity (sessionBeadIdentifier),
// the inverse direction of pool_detached_orphan_sweep.go's
// detachedOrphanRouteIndex (session_name -> route, single-valued).
func buildExecutorRouteIdentityIndex(items []beads.Bead) executorRouteIdentityIndex {
	idx := make(executorRouteIdentityIndex)
	for _, b := range items {
		if b.Type != sessionBeadType {
			continue
		}
		route := strings.TrimSpace(retiredSessionFallbackRoute(b))
		identity := strings.TrimSpace(sessionBeadIdentifier(b))
		if route == "" || identity == "" {
			continue
		}
		if idx[route] == nil {
			idx[route] = make(map[string]struct{})
		}
		idx[route][identity] = struct{}{}
	}
	return idx
}

func (idx executorRouteIdentityIndex) legitimate(route, identity string) bool {
	if route == "" || identity == "" {
		return false
	}
	_, ok := idx[route][identity]
	return ok
}

// staleExecutorIdentityStampKeys reports which metadata keys on bd carry
// stale executor-identity stamp residue, or nil if none. Closed and
// in_progress beads are always out of scope, as is workflow topology (a run
// root, scope latch, or formula spec is never itself claimed — only its
// descendant steps are — so a completed step's visibility stamp copied onto
// the root must not read as residue even when it no longer matches the
// root's own gc.routed_to).
//
// Two independent triggers follow, each contributing only the key(s) it
// actually fired on to the result — see staleWorkDirStamp and
// staleSessionNameStamp for the trigger conditions and stand-downs. Keeping
// the two disjoint is load-bearing: Fix() clears exactly the returned keys,
// so a trigger that did not fire can never lose state it never inspected.
func staleExecutorIdentityStampKeys(cfg *config.City, bd beads.Bead, routeIdentities executorRouteIdentityIndex) []string {
	if bd.Status == "closed" {
		return nil
	}
	if bd.Status == "in_progress" {
		return nil
	}
	if graphroute.IsWorkflowTopologyKind(bd.Metadata[beadmeta.KindMetadataKey]) {
		return nil
	}

	var keys []string
	if staleWorkDirStamp(cfg, bd) {
		keys = append(keys, beadmeta.WorkDirMetadataKey, beadmeta.LegacyWorkDirMetadataKey)
	}
	if staleSessionNameStamp(cfg, bd, routeIdentities) {
		keys = append(keys, beadmeta.SessionNameMetadataKey)
	}
	return keys
}

// staleWorkDirStamp reports whether bd's legacy work_dir disagrees with its
// canonical gc.work_dir in a way that is genuine residue, rather than a
// repair candidate or an actively worktree-owning bead.
//
// Two stand-downs guard against clearing state a downstream fail-closed
// check relies on:
//
//   - hasWorktreeOwnershipEvidence: worktreeSpecForBead treats a bead
//     carrying worktree ownership metadata as actively managed and fails
//     closed on a canonical/legacy disagreement for it. Clearing both keys
//     here would erase the very evidence that check inspects, silently
//     downgrading its fail-closed conflict error into a "no spec, unmanaged"
//     no-op — the ga-6af29d/#6135 round-2 regression this stand-down closes.
//   - poolSlotWorkDirRepairFor(cfg, bd) != nil: this shape (canonical
//     clobbered with a pool-slot label, legacy still holding real per-bead
//     evidence) is a repair candidate the reconciler's own one-shot sweep
//     restores from legacy. Flagging it here races that repair for the same
//     keys and, if this check's Fix() wins, blanks both instead of
//     restoring the canonical.
func staleWorkDirStamp(cfg *config.City, bd beads.Bead) bool {
	workDir := strings.TrimSpace(bd.Metadata[beadmeta.WorkDirMetadataKey])
	legacyWorkDir := strings.TrimSpace(bd.Metadata[beadmeta.LegacyWorkDirMetadataKey])
	if workDir == "" || legacyWorkDir == "" || workDir == legacyWorkDir {
		return false
	}
	if hasWorktreeOwnershipEvidence(bd) {
		return false
	}
	if poolSlotWorkDirRepairFor(cfg, bd) != nil {
		return false
	}
	return true
}

// hasWorktreeOwnershipEvidence reports whether bd publishes any of the
// worktree-ownership metadata keys worktreeSpecForBead inspects to tell a
// managed per-bead worktree apart from an unmanaged spawn.
func hasWorktreeOwnershipEvidence(bd beads.Bead) bool {
	for _, key := range []string{
		beadmeta.WorktreeRootMetadataKey,
		beadmeta.WorktreeRepoMetadataKey,
		beadmeta.WorktreeOwnerMetadataKey,
	} {
		if strings.TrimSpace(bd.Metadata[key]) != "" {
			return true
		}
	}
	return false
}

// staleSessionNameStamp reports whether bd carries a stale gc.session_name:
// a non-empty stamp on a bead with a non-empty gc.routed_to (an empty route
// is the ordinary post-claim/detached-orphan state, not residue) whose
// stamped session name is neither what the bead's CURRENT gc.routed_to would
// mint today — via agent.SessionNameFor, honoring any configured
// session_template, forward-encoded on every call, never against a fixed
// snapshot — nor a member of that route's legitimate executor-identity set
// (routeIdentities; a pool slot's own concrete session name is a legitimate
// stamp against its base route).
func staleSessionNameStamp(cfg *config.City, bd beads.Bead, routeIdentities executorRouteIdentityIndex) bool {
	sessionName := strings.TrimSpace(bd.Metadata[beadmeta.SessionNameMetadataKey])
	if sessionName == "" {
		return false
	}
	routedTo := strings.TrimSpace(bd.Metadata[beadmeta.RoutedToMetadataKey])
	if routedTo == "" {
		return false
	}
	var sessionTemplate string
	var cityName string
	if cfg != nil {
		sessionTemplate = cfg.Workspace.SessionTemplate
		cityName = cfg.EffectiveCityName()
	}
	expected := agent.SessionNameFor(cityName, routedTo, sessionTemplate)
	if expected == sessionName {
		return false
	}
	return !routeIdentities.legitimate(routedTo, sessionName)
}
