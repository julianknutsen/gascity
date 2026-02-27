package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/agent"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/session"
)

func TestSanitizeServiceName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"bright-lights", "bright-lights"},
		{"My City", "my-city"},
		{"foo/bar@baz", "foo-bar-baz"},
		{"---hello---", "hello"},
		{"UPPER", "upper"},
		{"a1-b2_c3", "a1-b2-c3"},
		{"", ""},
	}
	for _, tt := range tests {
		got := sanitizeServiceName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeServiceName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestReadDaemonPID(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Missing PID file returns 0.
	if got := readDaemonPID(dir); got != 0 {
		t.Errorf("readDaemonPID (missing) = %d, want 0", got)
	}

	// Valid PID file.
	if err := os.WriteFile(filepath.Join(gcDir, "daemon.pid"), []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readDaemonPID(dir); got != 12345 {
		t.Errorf("readDaemonPID = %d, want 12345", got)
	}

	// PID with trailing newline.
	if err := os.WriteFile(filepath.Join(gcDir, "daemon.pid"), []byte("67890\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readDaemonPID(dir); got != 67890 {
		t.Errorf("readDaemonPID (newline) = %d, want 67890", got)
	}

	// Garbage content returns 0.
	if err := os.WriteFile(filepath.Join(gcDir, "daemon.pid"), []byte("not-a-pid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readDaemonPID(dir); got != 0 {
		t.Errorf("readDaemonPID (garbage) = %d, want 0", got)
	}
}

func TestReadDaemonPIDEmpty(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gcDir, "daemon.pid"), []byte(""), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := readDaemonPID(dir); got != 0 {
		t.Errorf("readDaemonPID (empty) = %d, want 0", got)
	}
}

func TestLastControllerStarted(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// No events file → zero time.
	if got := lastControllerStarted(dir); !got.IsZero() {
		t.Errorf("lastControllerStarted (no file) = %v, want zero", got)
	}

	// Write events with two controller.started events; should return the last one.
	ts1 := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	ts2 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	eventsPath := filepath.Join(gcDir, "events.jsonl")
	var buf bytes.Buffer
	for _, ev := range []struct {
		Type string    `json:"type"`
		Ts   time.Time `json:"ts"`
	}{
		{"controller.started", ts1},
		{"agent.started", time.Now()},
		{"controller.started", ts2},
	} {
		b, _ := json.Marshal(ev)
		buf.Write(b)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(eventsPath, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	got := lastControllerStarted(dir)
	if !got.Equal(ts2) {
		t.Errorf("lastControllerStarted = %v, want %v", got, ts2)
	}
}

func TestDoDaemonStatusNotRunning(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doDaemonStatus([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("doDaemonStatus (not running) code = %d, want 1", code)
	}
	if !strings.Contains(stdout.String(), "not running") {
		t.Errorf("expected 'not running' in stdout, got: %s", stdout.String())
	}
}

func TestDoDaemonStatusStalePIDCleanup(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a PID file for a process that doesn't exist.
	pidPath := filepath.Join(gcDir, "daemon.pid")
	if err := os.WriteFile(pidPath, []byte("999999999"), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doDaemonStatus([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("doDaemonStatus (stale PID) code = %d, want 1", code)
	}

	// Stale PID file should be cleaned up.
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("stale PID file should have been removed")
	}
}

func TestDoDaemonStopNoController(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doDaemonStop([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("doDaemonStop (no controller) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "no controller") {
		t.Errorf("expected 'no controller' in stderr, got: %s", stderr.String())
	}
}

func TestDoDaemonStartAlreadyRunning(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Hold the controller lock to simulate an already-running daemon.
	lock, err := acquireControllerLock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close() //nolint:errcheck // test cleanup

	var stdout, stderr bytes.Buffer
	code := doDaemonStart([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("doDaemonStart (already running) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "already running") {
		t.Errorf("expected 'already running' in stderr, got: %s", stderr.String())
	}
}

func TestDoDaemonLogsNoFile(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doDaemonLogs([]string{dir}, 50, false, &stdout, &stderr)
	if code != 1 {
		t.Errorf("doDaemonLogs (no file) code = %d, want 1", code)
	}
	if !strings.Contains(stderr.String(), "not found") {
		t.Errorf("expected 'not found' in stderr, got: %s", stderr.String())
	}
}

func TestDoDaemonLogsExistingFile(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(gcDir, "daemon.log")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(logPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doDaemonLogs([]string{dir}, 2, false, &stdout, &stderr)
	if code != 0 {
		t.Errorf("doDaemonLogs code = %d, want 0; stderr: %s", code, stderr.String())
	}
	// tail -n 2 should show the last 2 lines.
	if !strings.Contains(stdout.String(), "line2") || !strings.Contains(stdout.String(), "line3") {
		t.Errorf("expected line2 and line3 in output, got: %s", stdout.String())
	}
}

func TestRenderLaunchdTemplate(t *testing.T) {
	data := &supervisorData{
		GCPath:   "/usr/local/bin/gc",
		CityRoot: "/home/user/bright-lights",
		CityName: "bright-lights",
		SafeName: "bright-lights",
		LogPath:  "/home/user/bright-lights/.gc/daemon.log",
	}

	content, err := renderTemplate(launchdPlistTemplate, data)
	if err != nil {
		t.Fatal(err)
	}

	// Check key elements.
	checks := []string{
		"com.gascity.daemon.bright-lights",
		"/usr/local/bin/gc",
		"/home/user/bright-lights",
		"daemon",
		"run",
		"RunAtLoad",
		"KeepAlive",
		"Crashed",
		"GC_CITY",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("plist missing %q", check)
		}
	}

	// Should be valid XML (starts with <?xml).
	if !strings.HasPrefix(content, "<?xml") {
		t.Error("plist should start with <?xml")
	}
}

func TestRenderSystemdTemplate(t *testing.T) {
	data := &supervisorData{
		GCPath:   "/usr/local/bin/gc",
		CityRoot: "/home/user/bright-lights",
		CityName: "bright-lights",
		SafeName: "bright-lights",
		LogPath:  "/home/user/bright-lights/.gc/daemon.log",
	}

	content, err := renderTemplate(systemdServiceTemplate, data)
	if err != nil {
		t.Fatal(err)
	}

	checks := []string{
		"[Unit]",
		"[Service]",
		"[Install]",
		"Type=simple",
		"Restart=always",
		"RestartSec=5s",
		"ExecStart=/usr/local/bin/gc --city /home/user/bright-lights daemon run",
		"WantedBy=default.target",
		"GC_CITY=/home/user/bright-lights",
		"bright-lights",
	}
	for _, check := range checks {
		if !strings.Contains(content, check) {
			t.Errorf("systemd unit missing %q", check)
		}
	}
}

func TestLaunchdPlistPath(t *testing.T) {
	path := launchdPlistPath("bright-lights")
	if !strings.Contains(path, "LaunchAgents") {
		t.Errorf("expected LaunchAgents in path, got: %s", path)
	}
	if !strings.HasSuffix(path, "com.gascity.daemon.bright-lights.plist") {
		t.Errorf("unexpected plist filename: %s", path)
	}
}

func TestSystemdServicePath(t *testing.T) {
	path := systemdServicePath("bright-lights")
	if !strings.Contains(path, "systemd/user") {
		t.Errorf("expected systemd/user in path, got: %s", path)
	}
	if !strings.HasSuffix(path, "gascity-daemon-bright-lights.service") {
		t.Errorf("unexpected service filename: %s", path)
	}
}

func TestDoDaemonInstallUnsupportedOS(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "linux" {
		t.Skip("test only meaningful on unsupported OS")
	}
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write minimal city.toml.
	if err := os.WriteFile(filepath.Join(dir, "city.toml"),
		[]byte("[workspace]\nname = \"test\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := doDaemonInstall([]string{dir}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("doDaemonInstall (unsupported) code = %d, want 1", code)
	}
}

func TestResolveDaemonDir(t *testing.T) {
	dir := t.TempDir()

	// With explicit arg.
	got, err := resolveDaemonDir([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Errorf("resolveDaemonDir(%q) = %q, want %q", dir, got, dir)
	}

	// With no args — falls back to cwd.
	got, err = resolveDaemonDir(nil)
	if err != nil {
		t.Fatal(err)
	}
	cwd, _ := os.Getwd()
	if got != cwd {
		t.Errorf("resolveDaemonDir(nil) = %q, want cwd %q", got, cwd)
	}
}

func TestDaemonRunCreatesLogFile(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// doDaemonRun will fail (no city.toml) but should still create the log dir
	// and open the log file.
	var stdout, stderr bytes.Buffer
	_ = doDaemonRun([]string{dir}, &stdout, &stderr)

	logPath := filepath.Join(gcDir, "daemon.log")
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("daemon.log should have been created")
	}
}

func TestControllerWritesPIDFile(t *testing.T) {
	dir := t.TempDir()
	gcDir := filepath.Join(dir, ".gc")
	if err := os.MkdirAll(gcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Write city.toml.
	writeCityTOML(t, dir, "test", "worker")

	sp := session.NewFake()
	buildFn := func(_ *config.City) []agent.Agent {
		return []agent.Agent{agent.New("worker", "test", "echo hi", "", nil, agent.StartupHints{}, "", "", nil, sp)}
	}

	cfg := &config.City{
		Workspace: config.Workspace{Name: "test"},
		Agents:    []config.Agent{{Name: "worker", StartCommand: "echo hi"}},
		Daemon:    config.DaemonConfig{ShutdownTimeout: "0s"},
	}

	var stdout, stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		done <- runController(dir, "", cfg, buildFn, sp, nil, nil, nil, events.Discard, &stdout, &stderr)
	}()

	// Wait for PID file to appear.
	pidPath := filepath.Join(gcDir, "daemon.pid")
	deadline := time.After(3 * time.Second)
	for {
		if _, err := os.Stat(pidPath); err == nil {
			break
		}
		select {
		case <-deadline:
			t.Fatal("PID file not created within deadline")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	pid := readDaemonPID(dir)
	if pid != os.Getpid() {
		t.Errorf("PID file contains %d, want %d", pid, os.Getpid())
	}

	// Stop the controller.
	tryStopController(dir, &bytes.Buffer{})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("controller didn't stop")
	}

	// PID file should be cleaned up after shutdown.
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Error("PID file should be removed after controller shutdown")
	}
}

func TestStartForegroundFlag(t *testing.T) {
	// Verify that the --foreground flag exists on the start command.
	var stdout, stderr bytes.Buffer
	cmd := newStartCmd(&stdout, &stderr)
	fg := cmd.Flags().Lookup("foreground")
	if fg == nil {
		t.Fatal("--foreground flag not found on start command")
	}
	if fg.Usage == "" {
		t.Error("--foreground flag has no usage string")
	}

	// --controller should also exist (hidden alias).
	ctrl := cmd.Flags().Lookup("controller")
	if ctrl == nil {
		t.Fatal("--controller flag not found (backward compat)")
	}
	if !ctrl.Hidden {
		t.Error("--controller flag should be hidden")
	}
}
