package main

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/plugins"
)

func newPluginCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage plugins (periodic formula dispatch)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc plugin: missing subcommand (list, show, run, check, history)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc plugin: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newPluginListCmd(stdout, stderr),
		newPluginShowCmd(stdout, stderr),
		newPluginRunCmd(stdout, stderr),
		newPluginCheckCmd(stdout, stderr),
		newPluginHistoryCmd(stdout, stderr),
	)
	return cmd
}

func newPluginListCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available plugins",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdPluginList(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newPluginShowCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show details of a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdPluginShow(args[0], stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newPluginRunCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "run <name>",
		Short: "Execute a plugin manually",
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdPluginRun(args[0], stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newPluginCheckCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check which plugins are due to run",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdPluginCheck(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newPluginHistoryCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "history [name]",
		Short: "Show plugin execution history",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			name := ""
			if len(args) > 0 {
				name = args[0]
			}
			if cmdPluginHistory(name, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// loadPlugins is the common preamble for plugin commands: resolve city,
// load config, scan formula layers for plugins.
func loadPlugins(stderr io.Writer, cmdName string) ([]plugins.Plugin, int) {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
		return nil, 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
		return nil, 1
	}

	layers := pluginFormulaLayers(cityPath, cfg)
	found, err := plugins.Scan(fsys.OSFS{}, layers, cfg.Plugins.Skip)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", cmdName, err) //nolint:errcheck // best-effort stderr
		return nil, 1
	}
	return found, 0
}

// pluginFormulaLayers returns the formula directory layers for plugin scanning.
// Uses FormulaLayers.City if populated (from LoadWithIncludes), otherwise
// falls back to the single formulas dir.
func pluginFormulaLayers(cityPath string, cfg *config.City) []string {
	if len(cfg.FormulaLayers.City) > 0 {
		return cfg.FormulaLayers.City
	}
	return []string{filepath.Join(cityPath, cfg.FormulasDir())}
}

// --- gc plugin list ---

func cmdPluginList(stdout, stderr io.Writer) int {
	pp, code := loadPlugins(stderr, "gc plugin list")
	if code != 0 {
		return code
	}
	return doPluginList(pp, stdout)
}

// doPluginList prints a table of plugins. Accepts pre-scanned plugins for testability.
func doPluginList(pp []plugins.Plugin, stdout io.Writer) int {
	if len(pp) == 0 {
		fmt.Fprintln(stdout, "No plugins found.") //nolint:errcheck // best-effort stdout
		return 0
	}

	fmt.Fprintf(stdout, "%-20s %-12s %-15s %s\n", "NAME", "GATE", "INTERVAL/SCHED", "POOL") //nolint:errcheck
	for _, p := range pp {
		timing := p.Interval
		if timing == "" {
			timing = p.Schedule
		}
		if timing == "" {
			timing = "-"
		}
		pool := p.Pool
		if pool == "" {
			pool = "-"
		}
		fmt.Fprintf(stdout, "%-20s %-12s %-15s %s\n", p.Name, p.Gate, timing, pool) //nolint:errcheck
	}
	return 0
}

// --- gc plugin show ---

func cmdPluginShow(name string, stdout, stderr io.Writer) int {
	pp, code := loadPlugins(stderr, "gc plugin show")
	if code != 0 {
		return code
	}
	return doPluginShow(pp, name, stdout, stderr)
}

// doPluginShow prints details of a named plugin.
func doPluginShow(pp []plugins.Plugin, name string, stdout, stderr io.Writer) int {
	p, ok := findPlugin(pp, name)
	if !ok {
		fmt.Fprintf(stderr, "gc plugin show: plugin %q not found\n", name) //nolint:errcheck // best-effort stderr
		return 1
	}

	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout
	w(fmt.Sprintf("Plugin:      %s", p.Name))
	if p.Description != "" {
		w(fmt.Sprintf("Description: %s", p.Description))
	}
	w(fmt.Sprintf("Formula:     %s", p.Formula))
	w(fmt.Sprintf("Gate:        %s", p.Gate))
	if p.Interval != "" {
		w(fmt.Sprintf("Interval:    %s", p.Interval))
	}
	if p.Schedule != "" {
		w(fmt.Sprintf("Schedule:    %s", p.Schedule))
	}
	if p.Check != "" {
		w(fmt.Sprintf("Check:       %s", p.Check))
	}
	if p.Pool != "" {
		w(fmt.Sprintf("Pool:        %s", p.Pool))
	}
	w(fmt.Sprintf("Source:      %s", p.Source))
	return 0
}

// --- gc plugin run ---

func cmdPluginRun(name string, stdout, stderr io.Writer) int {
	pp, code := loadPlugins(stderr, "gc plugin run")
	if code != 0 {
		return code
	}
	return doPluginRun(pp, name, shellSlingRunner, stdout, stderr)
}

// doPluginRun executes a plugin manually: instantiates a wisp from the
// plugin's formula and routes it to the target pool.
func doPluginRun(pp []plugins.Plugin, name string, runner SlingRunner, stdout, stderr io.Writer) int {
	p, ok := findPlugin(pp, name)
	if !ok {
		fmt.Fprintf(stderr, "gc plugin run: plugin %q not found\n", name) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Instantiate wisp from formula.
	rootID, err := instantiateWisp(p.Formula, "", nil, runner)
	if err != nil {
		fmt.Fprintf(stderr, "gc plugin run: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Route to pool if specified.
	if p.Pool != "" {
		routeCmd := fmt.Sprintf("bd update %s --label=pool:%s", rootID, p.Pool)
		if _, err := runner(routeCmd); err != nil {
			fmt.Fprintf(stderr, "gc plugin run: routing to pool %q: %v\n", p.Pool, err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	fmt.Fprintf(stdout, "Plugin %q executed: wisp %s", name, rootID) //nolint:errcheck
	if p.Pool != "" {
		fmt.Fprintf(stdout, " → pool:%s", p.Pool) //nolint:errcheck
	}
	fmt.Fprintln(stdout) //nolint:errcheck
	return 0
}

// --- gc plugin check ---

func cmdPluginCheck(stdout, stderr io.Writer) int {
	pp, code := loadPlugins(stderr, "gc plugin check")
	if code != 0 {
		return code
	}

	// Use a stub lastRunFn — in production this would query bead history.
	lastRunFn := func(_ string) (time.Time, error) { return time.Time{}, nil }
	return doPluginCheck(pp, time.Now(), lastRunFn, stdout)
}

// doPluginCheck evaluates gates for all plugins and prints a table.
// Returns 0 if any are due, 1 if none are due.
func doPluginCheck(pp []plugins.Plugin, now time.Time, lastRunFn plugins.LastRunFunc, stdout io.Writer) int {
	if len(pp) == 0 {
		fmt.Fprintln(stdout, "No plugins found.") //nolint:errcheck // best-effort stdout
		return 1
	}

	fmt.Fprintf(stdout, "%-20s %-12s %-5s %s\n", "NAME", "GATE", "DUE", "REASON") //nolint:errcheck
	anyDue := false
	for _, p := range pp {
		result := plugins.CheckGate(p, now, lastRunFn)
		due := "no"
		if result.Due {
			due = "yes"
			anyDue = true
		}
		fmt.Fprintf(stdout, "%-20s %-12s %-5s %s\n", p.Name, p.Gate, due, result.Reason) //nolint:errcheck
	}

	if anyDue {
		return 0
	}
	return 1
}

// --- gc plugin history ---

func cmdPluginHistory(name string, stdout, _ io.Writer) int {
	return doPluginHistory(name, stdout)
}

// doPluginHistory queries bead history for plugin runs and prints a table.
// For now, prints a placeholder — full implementation requires bead store
// label queries which will be wired when plugin execution tracking is added.
func doPluginHistory(name string, stdout io.Writer) int {
	if name != "" {
		fmt.Fprintf(stdout, "No plugin history for %q.\n", name) //nolint:errcheck
	} else {
		fmt.Fprintln(stdout, "No plugin history.") //nolint:errcheck
	}
	return 0
}

// findPlugin looks up a plugin by name.
func findPlugin(pp []plugins.Plugin, name string) (plugins.Plugin, bool) {
	for _, p := range pp {
		if p.Name == name {
			return p, true
		}
	}
	return plugins.Plugin{}, false
}
