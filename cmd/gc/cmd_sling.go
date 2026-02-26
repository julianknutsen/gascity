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
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/session"
	"github.com/steveyegge/gascity/internal/telemetry"
)

func newSlingCmd(stdout, stderr io.Writer) *cobra.Command {
	var formula bool
	var nudge bool
	var force bool
	var title string
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
			code := cmdSling(args[0], args[1], formula, nudge, force, title, stdout, stderr)
			if code != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&formula, "formula", "f", false, "treat argument as formula name")
	cmd.Flags().BoolVar(&nudge, "nudge", false, "nudge target after routing")
	cmd.Flags().BoolVar(&force, "force", false, "suppress warnings for suspended/empty targets")
	cmd.Flags().StringVarP(&title, "title", "t", "", "wisp root bead title (with --formula)")
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
func cmdSling(target, beadOrFormula string, isFormula, doNudge, force bool, title string, stdout, stderr io.Writer) int {
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

	return doSling(a, beadOrFormula, isFormula, doNudge, force, title,
		cityName, cfg, sp, shellSlingRunner, stdout, stderr)
}

// doSling is the pure logic for gc sling. Accepts injected runner and
// session provider for testability.
func doSling(a config.Agent, beadOrFormula string, isFormula, doNudge, force bool,
	title, cityName string, cfg *config.City,
	sp session.Provider, runner SlingRunner,
	stdout, stderr io.Writer,
) int {
	// Warn about suspended agents / empty pools (unless --force).
	if a.Suspended && !force {
		fmt.Fprintf(stderr, "warning: agent %q is suspended — bead routed but may not be picked up\n", a.QualifiedName()) //nolint:errcheck // best-effort
	}
	if a.IsPool() && a.Pool.Max == 0 && !force {
		fmt.Fprintf(stderr, "warning: pool %q has max=0 — bead routed but no instances to claim it\n", a.QualifiedName()) //nolint:errcheck // best-effort
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

	// Build and execute sling command.
	slingCmd := buildSlingCommand(a.EffectiveSlingQuery(), beadID)
	if _, err := runner(slingCmd); err != nil {
		fmt.Fprintf(stderr, "gc sling: %v\n", err) //nolint:errcheck // best-effort
		telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), method, err)
		return 1
	}

	telemetry.RecordSling(context.Background(), a.QualifiedName(), targetType(&a), method, nil)

	if isFormula {
		fmt.Fprintf(stdout, "Slung formula %q (wisp root %s) → %s\n", beadOrFormula, beadID, a.QualifiedName()) //nolint:errcheck // best-effort
	} else {
		fmt.Fprintf(stdout, "Slung %s → %s\n", beadID, a.QualifiedName()) //nolint:errcheck // best-effort
	}

	// Nudge target if requested.
	if doNudge {
		doSlingNudge(&a, cityName, cfg, sp, stdout, stderr)
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

// targetType returns "pool" or "agent" for telemetry attributes.
func targetType(a *config.Agent) string {
	if a.IsPool() {
		return "pool"
	}
	return "agent"
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
