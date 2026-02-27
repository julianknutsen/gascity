package main

import (
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/plugins"
)

func newPluginCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage plugins (periodic formula dispatch)",
		Long: `Manage plugins — formulas with gate conditions for periodic dispatch.

Plugins are formulas annotated with scheduling gates (interval, cron
schedule, or shell check commands). The controller evaluates gates
periodically and dispatches plugin formulas when they are due.`,
		Args: cobra.ArbitraryArgs,
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
		Long: `List all available plugins with their gate type, schedule, and target pool.

Scans formula layers for formulas that have plugin metadata
(gate, interval, schedule, check, pool).`,
		Args: cobra.NoArgs,
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
		Long: `Display detailed information about a named plugin.

Shows the plugin name, description, formula reference, gate type,
scheduling parameters, check command, target pool, and source file.`,
		Args: cobra.ExactArgs(1),
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
		Long: `Execute a plugin manually, bypassing its gate conditions.

Instantiates a wisp from the plugin's formula and routes it to the
target pool (if configured). Useful for testing plugins or triggering
them outside their normal schedule.`,
		Args: cobra.ExactArgs(1),
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
		Long: `Evaluate gate conditions for all plugins and show which are due.

Prints a table with each plugin's gate, due status, and reason. Returns
exit code 0 if any plugin is due, 1 if none are due.`,
		Args: cobra.NoArgs,
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
		Long: `Show execution history for plugins.

Queries bead history for past plugin runs. Optionally filter by plugin
name.`,
		Args: cobra.MaximumNArgs(1),
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
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc plugin run: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	store := beads.NewBdStore(cityPath, beads.ExecCommandRunner())
	return doPluginRun(pp, name, shellSlingRunner, store, stdout, stderr)
}

// doPluginRun executes a plugin manually: instantiates a wisp from the
// plugin's formula and routes it to the target pool.
func doPluginRun(pp []plugins.Plugin, name string, runner SlingRunner, store *beads.BdStore, stdout, stderr io.Writer) int {
	p, ok := findPlugin(pp, name)
	if !ok {
		fmt.Fprintf(stderr, "gc plugin run: plugin %q not found\n", name) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Instantiate wisp from formula.
	rootID, err := instantiateWisp(p.Formula, "", nil, store)
	if err != nil {
		fmt.Fprintf(stderr, "gc plugin run: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Label with plugin-run:<name> for tracking, plus pool routing if specified.
	routeCmd := fmt.Sprintf("bd update %s --label=plugin-run:%s", rootID, name)
	if p.Pool != "" {
		routeCmd += fmt.Sprintf(" --label=pool:%s", p.Pool)
	}
	if _, err := runner(routeCmd); err != nil {
		fmt.Fprintf(stderr, "gc plugin run: labeling wisp: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
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

	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc plugin check: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	store := beads.NewBdStore(cityPath, beads.ExecCommandRunner())
	lastRunFn := pluginLastRunFn(store)
	return doPluginCheck(pp, time.Now(), lastRunFn, stdout)
}

// pluginLastRunFn returns a LastRunFunc that queries BdStore for the most
// recent bead labeled plugin-run:<name>. Returns zero time if never run.
func pluginLastRunFn(store *beads.BdStore) plugins.LastRunFunc {
	return func(name string) (time.Time, error) {
		label := "plugin-run:" + name
		results, err := store.ListByLabel(label, 1)
		if err != nil {
			return time.Time{}, err
		}
		if len(results) == 0 {
			return time.Time{}, nil
		}
		return results[0].CreatedAt, nil
	}
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

func cmdPluginHistory(name string, stdout, stderr io.Writer) int {
	pp, code := loadPlugins(stderr, "gc plugin history")
	if code != 0 {
		return code
	}
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc plugin history: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	store := beads.NewBdStore(cityPath, beads.ExecCommandRunner())
	return doPluginHistory(name, pp, store, stdout)
}

// doPluginHistory queries bead history for plugin runs and prints a table.
// When name is empty, shows history for all plugins. When name is given,
// filters to that plugin only.
func doPluginHistory(name string, pp []plugins.Plugin, store *beads.BdStore, stdout io.Writer) int {
	// Filter plugins if name specified.
	targets := pp
	if name != "" {
		targets = nil
		for _, p := range pp {
			if p.Name == name {
				targets = append(targets, p)
				break
			}
		}
	}

	type historyEntry struct {
		plugin string
		id     string
		time   string
	}
	var entries []historyEntry

	for _, p := range targets {
		label := "plugin-run:" + p.Name
		results, err := store.ListByLabel(label, 0)
		if err != nil {
			continue
		}
		for _, b := range results {
			entries = append(entries, historyEntry{
				plugin: p.Name,
				id:     b.ID,
				time:   b.CreatedAt.Format(time.RFC3339),
			})
		}
	}

	if len(entries) == 0 {
		if name != "" {
			fmt.Fprintf(stdout, "No plugin history for %q.\n", name) //nolint:errcheck
		} else {
			fmt.Fprintln(stdout, "No plugin history.") //nolint:errcheck
		}
		return 0
	}

	fmt.Fprintf(stdout, "%-20s %-15s %s\n", "PLUGIN", "WISP", "EXECUTED") //nolint:errcheck
	for _, e := range entries {
		fmt.Fprintf(stdout, "%-20s %-15s %s\n", e.plugin, e.id, e.time) //nolint:errcheck
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
