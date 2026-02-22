//go:build integration

package integration

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gascity/test/tmuxtest"
)

// setupRunningCity creates a city directory, initializes it, writes a
// city.toml with start_command = "sleep 3600", and runs gc start.
// Returns the city directory path. The guard's t.Cleanup handles session
// teardown even if the test fails.
func setupRunningCity(t *testing.T, guard *tmuxtest.Guard) string {
	t.Helper()
	cityDir := filepath.Join(t.TempDir(), guard.CityName())

	// gc init creates .gc/ and city.toml with default mayor.
	out, err := gc("", "init", cityDir)
	if err != nil {
		t.Fatalf("gc init failed: %v\noutput: %s", err, out)
	}

	// Overwrite city.toml with our test config.
	writeCityToml(t, cityDir, guard.CityName(), "sleep 3600")

	// Start the city — creates real tmux session.
	out, err = gc("", "start", cityDir)
	if err != nil {
		t.Fatalf("gc start failed: %v\noutput: %s", err, out)
	}

	// Give tmux a moment to fully register the session.
	time.Sleep(200 * time.Millisecond)

	return cityDir
}

// writeCityToml overwrites city.toml with a single mayor agent using the
// given start command. The city name is set to cityName.
func writeCityToml(t *testing.T, cityDir, cityName, startCommand string) {
	t.Helper()
	content := "[workspace]\nname = " + quote(cityName) + "\n\n" +
		"[[agents]]\nname = \"mayor\"\nstart_command = " + quote(startCommand) + "\n"
	tomlPath := filepath.Join(cityDir, "city.toml")
	if err := os.WriteFile(tomlPath, []byte(content), 0o644); err != nil {
		t.Fatalf("writing city.toml: %v", err)
	}
}

// quote returns a TOML-safe quoted string.
func quote(s string) string {
	return "\"" + s + "\""
}
