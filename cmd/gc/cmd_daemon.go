package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/fsys"
)

// newDaemonCmd creates the "gc daemon" command group with run, start, stop,
// status, logs, install, and uninstall subcommands.
func newDaemonCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the city daemon (background controller)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newDaemonRunCmd(stdout, stderr),
		newDaemonStartCmd(stdout, stderr),
		newDaemonStopCmd(stdout, stderr),
		newDaemonStatusCmd(stdout, stderr),
		newDaemonLogsCmd(stdout, stderr),
		newDaemonInstallCmd(stdout, stderr),
		newDaemonUninstallCmd(stdout, stderr),
	)
	return cmd
}

// newDaemonRunCmd creates the "gc daemon run" subcommand — foreground controller
// with log file output.
func newDaemonRunCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run [path]",
		Short: "Run the controller in the foreground (with log file)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if doDaemonRun(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&extraConfigFiles, "file", "f", nil,
		"additional config files to layer (can be repeated)")
	cmd.Flags().BoolVar(&strictMode, "strict", false,
		"promote config collision warnings to errors")
	return cmd
}

// doDaemonRun runs the controller in the foreground, tee-ing output to both
// stdout and .gc/daemon.log. Delegates to doStart with controllerMode=true.
func doDaemonRun(args []string, stdout, stderr io.Writer) int {
	dir, err := resolveDaemonDir(args)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon run: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Ensure .gc/ exists (auto-init will create it, but we need it for the log).
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "gc daemon run: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	logPath := filepath.Join(gcDir, "daemon.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon run: opening log: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	defer logFile.Close() //nolint:errcheck // best-effort cleanup

	logWriter := io.MultiWriter(stdout, logFile)
	return doStart(args, true, logWriter, stderr)
}

// newDaemonStartCmd creates the "gc daemon start" subcommand — background fork.
func newDaemonStartCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "start [path]",
		Short: "Start the daemon in the background",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if doDaemonStart(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doDaemonStart forks a background "gc daemon run" process.
func doDaemonStart(args []string, stdout, stderr io.Writer) int {
	dir, err := resolveDaemonDir(args)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Pre-check: try to acquire lock to see if a controller is already running.
	lock, err := acquireControllerLock(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	// Release immediately — the child will re-acquire.
	lock.Close() //nolint:errcheck // releasing pre-check lock

	gcPath, err := os.Executable()
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon start: finding executable: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	cmdArgs := []string{"--city", dir, "daemon", "run"}
	child := exec.Command(gcPath, cmdArgs...)
	child.SysProcAttr = daemonSysProcAttr()
	// Detach from parent stdio.
	child.Stdin = nil
	child.Stdout = nil
	child.Stderr = nil

	if err := child.Start(); err != nil {
		fmt.Fprintf(stderr, "gc daemon start: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	childPID := child.Process.Pid

	// Brief pause then verify the child took the lock.
	time.Sleep(200 * time.Millisecond)
	lock2, err := acquireControllerLock(dir)
	if err == nil {
		// Lock succeeded — child didn't start properly.
		lock2.Close()                                                                  //nolint:errcheck // cleanup
		fmt.Fprintf(stderr, "gc daemon start: child process failed to acquire lock\n") //nolint:errcheck // best-effort stderr
		return 1
	}

	// Verify PID file matches the child we spawned.
	pid := readDaemonPID(dir)
	if pid != 0 && pid != childPID {
		fmt.Fprintf(stderr, "gc daemon start: PID mismatch (expected %d, got %d)\n", childPID, pid) //nolint:errcheck // best-effort stderr
		return 1
	}

	fmt.Fprintf(stdout, "Daemon started (PID %d)\n", childPID) //nolint:errcheck // best-effort stdout
	return 0
}

// newDaemonStopCmd creates the "gc daemon stop" subcommand.
func newDaemonStopCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "stop [path]",
		Short: "Stop the running daemon",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if doDaemonStop(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doDaemonStop signals the running controller to shut down via its socket.
func doDaemonStop(args []string, stdout, stderr io.Writer) int {
	dir, err := resolveDaemonDir(args)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon stop: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon stop: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if !tryStopController(cityPath, stdout) {
		fmt.Fprintf(stderr, "gc daemon stop: no controller is running\n") //nolint:errcheck // best-effort stderr
		return 1
	}
	return 0
}

// newDaemonStatusCmd creates the "gc daemon status" subcommand.
func newDaemonStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "status [path]",
		Short: "Show daemon status (PID, uptime)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if doDaemonStatus(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doDaemonStatus shows whether the daemon is running, its PID, and uptime.
func doDaemonStatus(args []string, stdout, stderr io.Writer) int {
	dir, err := resolveDaemonDir(args)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	pid := readDaemonPID(cityPath)
	if pid == 0 || !isDaemonAlive(pid) {
		// Clean stale PID file if present.
		if pid != 0 {
			os.Remove(filepath.Join(cityPath, ".gc", "daemon.pid")) //nolint:errcheck // best-effort cleanup
		}
		fmt.Fprintln(stdout, "Daemon is not running") //nolint:errcheck // best-effort stdout
		return 1
	}

	// Derive uptime from the last controller.started event.
	uptime := "unknown"
	if started := lastControllerStarted(cityPath); !started.IsZero() {
		uptime = time.Since(started).Truncate(time.Second).String()
	}

	fmt.Fprintf(stdout, "Daemon is running (PID %d, uptime %s)\n", pid, uptime) //nolint:errcheck // best-effort stdout
	return 0
}

// newDaemonLogsCmd creates the "gc daemon logs" subcommand.
func newDaemonLogsCmd(stdout, stderr io.Writer) *cobra.Command {
	var numLines int
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs [path]",
		Short: "Tail the daemon log file",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if doDaemonLogs(args, numLines, follow, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&numLines, "lines", "n", 50, "number of lines to show")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	return cmd
}

// doDaemonLogs tails the daemon log file.
func doDaemonLogs(args []string, numLines int, follow bool, stdout, stderr io.Writer) int {
	dir, err := resolveDaemonDir(args)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon logs: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon logs: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	logPath := filepath.Join(cityPath, ".gc", "daemon.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		fmt.Fprintf(stderr, "gc daemon logs: log file not found: %s\n", logPath) //nolint:errcheck // best-effort stderr
		return 1
	}

	tailArgs := []string{"-n", strconv.Itoa(numLines)}
	if follow {
		tailArgs = append(tailArgs, "-f")
	}
	tailArgs = append(tailArgs, logPath)

	tailCmd := exec.Command("tail", tailArgs...)
	tailCmd.Stdout = stdout
	tailCmd.Stderr = stderr
	if err := tailCmd.Run(); err != nil {
		fmt.Fprintf(stderr, "gc daemon logs: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return 0
}

// newDaemonInstallCmd creates the "gc daemon install" subcommand.
func newDaemonInstallCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "install [path]",
		Short: "Install the daemon as a platform service (launchd/systemd)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if doDaemonInstall(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doDaemonInstall generates and loads a platform service file.
func doDaemonInstall(args []string, stdout, stderr io.Writer) int {
	dir, err := resolveDaemonDir(args)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon install: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon install: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	data, err := buildSupervisorData(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon install: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	switch runtime.GOOS {
	case "darwin":
		return installLaunchd(data, stdout, stderr)
	case "linux":
		return installSystemd(data, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "gc daemon install: not supported on %s\n", runtime.GOOS) //nolint:errcheck // best-effort stderr
		return 1
	}
}

// newDaemonUninstallCmd creates the "gc daemon uninstall" subcommand.
func newDaemonUninstallCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall [path]",
		Short: "Remove the platform service (launchd/systemd)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if doDaemonUninstall(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// doDaemonUninstall removes the platform service file and stops the service.
func doDaemonUninstall(args []string, stdout, stderr io.Writer) int {
	dir, err := resolveDaemonDir(args)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon uninstall: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(dir)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon uninstall: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	data, err := buildSupervisorData(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon uninstall: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	switch runtime.GOOS {
	case "darwin":
		return uninstallLaunchd(data, stdout, stderr)
	case "linux":
		return uninstallSystemd(data, stdout, stderr)
	default:
		fmt.Fprintf(stderr, "gc daemon uninstall: not supported on %s\n", runtime.GOOS) //nolint:errcheck // best-effort stderr
		return 1
	}
}

// --- Helpers ---

// resolveDaemonDir resolves the city directory from args or flags.
func resolveDaemonDir(args []string) (string, error) {
	switch {
	case len(args) > 0:
		return filepath.Abs(args[0])
	case cityFlag != "":
		return filepath.Abs(cityFlag)
	default:
		return os.Getwd()
	}
}

// readDaemonPID reads the PID from .gc/daemon.pid. Returns 0 if the file
// is missing, empty, or unparseable.
func readDaemonPID(cityPath string) int {
	data, err := os.ReadFile(filepath.Join(cityPath, ".gc", "daemon.pid"))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// lastControllerStarted scans events.jsonl for the most recent
// controller.started event and returns its timestamp. Returns zero time
// if not found or on error.
func lastControllerStarted(cityPath string) time.Time {
	eventsPath := filepath.Join(cityPath, ".gc", "events.jsonl")
	f, err := os.Open(eventsPath)
	if err != nil {
		return time.Time{}
	}
	defer f.Close() //nolint:errcheck // best-effort cleanup

	var last time.Time
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var ev struct {
			Type string    `json:"type"`
			Ts   time.Time `json:"ts"`
		}
		if json.Unmarshal(scanner.Bytes(), &ev) == nil && ev.Type == "controller.started" {
			last = ev.Ts
		}
	}
	return last
}

// supervisorData holds template variables for platform service files.
type supervisorData struct {
	GCPath   string
	CityRoot string
	CityName string
	SafeName string // sanitized for service file names
	LogPath  string
}

// buildSupervisorData resolves the data needed for supervisor templates.
func buildSupervisorData(cityPath string) (*supervisorData, error) {
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}

	gcPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("finding executable: %w", err)
	}

	return &supervisorData{
		GCPath:   gcPath,
		CityRoot: cityPath,
		CityName: cityName,
		SafeName: sanitizeServiceName(cityName),
		LogPath:  filepath.Join(cityPath, ".gc", "daemon.log"),
	}, nil
}

// sanitizeServiceName converts a city name to a safe string for use in
// service file names and identifiers. Lowercase, non-alnum replaced with
// hyphens, trimmed.
func sanitizeServiceName(name string) string {
	name = strings.ToLower(name)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	name = re.ReplaceAllString(name, "-")
	return strings.Trim(name, "-")
}

// --- Platform service templates ---

const launchdPlistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.gascity.daemon.{{.SafeName}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.GCPath}}</string>
        <string>--city</string>
        <string>{{.CityRoot}}</string>
        <string>daemon</string>
        <string>run</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <dict>
        <key>Crashed</key>
        <true/>
        <key>SuccessfulExit</key>
        <false/>
    </dict>
    <key>StandardOutPath</key>
    <string>{{.LogPath}}</string>
    <key>StandardErrorPath</key>
    <string>{{.LogPath}}</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>GC_CITY</key>
        <string>{{.CityRoot}}</string>
    </dict>
</dict>
</plist>
`

const systemdServiceTemplate = `[Unit]
Description=Gas City daemon for {{.CityName}}

[Service]
Type=simple
ExecStart={{.GCPath}} --city {{.CityRoot}} daemon run
Restart=always
RestartSec=5s
StandardOutput=append:{{.LogPath}}
StandardError=append:{{.LogPath}}
Environment=GC_CITY={{.CityRoot}}

[Install]
WantedBy=default.target
`

// renderTemplate renders a text/template to a string.
func renderTemplate(tmplStr string, data *supervisorData) (string, error) {
	tmpl, err := template.New("service").Parse(tmplStr)
	if err != nil {
		return "", err
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// launchdPlistPath returns the path for a launchd plist file.
func launchdPlistPath(safeName string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents",
		fmt.Sprintf("com.gascity.daemon.%s.plist", safeName))
}

// systemdServicePath returns the path for a systemd user service file.
func systemdServicePath(safeName string) string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "systemd", "user",
		fmt.Sprintf("gascity-daemon-%s.service", safeName))
}

// installLaunchd renders the plist and loads it via launchctl.
func installLaunchd(data *supervisorData, stdout, stderr io.Writer) int {
	content, err := renderTemplate(launchdPlistTemplate, data)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon install: rendering plist: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	plistPath := launchdPlistPath(data.SafeName)
	if err := os.MkdirAll(filepath.Dir(plistPath), 0o755); err != nil {
		fmt.Fprintf(stderr, "gc daemon install: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := os.WriteFile(plistPath, []byte(content), 0o644); err != nil {
		fmt.Fprintf(stderr, "gc daemon install: writing plist: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Unload first (ignore error — may not be loaded).
	exec.Command("launchctl", "unload", plistPath).Run() //nolint:errcheck // best-effort
	if err := exec.Command("launchctl", "load", plistPath).Run(); err != nil {
		fmt.Fprintf(stderr, "gc daemon install: launchctl load: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	fmt.Fprintf(stdout, "Installed launchd service: %s\n", plistPath) //nolint:errcheck // best-effort stdout
	return 0
}

// uninstallLaunchd unloads and removes the launchd plist.
func uninstallLaunchd(data *supervisorData, stdout, stderr io.Writer) int {
	plistPath := launchdPlistPath(data.SafeName)
	exec.Command("launchctl", "unload", plistPath).Run() //nolint:errcheck // best-effort
	if err := os.Remove(plistPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(stderr, "gc daemon uninstall: removing plist: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	fmt.Fprintf(stdout, "Uninstalled launchd service: %s\n", plistPath) //nolint:errcheck // best-effort stdout
	return 0
}

// installSystemd renders the unit file, reloads, enables, and starts.
func installSystemd(data *supervisorData, stdout, stderr io.Writer) int {
	content, err := renderTemplate(systemdServiceTemplate, data)
	if err != nil {
		fmt.Fprintf(stderr, "gc daemon install: rendering unit: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	unitPath := systemdServicePath(data.SafeName)
	if err := os.MkdirAll(filepath.Dir(unitPath), 0o755); err != nil {
		fmt.Fprintf(stderr, "gc daemon install: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := os.WriteFile(unitPath, []byte(content), 0o644); err != nil {
		fmt.Fprintf(stderr, "gc daemon install: writing unit: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	serviceName := fmt.Sprintf("gascity-daemon-%s.service", data.SafeName)
	for _, args := range [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", serviceName},
		{"--user", "start", serviceName},
	} {
		if err := exec.Command("systemctl", args...).Run(); err != nil {
			fmt.Fprintf(stderr, "gc daemon install: systemctl %s: %v\n", strings.Join(args, " "), err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	fmt.Fprintf(stdout, "Installed systemd service: %s\n", unitPath) //nolint:errcheck // best-effort stdout
	return 0
}

// uninstallSystemd stops, disables, removes, and reloads systemd.
func uninstallSystemd(data *supervisorData, stdout, stderr io.Writer) int {
	serviceName := fmt.Sprintf("gascity-daemon-%s.service", data.SafeName)
	unitPath := systemdServicePath(data.SafeName)

	// Stop and disable (ignore errors — may not be running/enabled).
	exec.Command("systemctl", "--user", "stop", serviceName).Run()    //nolint:errcheck // best-effort
	exec.Command("systemctl", "--user", "disable", serviceName).Run() //nolint:errcheck // best-effort

	if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(stderr, "gc daemon uninstall: removing unit: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	exec.Command("systemctl", "--user", "daemon-reload").Run() //nolint:errcheck // best-effort

	fmt.Fprintf(stdout, "Uninstalled systemd service: %s\n", unitPath) //nolint:errcheck // best-effort stdout
	return 0
}
