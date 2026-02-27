//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/steveyegge/gascity/test/tmuxtest"
)

// gasTownAgent describes an agent for Gas Town integration tests.
// Extends agentConfig with pool, dir, and isolation settings.
type gasTownAgent struct {
	Name         string
	StartCommand string
	Dir          string // rig directory (required for worktree isolation)
	Isolation    string // "worktree" or ""
	Pool         *poolConfig
	Suspended    bool
}

// poolConfig mirrors config.PoolConfig for test setup.
type poolConfig struct {
	Min   int
	Max   int
	Check string
}

// setupGasTownCity creates a city from gastown-style config, starts it,
// and registers cleanup. Returns the city directory path.
func setupGasTownCity(t *testing.T, guard *tmuxtest.Guard, agents []gasTownAgent) string {
	t.Helper()

	var cityName string
	if guard != nil {
		cityName = guard.CityName()
	} else {
		cityName = uniqueCityName()
	}

	cityDir := filepath.Join(t.TempDir(), cityName)

	// gc init
	out, err := gc("", "init", cityDir)
	if err != nil {
		t.Fatalf("gc init failed: %v\noutput: %s", err, out)
	}

	// Write city.toml with gastown-style agents.
	writeGasTownToml(t, cityDir, cityName, agents)

	// gc start
	out, err = gc("", "start", cityDir)
	if err != nil {
		t.Fatalf("gc start failed: %v\noutput: %s", err, out)
	}

	t.Cleanup(func() {
		gc("", "stop", cityDir) //nolint:errcheck // best-effort cleanup
	})

	time.Sleep(200 * time.Millisecond)
	return cityDir
}

// setupGasTownCityNoGuard creates a Gas Town city without a tmux guard.
func setupGasTownCityNoGuard(t *testing.T, agents []gasTownAgent) string {
	t.Helper()
	return setupGasTownCity(t, nil, agents)
}

// writeGasTownToml writes a city.toml with gastown-style agents including
// pool config, dir, and isolation settings.
func writeGasTownToml(t *testing.T, cityDir, cityName string, agents []gasTownAgent) {
	t.Helper()

	var b strings.Builder
	fmt.Fprintf(&b, "[workspace]\nname = %s\n", quote(cityName))
	fmt.Fprintf(&b, "\n[daemon]\npatrol_interval = \"100ms\"\n")

	for _, a := range agents {
		fmt.Fprintf(&b, "\n[[agents]]\nname = %s\n", quote(a.Name))
		fmt.Fprintf(&b, "start_command = %s\n", quote(a.StartCommand))
		if a.Dir != "" {
			fmt.Fprintf(&b, "dir = %s\n", quote(a.Dir))
		}
		if a.Isolation != "" {
			fmt.Fprintf(&b, "isolation = %s\n", quote(a.Isolation))
		}
		if a.Suspended {
			fmt.Fprintf(&b, "suspended = true\n")
		}
		if a.Pool != nil {
			fmt.Fprintf(&b, "\n[agents.pool]\nmin = %d\nmax = %d\ncheck = %s\n",
				a.Pool.Min, a.Pool.Max, quote(a.Pool.Check))
		}
	}

	tomlPath := filepath.Join(cityDir, "city.toml")
	if err := os.WriteFile(tomlPath, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("writing city.toml: %v", err)
	}
}

// waitForBeadStatus polls until a bead reaches the expected status or times out.
func waitForBeadStatus(t *testing.T, cityDir, beadID, status string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, _ := bd(cityDir, "show", beadID)
		if strings.Contains(out, status) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	out, _ := bd(cityDir, "show", beadID)
	t.Fatalf("timed out waiting for bead %s to reach status %q:\n%s", beadID, status, out)
}

// waitForMail polls an agent's inbox until a message matching the pattern arrives.
func waitForMail(t *testing.T, cityDir, recipient, pattern string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		out, _ := gc(cityDir, "mail", "inbox", recipient)
		if strings.Contains(out, pattern) {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	out, _ := gc(cityDir, "mail", "inbox", recipient)
	t.Fatalf("timed out waiting for mail to %s matching %q:\n%s", recipient, pattern, out)
}

// createBead creates a bead and returns its ID.
func createBead(t *testing.T, cityDir, title string) string {
	t.Helper()
	out, err := bd(cityDir, "create", title)
	if err != nil {
		t.Fatalf("bd create %q failed: %v\noutput: %s", title, err, out)
	}
	return extractBeadID(t, out)
}

// claimBead assigns a bead to an agent.
func claimBead(t *testing.T, cityDir, agent, beadID string) {
	t.Helper()
	out, err := gc(cityDir, "agent", "claim", agent, beadID)
	if err != nil {
		t.Fatalf("gc agent claim %s %s failed: %v\noutput: %s", agent, beadID, err, out)
	}
}

// sendMail sends a message to a recipient.
func sendMail(t *testing.T, cityDir, to, body string) {
	t.Helper()
	out, err := gc(cityDir, "mail", "send", to, body)
	if err != nil {
		t.Fatalf("gc mail send %s %q failed: %v\noutput: %s", to, body, err, out)
	}
}

// verifyEvents checks that events of the given type exist in the event log.
func verifyEvents(t *testing.T, cityDir, eventType string) {
	t.Helper()
	out, err := gc(cityDir, "events", "--type", eventType)
	if err != nil {
		t.Fatalf("gc events --type %s failed: %v\noutput: %s", eventType, err, out)
	}
	if strings.Contains(out, "No events.") {
		t.Errorf("expected events of type %s, got 'No events.'", eventType)
	}
}

// setupBareGitRepo creates a bare git repo with an initial commit.
// Returns the path to the bare repo.
func setupBareGitRepo(t *testing.T) string {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "bare.git")

	cmds := []struct {
		dir  string
		args []string
	}{
		{"", []string{"git", "init", "--bare", bare}},
	}

	// Create a temp working dir to make the initial commit
	work := filepath.Join(t.TempDir(), "init-work")
	cmds = append(cmds,
		struct {
			dir  string
			args []string
		}{"", []string{"git", "clone", bare, work}},
		struct {
			dir  string
			args []string
		}{work, []string{"git", "config", "user.email", "test@test.com"}},
		struct {
			dir  string
			args []string
		}{work, []string{"git", "config", "user.name", "Test"}},
		struct {
			dir  string
			args []string
		}{work, []string{"touch", "README.md"}},
		struct {
			dir  string
			args []string
		}{work, []string{"git", "add", "README.md"}},
		struct {
			dir  string
			args []string
		}{work, []string{"git", "commit", "-m", "initial commit"}},
		struct {
			dir  string
			args []string
		}{work, []string{"git", "push", "origin", "main"}},
	)

	for _, c := range cmds {
		runGitCmd(t, c.dir, c.args...)
	}

	return bare
}

// setupWorkingRepo clones a bare repo and returns the working directory path.
func setupWorkingRepo(t *testing.T, bareRepo string) string {
	t.Helper()
	work := filepath.Join(t.TempDir(), "work")
	runGitCmd(t, "", "git", "clone", bareRepo, work)
	runGitCmd(t, work, "git", "config", "user.email", "test@test.com")
	runGitCmd(t, work, "git", "config", "user.name", "Test")
	return work
}

// runGitCmd runs a git command and fails the test if it errors.
func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("command %v failed: %v\noutput: %s", args, err, out)
	}
}
