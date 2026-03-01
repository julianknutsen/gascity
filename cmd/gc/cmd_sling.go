package main

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
	"github.com/steveyegge/gascity/internal/telemetry"
)

// BeadQuerier can retrieve a single bead by ID.
type BeadQuerier interface {
	Get(id string) (beads.Bead, error)
}

// BeadChildQuerier extends BeadQuerier with the ability to list children
// of a container bead (convoy, epic).
type BeadChildQuerier interface {
	BeadQuerier
	Children(parentID string) ([]beads.Bead, error)
}

func newSlingCmd(stdout, stderr io.Writer) *cobra.Command {
	var formula bool
	var nudge bool
	var force bool
	var title string
	var vars []string
	var merge string
	var noConvoy bool
	var owned bool
	var onFormula string
	var dryRun bool
	var noFormula bool
	cmd := &cobra.Command{
		Use:   "sling [target] <bead-or-formula>",
		Short: "Route work to an agent or pool",
		Long: `Route a bead to an agent or pool using the target's sling_query.

The target is an agent qualified name (e.g. "mayor" or "hello-world/polecat").
The second argument is a bead ID, or a formula name when --formula is set.

When target is omitted, the bead's rig prefix is used to look up the rig's
default_sling_target from config. Requires --formula to have an explicit target.

With --formula, a wisp (ephemeral molecule) is instantiated from the formula
and its root bead is routed to the target.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) < 1 || len(args) > 2 {
				fmt.Fprintf(stderr, "gc sling: requires 1 or 2 arguments: [target] <bead-or-formula>\n") //nolint:errcheck // best-effort stderr
				return errExit
			}
			if owned && noConvoy {
				fmt.Fprintf(stderr, "gc sling: --owned requires a convoy (cannot use with --no-convoy)\n") //nolint:errcheck // best-effort stderr
				return errExit
			}
			if merge != "" && merge != "direct" && merge != "mr" && merge != "local" {
				fmt.Fprintf(stderr, "gc sling: --merge must be direct, mr, or local\n") //nolint:errcheck // best-effort stderr
				return errExit
			}
			code := cmdSling(args, formula, nudge, force, title, vars, merge, noConvoy, owned, onFormula, noFormula, dryRun, stdout, stderr)
			if code != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&formula, "formula", "f", false, "treat argument as formula name")
	cmd.Flags().BoolVar(&nudge, "nudge", false, "nudge target after routing")
	cmd.Flags().BoolVar(&force, "force", false, "suppress warnings and allow cross-rig routing")
	cmd.Flags().StringVarP(&title, "title", "t", "", "wisp root bead title (with --formula or --on)")
	cmd.Flags().StringArrayVar(&vars, "var", nil, "variable substitution for formula (key=value, repeatable)")
	cmd.Flags().StringVar(&merge, "merge", "", "merge strategy: direct, mr, or local")
	cmd.Flags().BoolVar(&noConvoy, "no-convoy", false, "skip auto-convoy creation")
	cmd.Flags().BoolVar(&owned, "owned", false, "mark auto-convoy as owned (skip auto-close)")
	cmd.Flags().StringVar(&onFormula, "on", "", "attach wisp from formula to bead before routing")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would be done without executing")
	cmd.Flags().BoolVar(&noFormula, "no-formula", false, "suppress default formula (route raw bead)")
	cmd.MarkFlagsMutuallyExclusive("formula", "on")
	cmd.MarkFlagsMutuallyExclusive("no-formula", "formula")
	cmd.MarkFlagsMutuallyExclusive("no-formula", "on")
	return cmd
}

// slingOpts captures the user's intent from CLI flags.
type slingOpts struct {
	Target        config.Agent
	BeadOrFormula string
	IsFormula     bool
	OnFormula     string
	NoFormula     bool
	Title         string
	Vars          []string
	Merge         string // "", "direct", "mr", "local"
	NoConvoy      bool
	Owned         bool
	Nudge         bool
	Force         bool
	DryRun        bool
}

// slingDeps bundles infrastructure dependencies injected for testability.
type slingDeps struct {
	CityName string
	Cfg      *config.City
	SP       session.Provider
	Runner   SlingRunner
	Store    beads.Store
	Stdout   io.Writer
	Stderr   io.Writer
}

// SlingRunner executes a shell command and returns combined output.
type SlingRunner func(command string) (string, error)

// shellSlingRunner runs a command via sh -c and returns stdout.
func shellSlingRunner(command string) (string, error) {
	out, err := exec.Command("sh", "-c", command).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("running %q: %w", command, err)
	}
	return string(out), nil
}

// cmdSling is the CLI entry point for gc sling.
func cmdSling(args []string, isFormula, doNudge, force bool, title string, vars []string, merge string, noConvoy, owned bool, onFormula string, noFormula, dryRun bool, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, _, err := config.LoadWithIncludes(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	var target, beadOrFormula string
	if len(args) == 2 {
		target = args[0]
		beadOrFormula = args[1]
	} else {
		// 1-arg: bead ID only, resolve target from rig's default_sling_target.
		beadOrFormula = args[0]
		if isFormula {
			fmt.Fprintf(stderr, "gc sling: --formula requires explicit target\n") //nolint:errcheck // best-effort stderr
			return 1
		}
		bp := beadPrefix(beadOrFormula)
		if bp == "" {
			fmt.Fprintf(stderr, "gc sling: cannot derive rig from bead %q (no prefix)\n", beadOrFormula) //nolint:errcheck // best-effort stderr
			return 1
		}
		rig, found := findRigByPrefix(cfg, bp)
		if !found {
			fmt.Fprintf(stderr, "gc sling: no rig with prefix %q for bead %s\n", bp, beadOrFormula) //nolint:errcheck // best-effort stderr
			return 1
		}
		if rig.DefaultSlingTarget == "" {
			fmt.Fprintf(stderr, "gc sling: rig %q has no default_sling_target\n", rig.Name) //nolint:errcheck // best-effort stderr
			return 1
		}
		target = rig.DefaultSlingTarget
	}

	a, ok := resolveAgentIdentity(cfg, target, currentRigContext(cfg))
	if !ok {
		fmt.Fprintf(stderr, "gc sling: target %q not found in config\n", target) //nolint:errcheck // best-effort stderr
		return 1
	}

	sp := newSessionProvider()
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}

	opts := slingOpts{
		Target:        a,
		BeadOrFormula: beadOrFormula,
		IsFormula:     isFormula,
		OnFormula:     onFormula,
		NoFormula:     noFormula,
		Title:         title,
		Vars:          vars,
		Merge:         merge,
		NoConvoy:      noConvoy,
		Owned:         owned,
		Nudge:         doNudge,
		Force:         force,
		DryRun:        dryRun,
	}
	store := beads.NewBdStore(cityPath, beads.ExecCommandRunner())
	deps := slingDeps{
		CityName: cityName,
		Cfg:      cfg,
		SP:       sp,
		Runner:   shellSlingRunner,
		Store:    store,
		Stdout:   stdout,
		Stderr:   stderr,
	}

	return doSlingBatch(opts, deps, store)
}

// findRigByPrefix returns the rig whose effective prefix matches (case-insensitive).
func findRigByPrefix(cfg *config.City, prefix string) (config.Rig, bool) {
	lp := strings.ToLower(prefix)
	for _, r := range cfg.Rigs {
		if strings.ToLower(r.EffectivePrefix()) == lp {
			return r, true
		}
	}
	return config.Rig{}, false
}

// doSling is the pure logic for gc sling. Accepts injected deps, querier,
// and opts struct for testability.
func doSling(opts slingOpts, deps slingDeps, querier BeadQuerier) int {
	a := opts.Target
	// Warn about suspended agents / empty pools (unless --force).
	if a.Suspended && !opts.Force {
		fmt.Fprintf(deps.Stderr, "warning: agent %q is suspended — bead routed but may not be picked up\n", a.QualifiedName()) //nolint:errcheck // best-effort
	}
	if a.IsPool() && a.Pool.Max == 0 && !opts.Force {
		fmt.Fprintf(deps.Stderr, "warning: pool %q has max=0 — bead routed but no instances to claim it\n", a.QualifiedName()) //nolint:errcheck // best-effort
	}

	// Cross-rig guard — block when a rig-scoped agent receives a bead from
	// a different rig. Only for plain bead routing (formula creates fresh wisps).
	// Dry-run shows an informational section instead of blocking.
	if !opts.IsFormula && !opts.Force && !opts.DryRun {
		if msg := checkCrossRig(opts.BeadOrFormula, a, deps.Cfg); msg != "" {
			fmt.Fprintln(deps.Stderr, msg) //nolint:errcheck // best-effort
			return 1
		}
	}

	// Pre-flight idempotency check — before formula/wisp processing so an
	// idempotent bead skips ALL mutations. Only for plain bead routing
	// (formula mode creates fresh wisps, never idempotent).
	if !opts.IsFormula && !opts.Force {
		result := checkBeadState(querier, opts.BeadOrFormula, a)
		if result.Idempotent {
			if opts.DryRun {
				return dryRunSingle(opts, deps, querier)
			}
			fmt.Fprintf(deps.Stdout, "Bead %s already routed to %s — skipping (idempotent)\n", opts.BeadOrFormula, a.QualifiedName()) //nolint:errcheck // best-effort
			return 0
		}
		for _, w := range result.Warnings {
			fmt.Fprintln(deps.Stderr, w) //nolint:errcheck // best-effort
		}
	}

	// Dry-run: resolve and print preview without executing.
	if opts.DryRun {
		return dryRunSingle(opts, deps, querier)
	}

	beadID := opts.BeadOrFormula
	method := "bead"

	// If --formula, instantiate wisp and use the root bead ID.
	if opts.IsFormula {
		method = "formula"
		rootID, err := deps.Store.MolCook(opts.BeadOrFormula, opts.Title, opts.Vars)
		if err != nil {
			fmt.Fprintf(deps.Stderr, "gc sling: instantiating formula %q: %v\n", opts.BeadOrFormula, err) //nolint:errcheck // best-effort
			return 1
		}
		beadID = rootID
	}

	// If --on, attach a wisp to the bead and route the original bead.
	if opts.OnFormula != "" {
		method = "on-formula"
		if err := checkNoMoleculeChildren(querier, beadID); err != nil {
			fmt.Fprintf(deps.Stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort
			return 1
		}
		wispRootID, err := deps.Store.MolCookOn(opts.OnFormula, beadID, opts.Title, opts.Vars)
		if err != nil {
			fmt.Fprintf(deps.Stderr, "gc sling: instantiating formula %q on %s: %v\n", opts.OnFormula, beadID, err) //nolint:errcheck // best-effort
			return 1
		}
		fmt.Fprintf(deps.Stdout, "Attached wisp %s (formula %q) to %s\n", wispRootID, opts.OnFormula, beadID) //nolint:errcheck // best-effort
		// beadID unchanged — route original bead.
	}

	// Apply default formula if target has one and no explicit formula/--no-formula.
	if opts.OnFormula == "" && !opts.IsFormula && !opts.NoFormula && a.DefaultSlingFormula != "" {
		method = "default-on-formula"
		if err := checkNoMoleculeChildren(querier, beadID); err != nil {
			fmt.Fprintf(deps.Stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort
			return 1
		}
		wispRootID, err := deps.Store.MolCookOn(a.DefaultSlingFormula, beadID, opts.Title, opts.Vars)
		if err != nil {
			fmt.Fprintf(deps.Stderr, "gc sling: instantiating default formula %q on %s: %v\n", //nolint:errcheck // best-effort
				a.DefaultSlingFormula, beadID, err)
			return 1
		}
		fmt.Fprintf(deps.Stdout, "Attached wisp %s (default formula %q) to %s\n", //nolint:errcheck // best-effort
			wispRootID, a.DefaultSlingFormula, beadID)
	}

	// Build and execute sling command.
	slingCmd := buildSlingCommand(a.EffectiveSlingQuery(), beadID)
	if _, err := deps.Runner(slingCmd); err != nil {
		fmt.Fprintf(deps.Stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort
		telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), method, err)
		return 1
	}

	telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), method, nil)

	// Merge strategy metadata.
	if opts.Merge != "" && deps.Store != nil {
		if err := deps.Store.SetMetadata(beadID, "merge_strategy", opts.Merge); err != nil {
			fmt.Fprintf(deps.Stderr, "gc sling: setting merge strategy: %v\n", err) //nolint:errcheck // best-effort
			// Non-fatal — bead was already routed.
		}
	}

	// Auto-convoy: wrap single bead in a tracking convoy (unless suppressed).
	if !opts.NoConvoy && !opts.IsFormula && deps.Store != nil {
		var convoyLabels []string
		if opts.Owned {
			convoyLabels = []string{"owned"}
		}
		convoy, err := deps.Store.Create(beads.Bead{
			Title:  fmt.Sprintf("sling-%s", beadID),
			Type:   "convoy",
			Labels: convoyLabels,
		})
		if err != nil {
			fmt.Fprintf(deps.Stderr, "gc sling: creating auto-convoy: %v\n", err) //nolint:errcheck // best-effort
			// Non-fatal — bead was already routed successfully.
		} else {
			parentID := convoy.ID
			if err := deps.Store.Update(beadID, beads.UpdateOpts{ParentID: &parentID}); err != nil {
				fmt.Fprintf(deps.Stderr, "gc sling: linking bead to convoy: %v\n", err) //nolint:errcheck // best-effort
			} else {
				label := ""
				if opts.Owned {
					label = " (owned)"
				}
				fmt.Fprintf(deps.Stdout, "Auto-convoy %s%s\n", convoy.ID, label) //nolint:errcheck // best-effort
			}
		}
	}

	switch {
	case opts.IsFormula:
		fmt.Fprintf(deps.Stdout, "Slung formula %q (wisp root %s) → %s\n", opts.BeadOrFormula, beadID, a.QualifiedName()) //nolint:errcheck // best-effort
	case opts.OnFormula != "":
		fmt.Fprintf(deps.Stdout, "Slung %s (with formula %q) → %s\n", beadID, opts.OnFormula, a.QualifiedName()) //nolint:errcheck // best-effort
	default:
		fmt.Fprintf(deps.Stdout, "Slung %s → %s\n", beadID, a.QualifiedName()) //nolint:errcheck // best-effort
	}

	// Nudge target if requested.
	if opts.Nudge {
		doSlingNudge(&a, deps.CityName, deps.Cfg, deps.SP, deps.Stdout, deps.Stderr)
	}

	return 0
}

// doSlingBatch handles container bead expansion before delegating to doSling.
// If the argument is a container bead (convoy, epic), it expands open children
// and routes each individually. Otherwise it falls through to doSling.
func doSlingBatch(opts slingOpts, deps slingDeps, querier BeadChildQuerier) int {
	a := opts.Target
	// Formula mode, nil querier → delegate directly.
	if opts.IsFormula || querier == nil {
		return doSling(opts, deps, querier)
	}

	// Try to look up the bead to check if it's a container.
	b, err := querier.Get(opts.BeadOrFormula)
	if err != nil {
		// Can't query → fall through to doSling (best-effort).
		singleOpts := opts
		singleOpts.IsFormula = false
		return doSling(singleOpts, deps, querier)
	}

	if !beads.IsContainerType(b.Type) {
		singleOpts := opts
		singleOpts.IsFormula = false
		return doSling(singleOpts, deps, querier)
	}

	// Container expansion.
	children, err := querier.Children(b.ID)
	if err != nil {
		fmt.Fprintf(deps.Stderr, "gc sling: listing children of %s: %v\n", b.ID, err) //nolint:errcheck // best-effort
		return 1
	}

	// Partition children into open vs skipped.
	var open, skipped []beads.Bead
	for _, c := range children {
		if c.Status == "open" {
			open = append(open, c)
		} else {
			skipped = append(skipped, c)
		}
	}

	if len(open) == 0 {
		fmt.Fprintf(deps.Stderr, "gc sling: %s %s has no open children\n", b.Type, b.ID) //nolint:errcheck // best-effort
		return 1
	}

	// Cross-rig guard — check once on the container bead. Assumes all children
	// share the container's rig prefix. If a convoy contains beads from multiple
	// rigs, the per-child check would need to run inside the loop instead.
	if !opts.Force && !opts.DryRun {
		if msg := checkCrossRig(b.ID, a, deps.Cfg); msg != "" {
			fmt.Fprintln(deps.Stderr, msg) //nolint:errcheck // best-effort
			return 1
		}
	}

	// Pre-check: if --on or default formula, verify NO open child already has an attached molecule.
	useFormula := opts.OnFormula
	if useFormula == "" && !opts.IsFormula && !opts.NoFormula && a.DefaultSlingFormula != "" {
		useFormula = a.DefaultSlingFormula
	}
	if useFormula != "" {
		if err := checkBatchNoMoleculeChildren(querier, open); err != nil {
			fmt.Fprintf(deps.Stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort
			return 1
		}
	}

	// Dry-run: print container preview without executing.
	if opts.DryRun {
		return dryRunBatch(opts, deps, b, children, open, querier)
	}

	fmt.Fprintf(deps.Stdout, "Expanding %s %s (%d children, %d open)\n", b.Type, b.ID, len(children), len(open)) //nolint:errcheck // best-effort

	// Telemetry method.
	batchMethod := "batch"
	if opts.OnFormula != "" {
		batchMethod = "batch-on"
	} else if !opts.NoFormula && a.DefaultSlingFormula != "" {
		batchMethod = "batch-default-on"
	}

	// Route each open child.
	routed := 0
	failed := 0
	idempotent := 0
	for _, child := range open {
		// Per-child idempotency / pre-flight check (unless --force).
		if !opts.Force {
			result := checkBeadState(querier, child.ID, a)
			if result.Idempotent {
				fmt.Fprintf(deps.Stdout, "  Skipped %s — already routed to %s\n", child.ID, a.QualifiedName()) //nolint:errcheck // best-effort
				idempotent++
				continue
			}
			for _, w := range result.Warnings {
				fmt.Fprintln(deps.Stderr, w) //nolint:errcheck // best-effort
			}
		}

		// Attach wisp if --on.
		if opts.OnFormula != "" {
			wispRootID, err := deps.Store.MolCookOn(opts.OnFormula, child.ID, opts.Title, opts.Vars)
			if err != nil {
				fmt.Fprintf(deps.Stderr, "  Failed %s: instantiating formula %q: %v\n", child.ID, opts.OnFormula, err) //nolint:errcheck // best-effort
				telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), batchMethod, err)
				failed++
				continue
			}
			fmt.Fprintf(deps.Stdout, "  Attached wisp %s → %s\n", wispRootID, child.ID) //nolint:errcheck // best-effort
		} else if !opts.NoFormula && a.DefaultSlingFormula != "" {
			// Apply default formula per-child.
			wispRootID, err := deps.Store.MolCookOn(a.DefaultSlingFormula, child.ID, opts.Title, opts.Vars)
			if err != nil {
				fmt.Fprintf(deps.Stderr, "  Failed %s: instantiating default formula %q: %v\n", child.ID, a.DefaultSlingFormula, err) //nolint:errcheck // best-effort
				telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), batchMethod, err)
				failed++
				continue
			}
			fmt.Fprintf(deps.Stdout, "  Attached wisp %s (default formula) → %s\n", wispRootID, child.ID) //nolint:errcheck // best-effort
		}

		slingCmd := buildSlingCommand(a.EffectiveSlingQuery(), child.ID)
		if _, err := deps.Runner(slingCmd); err != nil {
			fmt.Fprintf(deps.Stderr, "  Failed %s: %v\n", child.ID, err) //nolint:errcheck // best-effort
			telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), batchMethod, err)
			failed++
			continue
		}

		telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), batchMethod, nil)
		fmt.Fprintf(deps.Stdout, "  Slung %s → %s\n", child.ID, a.QualifiedName()) //nolint:errcheck // best-effort
		routed++
	}

	// Report skipped children.
	for _, child := range skipped {
		fmt.Fprintf(deps.Stdout, "  Skipped %s (status: %s)\n", child.ID, child.Status) //nolint:errcheck // best-effort
	}

	// Summary line.
	summary := fmt.Sprintf("Slung %d/%d children of %s → %s", routed, len(children), b.ID, a.QualifiedName())
	if idempotent > 0 {
		summary += fmt.Sprintf(" (%d already routed)", idempotent)
	}
	fmt.Fprintln(deps.Stdout, summary) //nolint:errcheck // best-effort

	// Nudge once after all children.
	if opts.Nudge && routed > 0 {
		doSlingNudge(&a, deps.CityName, deps.Cfg, deps.SP, deps.Stdout, deps.Stderr)
	}

	if failed > 0 {
		return 1
	}
	return 0
}

// buildSlingCommand replaces {} in the sling query template with the bead ID.
func buildSlingCommand(template, beadID string) string {
	return strings.ReplaceAll(template, "{}", beadID)
}

// shellQuote quotes a string for safe use in shell commands.
// Uses single quotes with embedded single-quote escaping.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// formatBeadLabel formats a bead ID with optional title for display.
func formatBeadLabel(id, title string) string {
	if title != "" {
		return id + " — " + fmt.Sprintf("%q", title)
	}
	return id
}

// printCrossRigSection prints the Cross-rig dry-run section if applicable.
func printCrossRigSection(w func(string), beadID string, a config.Agent, cfg *config.City) {
	if msg := checkCrossRig(beadID, a, cfg); msg != "" {
		bp := beadPrefix(beadID)
		rp := rigPrefixForAgent(a, cfg)
		w("Cross-rig:")
		w(fmt.Sprintf("  Bead %s (prefix %q) targets %s (rig prefix %q).", beadID, bp, a.QualifiedName(), rp))
		w("  Without --force, sling would refuse to route (exit 1).")
		w("")
	}
}

// checkNoMoleculeChildren returns an error if the bead already has an attached
// molecule or wisp child. Best-effort: skips check if the querier doesn't
// support Children or if the query fails.
func checkNoMoleculeChildren(q BeadQuerier, beadID string) error {
	cq, ok := q.(BeadChildQuerier)
	if !ok || cq == nil {
		return nil // best-effort: can't check children
	}
	children, err := cq.Children(beadID)
	if err != nil {
		return nil // best-effort: query failed
	}
	for _, c := range children {
		if beads.IsMoleculeType(c.Type) {
			return fmt.Errorf("bead %s already has attached %s %s", beadID, c.Type, c.ID)
		}
	}
	return nil
}

// checkBatchNoMoleculeChildren checks all open children for existing molecule
// attachments before any wisps are created. Returns an error listing all
// problematic beads if any have attached molecules.
func checkBatchNoMoleculeChildren(q BeadChildQuerier, open []beads.Bead) error {
	var problems []string
	for _, child := range open {
		children, err := q.Children(child.ID)
		if err != nil {
			continue // best-effort per-child
		}
		for _, c := range children {
			if beads.IsMoleculeType(c.Type) {
				problems = append(problems, fmt.Sprintf("%s (has %s %s)", child.ID, c.Type, c.ID))
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("cannot use --on: beads already have attached molecules: %s",
			strings.Join(problems, ", "))
	}
	return nil
}

// targetType returns "pool" or "agent" for telemetry attributes.
func targetType(a *config.Agent) string {
	if a.IsPool() {
		return "pool"
	}
	return "agent"
}

// beadCheckResult captures the outcome of a pre-flight bead state check.
type beadCheckResult struct {
	Idempotent bool     // bead already routed to the same target
	Warnings   []string // warnings about existing routing to different targets
}

// checkBeadState checks whether a bead is already routed and returns a
// structured result. Callers decide how to handle idempotency vs warnings.
// Best-effort: nil querier or query failure → empty result (proceed silently).
func checkBeadState(q BeadQuerier, beadID string, a config.Agent) beadCheckResult {
	if q == nil {
		return beadCheckResult{}
	}
	b, err := q.Get(beadID)
	if err != nil {
		return beadCheckResult{} // best-effort: can't query → skip check
	}

	// Custom sling_query: can't determine idempotency — fall through to
	// generic warnings only.
	if isCustomSlingQuery(a) {
		var warnings []string
		if b.Assignee != "" {
			warnings = append(warnings, fmt.Sprintf("warning: bead %s already assigned to %q", beadID, b.Assignee))
		}
		for _, l := range b.Labels {
			if strings.HasPrefix(l, "pool:") {
				warnings = append(warnings, fmt.Sprintf("warning: bead %s already has pool label %q", beadID, l))
			}
		}
		return beadCheckResult{Warnings: warnings}
	}

	target := a.QualifiedName()

	// Fixed agent: check assignee match.
	if !a.IsPool() {
		if b.Assignee == target {
			return beadCheckResult{Idempotent: true}
		}
		var warnings []string
		if b.Assignee != "" {
			warnings = append(warnings, fmt.Sprintf("warning: bead %s already assigned to %q", beadID, b.Assignee))
		}
		for _, l := range b.Labels {
			if strings.HasPrefix(l, "pool:") {
				warnings = append(warnings, fmt.Sprintf("warning: bead %s already has pool label %q", beadID, l))
			}
		}
		return beadCheckResult{Warnings: warnings}
	}

	// Pool: check for matching pool label.
	poolLabel := "pool:" + target
	for _, l := range b.Labels {
		if l == poolLabel {
			return beadCheckResult{Idempotent: true}
		}
	}
	var warnings []string
	if b.Assignee != "" {
		warnings = append(warnings, fmt.Sprintf("warning: bead %s already assigned to %q", beadID, b.Assignee))
	}
	for _, l := range b.Labels {
		if strings.HasPrefix(l, "pool:") {
			warnings = append(warnings, fmt.Sprintf("warning: bead %s already has pool label %q", beadID, l))
		}
	}
	return beadCheckResult{Warnings: warnings}
}

// doSlingNudge sends a nudge to the target agent after routing.
// For pools, nudges the first running instance. Warns and skips if
// the target is not running.
func doSlingNudge(a *config.Agent, cityName string, cfg *config.City,
	sp session.Provider, stdout, stderr io.Writer,
) {
	st := cfg.Workspace.SessionTemplate

	if a.Suspended {
		fmt.Fprintf(stderr, "cannot nudge: agent %q is suspended\n", a.QualifiedName()) //nolint:errcheck // best-effort
		return
	}

	if a.IsPool() {
		// Find a running pool member to nudge.
		pool := a.EffectivePool()
		for i := 1; i <= pool.Max; i++ {
			name := a.Name
			if pool.Max > 1 {
				name = fmt.Sprintf("%s-%d", a.Name, i)
			}
			qn := name
			if a.Dir != "" {
				qn = a.Dir + "/" + name
			}
			sn := agent.SessionNameFor(cityName, qn, st)
			if sp.IsRunning(sn) {
				nudgeAgent := agent.New(qn, cityName, "", "", nil, agent.StartupHints{}, "", st, nil, sp)
				if err := nudgeAgent.Nudge("Work slung. Check your hook."); err != nil {
					fmt.Fprintf(stderr, "gc sling: nudge failed: %v\n", err) //nolint:errcheck // best-effort
				} else {
					fmt.Fprintf(stdout, "Nudged %s\n", qn) //nolint:errcheck // best-effort
				}
				return
			}
		}
		fmt.Fprintf(stderr, "cannot nudge: no running pool members for %q\n", a.QualifiedName()) //nolint:errcheck // best-effort
		return
	}

	// Fixed agent: nudge directly.
	sn := agent.SessionNameFor(cityName, a.QualifiedName(), st)
	if !sp.IsRunning(sn) {
		fmt.Fprintf(stderr, "cannot nudge: agent %q has no running session\n", a.QualifiedName()) //nolint:errcheck // best-effort
		return
	}
	nudgeAgent := agent.New(a.QualifiedName(), cityName, "", "", nil, agent.StartupHints{}, "", st, nil, sp)
	if err := nudgeAgent.Nudge("Work slung. Check your hook."); err != nil {
		fmt.Fprintf(stderr, "gc sling: nudge failed: %v\n", err) //nolint:errcheck // best-effort
	} else {
		fmt.Fprintf(stdout, "Nudged %s\n", a.QualifiedName()) //nolint:errcheck // best-effort
	}
}

// dryRunSingle prints a step-by-step preview of what gc sling would do for a
// single bead (or formula) without executing any side effects.
func dryRunSingle(opts slingOpts, deps slingDeps, querier BeadQuerier) int {
	a := opts.Target
	w := func(s string) { fmt.Fprintln(deps.Stdout, s) } //nolint:errcheck // best-effort

	// Header.
	header := "Dry run: gc sling " + a.QualifiedName() + " " + opts.BeadOrFormula
	if opts.IsFormula {
		header += " --formula"
	}
	if opts.OnFormula != "" {
		header += " --on=" + opts.OnFormula
	}
	w(header)
	w("")

	// Target section.
	printTarget(w, a)

	// Formula mode.
	if opts.IsFormula {
		w("Formula:")
		w("  Name: " + opts.BeadOrFormula)
		w("  A formula is a template for structured work. --formula creates a")
		w("  wisp (ephemeral molecule) — a tree of step beads that guide the")
		w("  agent through the workflow.")
		w("")
		cookCmd := fmt.Sprintf("bd mol cook --formula=%s", opts.BeadOrFormula)
		if opts.Title != "" {
			cookCmd += fmt.Sprintf(" --title=%s", opts.Title)
		}
		w("  Would run: " + cookCmd)
		w("  This creates a wisp and returns its root bead ID.")
		w("")

		routeCmd := buildSlingCommand(a.EffectiveSlingQuery(), "<wisp-root>")
		w("Route command (not executed):")
		w("  " + routeCmd)
		w("  The wisp root bead (not the formula name) is routed to the agent.")
		w("")
	} else {
		// Work section (bead info).
		printBeadInfo(w, querier, opts.BeadOrFormula)

		// Cross-rig section — show when bead prefix doesn't match agent's rig.
		printCrossRigSection(w, opts.BeadOrFormula, a, deps.Cfg)

		// Idempotency section — show when bead is already routed to this target.
		result := checkBeadState(querier, opts.BeadOrFormula, a)
		if result.Idempotent {
			w("Idempotency:")
			w("  Bead " + opts.BeadOrFormula + " is already routed to " + a.QualifiedName() + ".")
			w("  Without --force, sling would skip routing (exit 0).")
			w("")
		}

		// Attach formula section (--on or default).
		if opts.OnFormula != "" {
			if err := checkNoMoleculeChildren(querier, opts.BeadOrFormula); err != nil {
				fmt.Fprintf(deps.Stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort
				return 1
			}

			w("Attach formula:")
			w("  Formula: " + opts.OnFormula)
			w("  --on attaches a wisp (structured work instructions) to an existing")
			w("  bead. The agent receives the original bead with the workflow")
			w("  attached, rather than a standalone wisp.")
			w("")
			cookCmd := fmt.Sprintf("bd mol cook --formula=%s --on=%s", opts.OnFormula, opts.BeadOrFormula)
			if opts.Title != "" {
				cookCmd += fmt.Sprintf(" --title=%s", opts.Title)
			}
			w("  Would run: " + cookCmd)
			w("  Pre-check: " + opts.BeadOrFormula + " has no existing molecule/wisp children ✓")
			w("")
		} else if !opts.NoFormula && a.DefaultSlingFormula != "" {
			if err := checkNoMoleculeChildren(querier, opts.BeadOrFormula); err != nil {
				fmt.Fprintf(deps.Stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort
				return 1
			}

			w("Default formula:")
			w("  Formula: " + a.DefaultSlingFormula)
			w("  Target " + a.QualifiedName() + " has a default_sling_formula configured.")
			w("  A wisp will be attached automatically (use --no-formula to suppress).")
			w("")
			cookCmd := fmt.Sprintf("bd mol cook --formula=%s --on=%s", a.DefaultSlingFormula, opts.BeadOrFormula)
			if opts.Title != "" {
				cookCmd += fmt.Sprintf(" --title=%s", opts.Title)
			}
			w("  Would run: " + cookCmd)
			w("  Pre-check: " + opts.BeadOrFormula + " has no existing molecule/wisp children ✓")
			w("")
		}

		routeCmd := buildSlingCommand(a.EffectiveSlingQuery(), opts.BeadOrFormula)
		w("Route command (not executed):")
		w("  " + routeCmd)
		if !isCustomSlingQuery(a) {
			if a.IsPool() {
				w("  This labels the bead for pool \"" + a.QualifiedName() + "\".")
			} else {
				w("  This assigns the bead to \"" + a.QualifiedName() + "\".")
			}
		}
		w("")
	}

	// Nudge section.
	if opts.Nudge {
		printNudgePreview(w, a, deps.CityName, deps.SP, deps.Cfg)
	}

	w("No side effects executed (--dry-run).")
	return 0
}

// dryRunBatch prints a step-by-step preview of what gc sling would do for a
// container bead (convoy, epic) without executing any side effects.
func dryRunBatch(opts slingOpts, deps slingDeps,
	b beads.Bead, children, open []beads.Bead, querier BeadQuerier,
) int {
	a := opts.Target
	w := func(s string) { fmt.Fprintln(deps.Stdout, s) } //nolint:errcheck // best-effort

	// Header.
	w("Dry run: gc sling " + a.QualifiedName() + " " + b.ID)
	w("")

	// Target section.
	printTarget(w, a)

	// Work section — container.
	w("Work:")
	label := formatBeadLabel(b.ID, b.Title)
	w("  Bead: " + label)
	w("  Type: " + b.Type)
	w("")
	w("  A " + b.Type + " is a container bead that groups related work. Sling")
	w("  expands it and routes each open child individually.")
	w("")

	// Cross-rig section — show when container bead prefix doesn't match agent's rig.
	printCrossRigSection(w, b.ID, a, deps.Cfg)

	// Children list.
	w(fmt.Sprintf("  Children (%d total, %d open):", len(children), len(open)))
	for _, c := range children {
		clabel := formatBeadLabel(c.ID, c.Title)
		if c.Status == "open" {
			// Check idempotency for open children.
			result := checkBeadState(querier, c.ID, a)
			if result.Idempotent {
				w("    " + clabel + " (open) → already routed (skip)")
			} else {
				suffix := " → would route"
				if opts.OnFormula != "" || (!opts.NoFormula && a.DefaultSlingFormula != "") {
					suffix = " → would route + attach wisp"
				}
				w("    " + clabel + " (open)" + suffix)
			}
		} else {
			w("    " + clabel + " (" + c.Status + ") → skip")
		}
	}
	w("")

	// Attach formula section (per open child).
	if opts.OnFormula != "" {
		w("Attach formula (per open child):")
		w("  Would run:")
		for _, c := range open {
			w("    bd mol cook --formula=" + opts.OnFormula + " --on=" + c.ID)
		}
		w("")
	} else if !opts.NoFormula && a.DefaultSlingFormula != "" {
		w("Default formula (per open child):")
		w("  Formula: " + a.DefaultSlingFormula)
		w("  Would run:")
		for _, c := range open {
			w("    bd mol cook --formula=" + a.DefaultSlingFormula + " --on=" + c.ID)
		}
		w("")
	}

	// Route commands.
	w("Route commands (not executed):")
	for _, c := range open {
		routeCmd := buildSlingCommand(a.EffectiveSlingQuery(), c.ID)
		w("  " + routeCmd)
	}
	w("")

	// Nudge section.
	if opts.Nudge {
		printNudgePreview(w, a, deps.CityName, deps.SP, deps.Cfg)
	}

	w("No side effects executed (--dry-run).")
	return 0
}

// printTarget prints the Target section for dry-run output.
func printTarget(w func(string), a config.Agent) {
	w("Target:")
	if a.IsPool() {
		pool := a.EffectivePool()
		w(fmt.Sprintf("  Pool:        %s (min=%d max=%d)", a.QualifiedName(), pool.Min, pool.Max))
	} else {
		w("  Agent:       " + a.QualifiedName() + " (fixed agent)")
	}
	sq := a.EffectiveSlingQuery()
	w("  Sling query: " + sq)
	if !isCustomSlingQuery(a) {
		if a.IsPool() {
			w("               Pool agents share a work queue via labels instead of")
			w("               direct assignment. Any idle pool member can claim work")
			w("               labeled for its pool.")
		} else {
			w("               A sling query is the shell command that routes work.")
			w("               {} is replaced with the bead ID at dispatch time.")
		}
	}
	w("")
}

// printBeadInfo prints the Work section for dry-run output. Gracefully handles
// nil querier or query failure by showing the bead ID only.
func printBeadInfo(w func(string), q BeadQuerier, beadID string) {
	w("Work:")
	if q == nil {
		w("  Bead: " + beadID)
		w("")
		return
	}
	b, err := q.Get(beadID)
	if err != nil {
		w("  Bead: " + beadID)
		w("")
		return
	}
	title := beadID
	if b.Title != "" {
		title = beadID + " — " + fmt.Sprintf("%q", b.Title)
	}
	w("  Bead:   " + title)
	if b.Type != "" {
		w("  Type:   " + b.Type)
	}
	if b.Status != "" {
		w("  Status: " + b.Status)
	}
	w("")
}

// printNudgePreview prints the Nudge section for dry-run output.
func printNudgePreview(w func(string), a config.Agent, cityName string,
	sp session.Provider, cfg *config.City,
) {
	st := cfg.Workspace.SessionTemplate
	w("Nudge:")
	sn := agent.SessionNameFor(cityName, a.QualifiedName(), st)
	if sp.IsRunning(sn) {
		w("  Would nudge " + a.QualifiedName() + " (session " + sn + ").")
		w("  Currently: running ✓")
	} else {
		w("  Would nudge " + a.QualifiedName() + " — but no running session found.")
	}
	w("")
}

// isCustomSlingQuery returns true if the agent has a user-defined sling_query
// (not the auto-generated default).
func isCustomSlingQuery(a config.Agent) bool {
	return a.SlingQuery != ""
}

// beadPrefix extracts the rig prefix from a bead ID by taking the lowercase
// letters before the first dash. "HW-7" → "hw", "FE-123" → "fe".
// Returns "" if the ID has no dash (can't determine prefix).
func beadPrefix(beadID string) string {
	i := strings.Index(beadID, "-")
	if i <= 0 {
		return ""
	}
	return strings.ToLower(beadID[:i])
}

// rigPrefixForAgent returns the effective bead prefix for the rig that an
// agent belongs to. City-wide agents (Dir="") return "" (exempt from cross-rig
// checks). Returns "" if no matching rig is found (best-effort skip).
func rigPrefixForAgent(a config.Agent, cfg *config.City) string {
	if a.Dir == "" {
		return ""
	}
	for i := range cfg.Rigs {
		if cfg.Rigs[i].Name == a.Dir {
			return strings.ToLower(cfg.Rigs[i].EffectivePrefix())
		}
	}
	return ""
}

// checkCrossRig returns a non-empty error message if a bead's rig prefix
// doesn't match the target agent's rig prefix. Returns "" when the check
// passes or can't be performed (missing prefix, city-wide agent, no rig).
func checkCrossRig(beadID string, a config.Agent, cfg *config.City) string {
	bp := beadPrefix(beadID)
	if bp == "" {
		return ""
	}
	rp := rigPrefixForAgent(a, cfg)
	if rp == "" {
		return ""
	}
	if bp == rp {
		return ""
	}
	return fmt.Sprintf("gc sling: cross-rig routing blocked — bead %s (prefix %q) targets %s (rig prefix %q); use --force to override",
		beadID, bp, a.QualifiedName(), rp)
}
