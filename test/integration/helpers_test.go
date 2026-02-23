//go:build integration

package integration

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gascity/test/tmuxtest"
)

// agentConfig describes a single agent for setupCity.
type agentConfig struct {
	Name         string // agent name in city.toml
	StartCommand string // shell command (e.g., "sleep 3600", "bash /path/to/script.sh")
}

// usingSubprocess reports whether GC_SESSION=subprocess is set.
func usingSubprocess() bool {
	return os.Getenv("GC_SESSION") == "subprocess"
}

// uniqueCityName generates a random city name for test isolation.
func uniqueCityName() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic("generating random city name: " + err.Error())
	}
	return fmt.Sprintf("gctest-%x", b)
}

// setupCity creates a city directory, initializes it, writes a city.toml
// with the given agents, and runs gc start. Returns the city directory path.
// This is the general-purpose front-door setup for all integration tests.
//
// When using tmux, pass a non-nil guard for session cleanup. When using
// subprocess, guard may be nil (cleanup is via gc stop in t.Cleanup).
func setupCity(t *testing.T, guard *tmuxtest.Guard, agents []agentConfig) string {
	t.Helper()

	var cityName string
	if guard != nil {
		cityName = guard.CityName()
	} else {
		cityName = uniqueCityName()
	}

	cityDir := filepath.Join(t.TempDir(), cityName)

	// gc init — front door.
	out, err := gc("", "init", cityDir)
	if err != nil {
		t.Fatalf("gc init failed: %v\noutput: %s", err, out)
	}

	// Overwrite city.toml with our agent config.
	writeAgentsToml(t, cityDir, cityName, agents)

	// gc start — front door.
	out, err = gc("", "start", cityDir)
	if err != nil {
		t.Fatalf("gc start failed: %v\noutput: %s", err, out)
	}

	// Register cleanup: gc stop on test end.
	t.Cleanup(func() {
		gc("", "stop", cityDir) //nolint:errcheck // best-effort cleanup
	})

	// Give sessions a moment to register.
	time.Sleep(200 * time.Millisecond)

	return cityDir
}

// setupCityNoGuard creates a city without requiring a tmuxtest.Guard.
// Used by tests that work with any session provider.
func setupCityNoGuard(t *testing.T, agents []agentConfig) string {
	t.Helper()
	return setupCity(t, nil, agents)
}

// setupRunningCity creates a city directory, initializes it, writes a
// city.toml with start_command = "sleep 3600", and runs gc start.
// Returns the city directory path.
func setupRunningCity(t *testing.T, guard *tmuxtest.Guard) string {
	t.Helper()
	return setupCity(t, guard, []agentConfig{
		{Name: "mayor", StartCommand: "sleep 3600"},
	})
}

// writeAgentsToml writes a city.toml with the given agents.
func writeAgentsToml(t *testing.T, cityDir, cityName string, agents []agentConfig) {
	t.Helper()
	content := "[workspace]\nname = " + quote(cityName) + "\n"
	for _, a := range agents {
		content += fmt.Sprintf("\n[[agents]]\nname = %s\nstart_command = %s\n",
			quote(a.Name), quote(a.StartCommand))
	}
	tomlPath := filepath.Join(cityDir, "city.toml")
	if err := os.WriteFile(tomlPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing city.toml: %v", err)
	}
}

// agentScript returns the absolute path to a test agent script in test/agents/.
func agentScript(name string) string {
	root := findModuleRoot()
	return filepath.Join(root, "test", "agents", name)
}

// writeCityToml overwrites city.toml with a single mayor agent using the
// given start command. The city name is set to cityName.
func writeCityToml(t *testing.T, cityDir, cityName, startCommand string) {
	t.Helper()
	writeAgentsToml(t, cityDir, cityName, []agentConfig{
		{Name: "mayor", StartCommand: startCommand},
	})
}

// quote returns a TOML-safe quoted string.
func quote(s string) string {
	return "\"" + s + "\""
}
