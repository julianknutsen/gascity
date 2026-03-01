package demo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// demoDir returns the absolute path to the contrib/demo directory.
func demoDir(t *testing.T) string {
	t.Helper()
	// This test file lives in contrib/demo/, so use its location.
	// However, go test runs from the package directory, so "." works.
	dir, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolving demo dir: %v", err)
	}
	return dir
}

// shellScripts returns all .sh files in the demo directory.
func shellScripts(t *testing.T) []string {
	t.Helper()
	dir := demoDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading demo dir: %v", err)
	}
	var scripts []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sh") {
			scripts = append(scripts, filepath.Join(dir, e.Name()))
		}
	}
	if len(scripts) == 0 {
		t.Fatal("no .sh files found in demo directory")
	}
	return scripts
}

// TestDemoScripts_Shellcheck runs shellcheck on all demo scripts if available.
func TestDemoScripts_Shellcheck(t *testing.T) {
	shellcheck, err := exec.LookPath("shellcheck")
	if err != nil {
		t.Skip("shellcheck not installed, skipping")
	}

	for _, script := range shellScripts(t) {
		name := filepath.Base(script)
		t.Run(name, func(t *testing.T) {
			cmd := exec.Command(shellcheck,
				"-e", "SC1091", // source file not found (cross-script sourcing)
				"-e", "SC2034", // unused variable (often set for child scripts)
				"-e", "SC2012", // info: use find instead of ls
				"-e", "SC2119", // info: function args vs script args
				"-e", "SC2120", // warning: function references $1 but none passed (false positive for optional args)
				"-S", "warning", // only flag warnings and above
				script)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Errorf("shellcheck %s:\n%s", name, string(out))
			}
		})
	}
}

// tmuxSendKeysHardcodedTarget matches `tmux send-keys -t` followed by a
// literal string (not a $VARIABLE). We allow quoted variables like "$PANE_FOO".
var tmuxSendKeysHardcodedTarget = regexp.MustCompile(
	`tmux\s+send-keys\s+-t\s+([^$"\s]\S*)`,
)

// TestDemoScripts_TmuxPaneVariables verifies that tmux send-keys -t targets
// use $PANE_* variables rather than hardcoded pane names.
func TestDemoScripts_TmuxPaneVariables(t *testing.T) {
	for _, script := range shellScripts(t) {
		name := filepath.Base(script)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(script)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				// Skip comments and empty lines.
				if trimmed == "" || strings.HasPrefix(trimmed, "#") {
					continue
				}
				if matches := tmuxSendKeysHardcodedTarget.FindStringSubmatch(line); len(matches) > 1 {
					target := matches[1]
					// Allow session-level targets (e.g., "$DEMO_SESSION")
					// but flag hardcoded pane identifiers.
					if !strings.HasPrefix(target, "$") && !strings.HasPrefix(target, "\"$") {
						t.Errorf("%s:%d: tmux send-keys uses hardcoded target %q (use $PANE_* variable)", name, i+1, target)
					}
				}
			}
		})
	}
}

// TestDemoScripts_NoInvalidTmuxFlags checks for known-invalid tmux flags
// like --name (which doesn't exist in tmux).
func TestDemoScripts_NoInvalidTmuxFlags(t *testing.T) {
	invalidFlags := []string{"--name"}

	for _, script := range shellScripts(t) {
		name := filepath.Base(script)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(script)
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(string(data), "\n")
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" || strings.HasPrefix(trimmed, "#") {
					continue
				}
				if !strings.Contains(line, "tmux") {
					continue
				}
				for _, flag := range invalidFlags {
					if strings.Contains(line, flag) {
						t.Errorf("%s:%d: invalid tmux flag %q in: %s", name, i+1, flag, strings.TrimSpace(line))
					}
				}
			}
		})
	}
}

// TestDemoScripts_BdRigCwd checks that bd commands that need rig context
// use the rig variable ($DEMO_REPO or similar), not the city variable.
// Only applies to scripts that define DEMO_REPO (meaning they know about
// the rig/city distinction). Scripts without DEMO_REPO may be using the
// city root as the rig, which is valid for single-rig topologies.
func TestDemoScripts_BdRigCwd(t *testing.T) {
	// Pattern: `cd "$DEMO_CITY" && bd <subcommand>` where subcommand is
	// one that operates on rig beads (create, list, ready, update, close).
	rigBdCmds := regexp.MustCompile(
		`cd\s+"\$DEMO_CITY"\s*&&\s*bd\s+(create|list|ready|update|close)\b`,
	)
	definesRepo := regexp.MustCompile(`DEMO_REPO=`)

	for _, script := range shellScripts(t) {
		name := filepath.Base(script)
		t.Run(name, func(t *testing.T) {
			data, err := os.ReadFile(script)
			if err != nil {
				t.Fatal(err)
			}
			content := string(data)
			// Only check scripts that define DEMO_REPO — they know about
			// the rig/city distinction and should use the rig variable.
			if !definesRepo.MatchString(content) {
				return
			}
			lines := strings.Split(content, "\n")
			for i, line := range lines {
				trimmed := strings.TrimSpace(line)
				if trimmed == "" || strings.HasPrefix(trimmed, "#") {
					continue
				}
				if rigBdCmds.MatchString(line) {
					t.Errorf("%s:%d: bd command uses $DEMO_CITY cwd (should use rig dir): %s",
						name, i+1, strings.TrimSpace(line))
				}
			}
		})
	}
}

// TestDemoScripts_Executable verifies all .sh files have the executable bit set.
func TestDemoScripts_Executable(t *testing.T) {
	for _, script := range shellScripts(t) {
		name := filepath.Base(script)
		t.Run(name, func(t *testing.T) {
			info, err := os.Stat(script)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode()&0o111 == 0 {
				t.Errorf("%s is not executable (mode: %o)", name, info.Mode())
			}
		})
	}
}
