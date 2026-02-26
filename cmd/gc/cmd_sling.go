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
	var onFormula string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "sling <target> <bead-or-formula>",
		Short: "Route work to an agent or pool",
		Long: `Route a bead to an agent or pool using the target's sling_query.

The target is an agent qualified name (e.g. "mayor" or "hello-world/polecat").
The second argument is a bead ID, or a formula name when --formula is set.

With --formula, a wisp (ephemeral molecule) is instantiated from the formula
and its root bead is routed to the target.`,
		Args: cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			code := cmdSling(args[0], args[1], formula, nudge, force, title, onFormula, dryRun, stdout, stderr)
			if code != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&formula, "formula", "f", false, "treat argument as formula name")
	cmd.Flags().BoolVar(&nudge, "nudge", false, "nudge target after routing")
	cmd.Flags().BoolVar(&force, "force", false, "suppress warnings for suspended/empty targets")
	cmd.Flags().StringVarP(&title, "title", "t", "", "wisp root bead title (with --formula or --on)")
	cmd.Flags().StringVar(&onFormula, "on", "", "attach wisp from formula to bead before routing")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "show what would be done without executing")
	cmd.MarkFlagsMutuallyExclusive("formula", "on")
	return cmd
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
func cmdSling(target, beadOrFormula string, isFormula, doNudge, force bool, title, onFormula string, dryRun bool, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
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

	store := beads.NewBdStore(cityPath, beads.ExecCommandRunner())
	return doSlingBatch(a, beadOrFormula, isFormula, doNudge, force, title, onFormula,
		dryRun, cityName, cfg, sp, shellSlingRunner, store, stdout, stderr)
}

// doSling is the pure logic for gc sling. Accepts injected runner, querier,
// and session provider for testability.
func doSling(a config.Agent, beadOrFormula string, isFormula, doNudge, force bool,
	title, onFormula string, dryRun bool, cityName string, cfg *config.City,
	sp session.Provider, runner SlingRunner, querier BeadQuerier,
	stdout, stderr io.Writer,
) int {
	// Warn about suspended agents / empty pools (unless --force).
	if a.Suspended && !force {
		fmt.Fprintf(stderr, "warning: agent %q is suspended — bead routed but may not be picked up\n", a.QualifiedName()) //nolint:errcheck // best-effort
	}
	if a.IsPool() && a.Pool.Max == 0 && !force {
		fmt.Fprintf(stderr, "warning: pool %q has max=0 — bead routed but no instances to claim it\n", a.QualifiedName()) //nolint:errcheck // best-effort
	}

	// Dry-run: resolve and print preview without executing.
	if dryRun {
		return dryRunSingle(a, beadOrFormula, isFormula, onFormula, title,
			doNudge, cityName, sp, cfg, querier, stdout, stderr)
	}

	beadID := beadOrFormula
	method := "bead"

	// If --formula, instantiate wisp and use the root bead ID.
	if isFormula {
		method = "formula"
		rootID, err := instantiateWisp(beadOrFormula, title, runner)
		if err != nil {
			fmt.Fprintf(stderr, "gc sling: instantiating formula %q: %v\n", beadOrFormula, err) //nolint:errcheck // best-effort
			return 1
		}
		beadID = rootID
	}

	// If --on, attach a wisp to the bead and route the original bead.
	if onFormula != "" {
		method = "on-formula"
		if err := checkNoMoleculeChildren(querier, beadID); err != nil {
			fmt.Fprintf(stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort
			return 1
		}
		wispRootID, err := instantiateWispOn(onFormula, beadID, title, runner)
		if err != nil {
			fmt.Fprintf(stderr, "gc sling: instantiating formula %q on %s: %v\n", onFormula, beadID, err) //nolint:errcheck // best-effort
			return 1
		}
		fmt.Fprintf(stdout, "Attached wisp %s (formula %q) to %s\n", wispRootID, onFormula, beadID) //nolint:errcheck // best-effort
		// beadID unchanged — route original bead.
	}

	// Pre-flight: warn about already-routed beads (unless --force).
	if !force {
		checkBeadState(querier, beadID, stderr)
	}

	// Build and execute sling command.
	slingCmd := buildSlingCommand(a.EffectiveSlingQuery(), beadID)
	if _, err := runner(slingCmd); err != nil {
		fmt.Fprintf(stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort
		telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), method, err)
		return 1
	}

	telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), method, nil)

	switch {
	case isFormula:
		fmt.Fprintf(stdout, "Slung formula %q (wisp root %s) → %s\n", beadOrFormula, beadID, a.QualifiedName()) //nolint:errcheck // best-effort
	case onFormula != "":
		fmt.Fprintf(stdout, "Slung %s (with formula %q) → %s\n", beadID, onFormula, a.QualifiedName()) //nolint:errcheck // best-effort
	default:
		fmt.Fprintf(stdout, "Slung %s → %s\n", beadID, a.QualifiedName()) //nolint:errcheck // best-effort
	}

	// Nudge target if requested.
	if doNudge {
		doSlingNudge(&a, cityName, cfg, sp, stdout, stderr)
	}

	return 0
}

// doSlingBatch handles container bead expansion before delegating to doSling.
// If the argument is a container bead (convoy, epic), it expands open children
// and routes each individually. Otherwise it falls through to doSling.
func doSlingBatch(
	a config.Agent, beadOrFormula string, isFormula, doNudge, force bool,
	title, onFormula string, dryRun bool, cityName string, cfg *config.City,
	sp session.Provider, runner SlingRunner, querier BeadChildQuerier,
	stdout, stderr io.Writer,
) int {
	// Formula mode, nil querier → delegate directly.
	if isFormula || querier == nil {
		return doSling(a, beadOrFormula, isFormula, doNudge, force, title, onFormula,
			dryRun, cityName, cfg, sp, runner, querier, stdout, stderr)
	}

	// Try to look up the bead to check if it's a container.
	b, err := querier.Get(beadOrFormula)
	if err != nil {
		// Can't query → fall through to doSling (best-effort).
		return doSling(a, beadOrFormula, false, doNudge, force, title, onFormula,
			dryRun, cityName, cfg, sp, runner, querier, stdout, stderr)
	}

	if !beads.IsContainerType(b.Type) {
		return doSling(a, beadOrFormula, false, doNudge, force, title, onFormula,
			dryRun, cityName, cfg, sp, runner, querier, stdout, stderr)
	}

	// Container expansion.
	children, err := querier.Children(b.ID)
	if err != nil {
		fmt.Fprintf(stderr, "gc sling: listing children of %s: %v\n", b.ID, err) //nolint:errcheck // best-effort
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
		fmt.Fprintf(stderr, "gc sling: %s %s has no open children\n", b.Type, b.ID) //nolint:errcheck // best-effort
		return 1
	}

	// Pre-check: if --on, verify NO open child already has an attached molecule.
	if onFormula != "" {
		if err := checkBatchNoMoleculeChildren(querier, open); err != nil {
			fmt.Fprintf(stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort
			return 1
		}
	}

	// Dry-run: print container preview without executing.
	if dryRun {
		return dryRunBatch(a, b, children, open, skipped,
			onFormula, doNudge, cityName, sp, cfg, stdout, stderr)
	}

	fmt.Fprintf(stdout, "Expanding %s %s (%d children, %d open)\n", b.Type, b.ID, len(children), len(open)) //nolint:errcheck // best-effort

	// Telemetry method.
	batchMethod := "batch"
	if onFormula != "" {
		batchMethod = "batch-on"
	}

	// Route each open child.
	routed := 0
	failed := 0
	for _, child := range open {
		// Per-child pre-flight check (unless --force).
		if !force {
			checkBeadState(querier, child.ID, stderr)
		}

		// Attach wisp if --on.
		if onFormula != "" {
			wispRootID, err := instantiateWispOn(onFormula, child.ID, title, runner)
			if err != nil {
				fmt.Fprintf(stderr, "  Failed %s: instantiating formula %q: %v\n", child.ID, onFormula, err) //nolint:errcheck // best-effort
				telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), batchMethod, err)
				failed++
				continue
			}
			fmt.Fprintf(stdout, "  Attached wisp %s → %s\n", wispRootID, child.ID) //nolint:errcheck // best-effort
		}

		slingCmd := buildSlingCommand(a.EffectiveSlingQuery(), child.ID)
		if _, err := runner(slingCmd); err != nil {
			fmt.Fprintf(stderr, "  Failed %s: %v\n", child.ID, err) //nolint:errcheck // best-effort
			telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), batchMethod, err)
			failed++
			continue
		}

		telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), batchMethod, nil)
		fmt.Fprintf(stdout, "  Slung %s → %s\n", child.ID, a.QualifiedName()) //nolint:errcheck // best-effort
		routed++
	}

	// Report skipped children.
	for _, child := range skipped {
		fmt.Fprintf(stdout, "  Skipped %s (status: %s)\n", child.ID, child.Status) //nolint:errcheck // best-effort
	}

	// Summary line.
	fmt.Fprintf(stdout, "Slung %d/%d children of %s → %s\n", routed, len(children), b.ID, a.QualifiedName()) //nolint:errcheck // best-effort

	// Nudge once after all children.
	if doNudge && routed > 0 {
		doSlingNudge(&a, cityName, cfg, sp, stdout, stderr)
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

// instantiateWisp creates an ephemeral molecule from a formula and returns
// the root bead ID. Uses "bd mol cook" to instantiate.
func instantiateWisp(formulaName, title string, runner SlingRunner) (string, error) {
	cmd := "bd mol cook --formula=" + formulaName
	if title != "" {
		cmd += " --title=" + title
	}
	out, err := runner(cmd)
	if err != nil {
		return "", err
	}
	rootID := strings.TrimSpace(out)
	if rootID == "" {
		return "", fmt.Errorf("bd mol cook produced empty output")
	}
	return rootID, nil
}

// instantiateWispOn creates an ephemeral molecule from a formula attached to an
// existing bead, and returns the wisp root bead ID. Uses "bd mol cook --on".
func instantiateWispOn(formulaName, beadID, title string, runner SlingRunner) (string, error) {
	cmd := "bd mol cook --formula=" + formulaName + " --on=" + beadID
	if title != "" {
		cmd += " --title=" + title
	}
	out, err := runner(cmd)
	if err != nil {
		return "", err
	}
	rootID := strings.TrimSpace(out)
	if rootID == "" {
		return "", fmt.Errorf("bd mol cook produced empty output")
	}
	return rootID, nil
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

// checkBeadState warns if the bead already has an assignee or pool labels.
// Best-effort: query failure → no warning, proceed silently.
// Returns nothing — warnings go to stderr, never blocks routing.
func checkBeadState(q BeadQuerier, beadID string, stderr io.Writer) {
	if q == nil {
		return
	}
	b, err := q.Get(beadID)
	if err != nil {
		return // best-effort: can't query → skip check
	}
	if b.Assignee != "" {
		fmt.Fprintf(stderr, "warning: bead %s already assigned to %q\n", beadID, b.Assignee) //nolint:errcheck // best-effort
	}
	for _, l := range b.Labels {
		if strings.HasPrefix(l, "pool:") {
			fmt.Fprintf(stderr, "warning: bead %s already has pool label %q\n", beadID, l) //nolint:errcheck // best-effort
		}
	}
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
func dryRunSingle(
	a config.Agent, beadOrFormula string, isFormula bool,
	onFormula, title string, doNudge bool,
	cityName string, sp session.Provider, cfg *config.City,
	querier BeadQuerier, stdout, stderr io.Writer,
) int {
	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort

	// Header.
	header := "Dry run: gc sling " + a.QualifiedName() + " " + beadOrFormula
	if isFormula {
		header += " --formula"
	}
	if onFormula != "" {
		header += " --on=" + onFormula
	}
	w(header)
	w("")

	// Target section.
	printTarget(w, a)

	// Formula mode.
	if isFormula {
		w("Formula:")
		w("  Name: " + beadOrFormula)
		w("  A formula is a template for structured work. --formula creates a")
		w("  wisp (ephemeral molecule) — a tree of step beads that guide the")
		w("  agent through the workflow.")
		w("")
		cookCmd := "bd mol cook --formula=" + beadOrFormula
		if title != "" {
			cookCmd += " --title=" + title
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
		printBeadInfo(w, querier, beadOrFormula)

		// Attach formula section (--on).
		if onFormula != "" {
			if err := checkNoMoleculeChildren(querier, beadOrFormula); err != nil {
				fmt.Fprintf(stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort
				return 1
			}

			w("Attach formula:")
			w("  Formula: " + onFormula)
			w("  --on attaches a wisp (structured work instructions) to an existing")
			w("  bead. The agent receives the original bead with the workflow")
			w("  attached, rather than a standalone wisp.")
			w("")
			cookCmd := "bd mol cook --formula=" + onFormula + " --on=" + beadOrFormula
			if title != "" {
				cookCmd += " --title=" + title
			}
			w("  Would run: " + cookCmd)
			w("  Pre-check: " + beadOrFormula + " has no existing molecule/wisp children ✓")
			w("")
		}

		routeCmd := buildSlingCommand(a.EffectiveSlingQuery(), beadOrFormula)
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
	if doNudge {
		printNudgePreview(w, a, cityName, sp, cfg)
	}

	w("No side effects executed (--dry-run).")
	return 0
}

// dryRunBatch prints a step-by-step preview of what gc sling would do for a
// container bead (convoy, epic) without executing any side effects.
func dryRunBatch(
	a config.Agent, b beads.Bead, children, open, _ []beads.Bead,
	onFormula string, doNudge bool,
	cityName string, sp session.Provider, cfg *config.City,
	stdout, _ io.Writer,
) int {
	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort

	// Header.
	w("Dry run: gc sling " + a.QualifiedName() + " " + b.ID)
	w("")

	// Target section.
	printTarget(w, a)

	// Work section — container.
	w("Work:")
	title := b.ID
	if b.Title != "" {
		title = b.ID + " — " + fmt.Sprintf("%q", b.Title)
	}
	w("  Bead: " + title)
	w("  Type: " + b.Type)
	w("")
	w("  A " + b.Type + " is a container bead that groups related work. Sling")
	w("  expands it and routes each open child individually.")
	w("")

	// Children list.
	w(fmt.Sprintf("  Children (%d total, %d open):", len(children), len(open)))
	for _, c := range children {
		label := c.ID
		if c.Title != "" {
			label += " — " + fmt.Sprintf("%q", c.Title)
		}
		if c.Status == "open" {
			suffix := " → would route"
			if onFormula != "" {
				suffix = " → would route + attach wisp"
			}
			w("    " + label + " (open)" + suffix)
		} else {
			w("    " + label + " (" + c.Status + ") → skip")
		}
	}
	w("")

	// Attach formula section (per open child).
	if onFormula != "" {
		w("Attach formula (per open child):")
		w("  Would run:")
		for _, c := range open {
			w("    bd mol cook --formula=" + onFormula + " --on=" + c.ID)
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
	if doNudge {
		printNudgePreview(w, a, cityName, sp, cfg)
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
