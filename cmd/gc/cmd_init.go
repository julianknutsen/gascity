package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/dolt"
	"github.com/steveyegge/gascity/internal/fsys"
	"github.com/steveyegge/gascity/internal/hooks"
)

// wizardConfig carries the results of the interactive init wizard (or defaults
// for non-interactive paths). doInit uses it to decide which config to write.
type wizardConfig struct {
	interactive  bool   // true if the wizard ran with user interaction
	configName   string // "hello-world" or "custom"
	provider     string // "claude", "codex", "gemini", or "" if startCommand set
	startCommand string // custom start command (workspace-level)
}

// defaultWizardConfig returns a non-interactive wizardConfig that produces
// identical output to today — one mayor agent, no provider.
func defaultWizardConfig() wizardConfig {
	return wizardConfig{configName: "hello-world"}
}

// isTerminal reports whether f is connected to a terminal (not a pipe or file).
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// readLine reads a single line from br and returns it trimmed.
// Returns empty string on EOF or error.
func readLine(br *bufio.Reader) string {
	line, err := br.ReadString('\n')
	if err != nil {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(line)
}

// runWizard runs the interactive init wizard, asking the user to choose a
// config template and a coding agent provider. If stdin is nil, returns
// defaultWizardConfig() (non-interactive).
func runWizard(stdin io.Reader, stdout io.Writer) wizardConfig {
	if stdin == nil {
		return defaultWizardConfig()
	}

	br := bufio.NewReader(stdin)

	fmt.Fprintln(stdout, "Welcome to Gas City SDK!")                                   //nolint:errcheck // best-effort stdout
	fmt.Fprintln(stdout, "")                                                           //nolint:errcheck // best-effort stdout
	fmt.Fprintln(stdout, "Choose a config template:")                                  //nolint:errcheck // best-effort stdout
	fmt.Fprintln(stdout, "  1. hello-world  — single mayor agent (default)")           //nolint:errcheck // best-effort stdout
	fmt.Fprintln(stdout, "  2. custom       — empty workspace, configure it yourself") //nolint:errcheck // best-effort stdout
	fmt.Fprintf(stdout, "Template [1]: ")                                              //nolint:errcheck // best-effort stdout

	configChoice := readLine(br)
	configName := "hello-world"

	switch configChoice {
	case "", "1", "hello-world":
		configName = "hello-world"
	case "2", "custom":
		configName = "custom"
	default:
		fmt.Fprintf(stdout, "Unknown template %q, using hello-world.\n", configChoice) //nolint:errcheck // best-effort stdout
	}

	// Custom config → skip agent question, return minimal config.
	if configName == "custom" {
		return wizardConfig{
			interactive: true,
			configName:  "custom",
		}
	}

	// Build agent menu from built-in provider presets.
	order := config.BuiltinProviderOrder()
	builtins := config.BuiltinProviders()

	fmt.Fprintln(stdout, "")                          //nolint:errcheck // best-effort stdout
	fmt.Fprintln(stdout, "Choose your coding agent:") //nolint:errcheck // best-effort stdout
	for i, name := range order {
		spec := builtins[name]
		suffix := ""
		if i == 0 {
			suffix = "  (default)"
		}
		fmt.Fprintf(stdout, "  %d. %s%s\n", i+1, spec.DisplayName, suffix) //nolint:errcheck // best-effort stdout
	}
	customNum := len(order) + 1
	fmt.Fprintf(stdout, "  %d. Custom command\n", customNum) //nolint:errcheck // best-effort stdout
	fmt.Fprintf(stdout, "Agent [1]: ")                       //nolint:errcheck // best-effort stdout

	agentChoice := readLine(br)
	var provider, startCommand string

	provider = resolveAgentChoice(agentChoice, order, builtins, customNum)
	if provider == "" {
		// Custom command or invalid choice resolved to custom.
		switch {
		case agentChoice == fmt.Sprintf("%d", customNum) || agentChoice == "Custom command":
			fmt.Fprintf(stdout, "Enter start command: ") //nolint:errcheck // best-effort stdout
			startCommand = readLine(br)
		case agentChoice != "":
			fmt.Fprintf(stdout, "Unknown agent %q, using %s.\n", agentChoice, builtins[order[0]].DisplayName) //nolint:errcheck // best-effort stdout
			provider = order[0]
		default:
			provider = order[0]
		}
	}

	return wizardConfig{
		interactive:  true,
		configName:   "hello-world",
		provider:     provider,
		startCommand: startCommand,
	}
}

// resolveAgentChoice maps user input to a provider name. Input can be a
// number (1-based), a display name, or a provider key. Returns "" if the
// input doesn't match any built-in provider.
func resolveAgentChoice(input string, order []string, builtins map[string]config.ProviderSpec, _ int) string {
	if input == "" {
		return order[0]
	}
	// Check by number.
	n, err := strconv.Atoi(input)
	if err == nil && n >= 1 && n <= len(order) {
		return order[n-1]
	}
	// Check by display name or provider key.
	for _, name := range order {
		if input == builtins[name].DisplayName || input == name {
			return name
		}
	}
	return ""
}

func newInitCmd(stdout, stderr io.Writer) *cobra.Command {
	var fileFlag string
	cmd := &cobra.Command{
		Use:   "init [path]",
		Short: "Initialize a new city",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if fileFlag != "" {
				if cmdInitFromFile(fileFlag, args, stdout, stderr) != 0 {
					return errExit
				}
				return nil
			}
			if cmdInit(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&fileFlag, "file", "", "path to a TOML file to use as city.toml")
	return cmd
}

// cmdInit initializes a new city at the given path (or cwd if no path given).
// Runs the interactive wizard to choose a config template and provider.
// Creates .gc/, rigs/, and city.toml. If the bead provider is "bd", also
// runs bd init.
func cmdInit(args []string, stdout, stderr io.Writer) int {
	var cityPath string
	if len(args) > 0 {
		var err error
		cityPath, err = filepath.Abs(args[0])
		if err != nil {
			fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	} else {
		var err error
		cityPath, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}
	var wiz wizardConfig
	if isTerminal(os.Stdin) {
		wiz = runWizard(os.Stdin, stdout)
	} else {
		wiz = defaultWizardConfig()
	}
	cityName := filepath.Base(cityPath)
	if code := doInit(fsys.OSFS{}, cityPath, wiz, stdout, stderr); code != 0 {
		return code
	}
	if code := initBeads(cityPath, cityName, stderr); code != 0 {
		return code
	}
	// Install bd hooks so bead mutations emit Gas City events.
	if err := installBeadHooks(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc init: installing hooks: %v\n", err) //nolint:errcheck // best-effort stderr
	}
	return 0
}

// cmdInitFromFile initializes a city using the --file flag (non-interactive).
// The flag value is a path to a TOML file that is copied as the city's city.toml.
func cmdInitFromFile(fileArg string, args []string, stdout, stderr io.Writer) int {
	var cityPath string
	if len(args) > 0 {
		var err error
		cityPath, err = filepath.Abs(args[0])
		if err != nil {
			fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	} else {
		var err error
		cityPath, err = os.Getwd()
		if err != nil {
			fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	return cmdInitFromTOMLFile(fsys.OSFS{}, fileArg, cityPath, stdout, stderr)
}

// cmdInitFromTOMLFile initializes a city by copying a user-provided TOML
// file as city.toml. Creates .gc/, rigs/, prompts/, and runs bead init.
func cmdInitFromTOMLFile(fs fsys.FS, tomlSrc, cityPath string, stdout, stderr io.Writer) int {
	// Validate the source file parses as a valid city config.
	data, err := os.ReadFile(tomlSrc)
	if err != nil {
		fmt.Fprintf(stderr, "gc init: reading %q: %v\n", tomlSrc, err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Parse(data)
	if err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Override workspace name with the directory name.
	cityName := filepath.Base(cityPath)
	cfg.Workspace.Name = cityName

	// Re-marshal so the name is updated.
	content, err := cfg.Marshal()
	if err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Create directory structure.
	gcDir := filepath.Join(cityPath, ".gc")
	if _, err := fs.Stat(gcDir); err == nil {
		fmt.Fprintln(stderr, "gc init: already initialized") //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := fs.MkdirAll(gcDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := fs.MkdirAll(filepath.Join(cityPath, "rigs"), 0o755); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Install Claude Code hooks (settings.json).
	if code := installClaudeHooks(fs, cityPath); code != 0 {
		return code
	}

	// Write default prompts.
	if code := writeDefaultPrompts(fs, cityPath, stderr); code != 0 {
		return code
	}

	// Write default formulas.
	if code := writeDefaultFormulas(fs, cityPath, stderr); code != 0 {
		return code
	}

	// Materialize formula symlinks so bd finds them immediately after init.
	formulasInitDir := filepath.Join(cityPath, ".gc", "formulas")
	if rfErr := ResolveFormulas(cityPath, []string{formulasInitDir}); rfErr != nil {
		fmt.Fprintf(stderr, "gc init: resolving formulas: %v\n", rfErr) //nolint:errcheck // best-effort stderr
	}

	// Write city.toml.
	if err := fs.WriteFile(filepath.Join(cityPath, "city.toml"), content, 0o644); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	fmt.Fprintf(stdout, "Welcome to Gas City!\n")                                           //nolint:errcheck // best-effort stdout
	fmt.Fprintf(stdout, "Initialized city %q from %s.\n", cityName, filepath.Base(tomlSrc)) //nolint:errcheck // best-effort stdout
	if code := initBeads(cityPath, cityName, stderr); code != 0 {
		return code
	}
	// Install bd hooks so bead mutations emit Gas City events.
	if err := installBeadHooks(cityPath); err != nil {
		fmt.Fprintf(stderr, "gc init: installing hooks: %v\n", err) //nolint:errcheck // best-effort stderr
	}
	return 0
}

// doInit is the pure logic for "gc init". It creates the city directory
// structure (.gc/, rigs/) and writes city.toml. When wiz.interactive is true,
// uses WizardCity (one agent + provider); otherwise uses DefaultCity (one
// mayor, no provider). Errors if .gc/ already exists. Accepts an injected FS
// for testability.
func doInit(fs fsys.FS, cityPath string, wiz wizardConfig, stdout, stderr io.Writer) int {
	gcDir := filepath.Join(cityPath, ".gc")

	// Check if already initialized.
	if _, err := fs.Stat(gcDir); err == nil {
		fmt.Fprintln(stderr, "gc init: already initialized") //nolint:errcheck // best-effort stderr
		return 1
	}

	// Create directory structure.
	if err := fs.MkdirAll(gcDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := fs.MkdirAll(filepath.Join(cityPath, "rigs"), 0o755); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Install Claude Code hooks (settings.json).
	if code := installClaudeHooks(fs, cityPath); code != 0 {
		return code
	}

	// Write default prompt files.
	if code := writeDefaultPrompts(fs, cityPath, stderr); code != 0 {
		return code
	}

	// Write default formula files.
	if code := writeDefaultFormulas(fs, cityPath, stderr); code != 0 {
		return code
	}

	// Materialize formula symlinks so bd finds them immediately after init.
	formulasDir := filepath.Join(cityPath, ".gc", "formulas")
	if err := ResolveFormulas(cityPath, []string{formulasDir}); err != nil {
		fmt.Fprintf(stderr, "gc init: resolving formulas: %v\n", err) //nolint:errcheck // best-effort stderr
	}

	// Write city.toml — wizard path gets one agent + provider/startCommand;
	// non-interactive path gets one mayor + no provider (backwards compat);
	// custom path gets one mayor + no provider (user configures manually).
	cityName := filepath.Base(cityPath)
	var cfg config.City
	switch {
	case !wiz.interactive, wiz.configName == "custom":
		cfg = config.DefaultCity(cityName)
	default:
		cfg = config.WizardCity(cityName, wiz.provider, wiz.startCommand)
	}
	content, err := cfg.Marshal()
	if err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	tomlPath := filepath.Join(cityPath, "city.toml")
	if err := fs.WriteFile(tomlPath, content, 0o644); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	if wiz.interactive {
		fmt.Fprintf(stdout, "Created %s config (Level 1) in %q.\n", wiz.configName, cityName) //nolint:errcheck // best-effort stdout
	} else {
		fmt.Fprintln(stdout, "Welcome to Gas City!")                                     //nolint:errcheck // best-effort stdout
		fmt.Fprintf(stdout, "Initialized city %q with default mayor agent.\n", cityName) //nolint:errcheck // best-effort stdout
	}
	return 0
}

// installClaudeHooks writes Claude Code hook settings for the city.
// Delegates to hooks.Install which is idempotent (won't overwrite existing files).
func installClaudeHooks(fs fsys.FS, cityPath string) int {
	if err := hooks.Install(fs, cityPath, cityPath, []string{"claude"}); err != nil {
		return 1
	}
	return 0
}

// writeDefaultPrompts creates the prompts/ directory and writes all
// embedded prompt files used across the tutorials.
func writeDefaultPrompts(fs fsys.FS, cityPath string, stderr io.Writer) int {
	promptsDir := filepath.Join(cityPath, "prompts")
	if err := fs.MkdirAll(promptsDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	for _, name := range []string{"mayor.md", "worker.md", "one-shot.md", "loop.md", "loop-mail.md", "pool-worker.md"} {
		data, err := defaultPrompts.ReadFile("prompts/" + name)
		if err != nil {
			fmt.Fprintf(stderr, "gc init: reading embedded %s: %v\n", name, err) //nolint:errcheck // best-effort stderr
			return 1
		}
		dst := filepath.Join(promptsDir, name)
		if err := fs.WriteFile(dst, data, 0o644); err != nil {
			fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}
	return 0
}

// writeDefaultFormulas creates the .gc/formulas/ directory and writes
// embedded example formula files used across the tutorials.
func writeDefaultFormulas(fs fsys.FS, cityPath string, stderr io.Writer) int {
	formulasDir := filepath.Join(cityPath, ".gc", "formulas")
	if err := fs.MkdirAll(formulasDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	for _, name := range []string{"pancakes.formula.toml", "cooking.formula.toml"} {
		data, err := defaultFormulas.ReadFile("formulas/" + name)
		if err != nil {
			fmt.Fprintf(stderr, "gc init: reading embedded %s: %v\n", name, err) //nolint:errcheck // best-effort stderr
			return 1
		}
		dst := filepath.Join(formulasDir, name)
		if err := fs.WriteFile(dst, data, 0o644); err != nil {
			fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}
	return 0
}

// initBeads initializes the beads database if the provider is "bd".
// For bd provider, sets up dolt server and runs bd init --server.
// Skips if provider is not "bd" or if GC_DOLT=skip.
// Returns 0 on success, 1 on failure.
func initBeads(cityPath, cityName string, stderr io.Writer) int {
	provider := beadsProvider(cityPath)
	if provider != "bd" {
		return 0
	}

	if os.Getenv("GC_DOLT") == "skip" {
		return 0
	}

	if err := dolt.InitCity(cityPath, cityName, stderr); err != nil {
		fmt.Fprintf(stderr, "gc init: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return 0
}
