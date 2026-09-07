package scripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMakefileTestEnvIgnoresUserGitConfiguration(t *testing.T) {
	repoRoot := repoRoot(t)
	makefile, err := os.ReadFile(filepath.Join(repoRoot, "Makefile"))
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}

	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ".gitconfig"), []byte("[commit]\n\tgpgsign = true\n"), 0o644); err != nil {
		t.Fatalf("write poisoned global git config: %v", err)
	}

	// Exercise the actual xargs -> bash child and its env scrub without
	// launching a test suite. A source-only pin cannot catch an unexported
	// parent variable that disappears at this boundary.
	parallel := localParallelScript(t)
	setupStart := strings.Index(parallel, "# One shared seeded gitconfig")
	setupEnd := strings.Index(parallel, "# shellcheck source=lib/test-slice.sh")
	if setupStart < 0 || setupEnd <= setupStart {
		t.Fatal("cannot locate parallel runner gitconfig setup")
	}
	probeDir := t.TempDir()
	probe := filepath.Join(probeDir, "fanout.sh")
	probeScript := `#!/usr/bin/env bash
set -euo pipefail
repo_root="$1"
export LOCAL_TEST_LOG_DIR="$2"
gate_fd=""
local_jobs=1
export TEST_LOCAL_GOPATH="" TEST_LOCAL_GOCACHE="" TEST_LOCAL_GOMODCACHE=""
export TEST_LOCAL_GOTMPDIR="" TEST_LOCAL_GOROOT=""
` + parallel[setupStart:setupEnd] + `
printf -v probe_command 'test "$GIT_CONFIG_GLOBAL" = %q && test "$GIT_CONFIG_NOSYSTEM" = 1 && test "${GIT_DIR-unset}" = unset && test "$(git config --global --get beads.role)" = maintainer && printf "fanout-git-env-ok\n"' "$gc_test_gitconfig"
jobspecs=("git-env::$probe_command")
run_fan_out() {
` + shellFunctionBody(t, parallel, "run_fan_out") + `
}
run_fan_out
cat "$LOCAL_TEST_LOG_DIR/git-env.log"
`
	if err := os.WriteFile(probe, []byte(probeScript), 0o600); err != nil {
		t.Fatalf("write fanout probe: %v", err)
	}

	testMakefile := filepath.Join(t.TempDir(), "Makefile")
	content := string(makefile) + `
.PHONY: print-test-env-git
print-test-env-git:
	@$(TEST_ENV) sh -c 'printf "global=%s\nnosystem=%s\ngitdir=%s\ngpgsign=%s\n" "$$GIT_CONFIG_GLOBAL" "$$GIT_CONFIG_NOSYSTEM" "$${GIT_DIR-unset}" "$$(git config --global --get commit.gpgsign 2>/dev/null || printf unset)"'
	@bash "$(FANOUT_PROBE)" "$(CURDIR)" "$(FANOUT_LOG_DIR)"
`
	if err := os.WriteFile(testMakefile, []byte(content), 0o644); err != nil {
		t.Fatalf("write test Makefile: %v", err)
	}

	cmd := makeCommand("--no-print-directory", "-f", testMakefile, "print-test-env-git")
	cmd.Dir = repoRoot
	cmd.Env = []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"USER=" + os.Getenv("USER"),
		"SHELL=/bin/sh",
		"GIT_DIR=/poison/.git",
		"FANOUT_PROBE=" + probe,
		"FANOUT_LOG_DIR=" + probeDir,
		"TMPDIR=" + probeDir,
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("make print-test-env-git failed: %v\n%s", err, out)
	}
	for _, want := range []string{
		"nosystem=1",
		"gitdir=unset",
		"gpgsign=unset",
		"fanout-git-env-ok",
	} {
		if !strings.Contains(string(out), want+"\n") {
			t.Errorf("TEST_ENV output missing %q:\n%s", want, out)
		}
	}

	// The global config is a real, writable, seeded file outside the user's
	// HOME — not the user's own config and not an unwritable sentinel (a
	// /dev/null sentinel breaks ensure_beads_role's global write path).
	var globalPath string
	for _, line := range strings.Split(string(out), "\n") {
		if v, ok := strings.CutPrefix(line, "global="); ok {
			globalPath = v
		}
	}
	if globalPath == "" {
		t.Fatalf("TEST_ENV output missing global= line:\n%s", out)
	}
	if strings.HasPrefix(globalPath, home+string(os.PathSeparator)) {
		t.Errorf("GIT_CONFIG_GLOBAL %q resolves under the poisoned HOME %q", globalPath, home)
	}
	info, err := os.Stat(globalPath)
	if err != nil {
		t.Fatalf("GIT_CONFIG_GLOBAL %q does not exist: %v", globalPath, err)
	}
	if info.Mode().Perm()&0o200 == 0 {
		t.Errorf("GIT_CONFIG_GLOBAL %q is not writable (mode %v)", globalPath, info.Mode())
	}
}

func TestShardTestEnvsIgnoreUserGitConfiguration(t *testing.T) {
	repoRoot := repoRoot(t)
	for _, path := range []string{
		"scripts/test-local-parallel",
		"scripts/test-go-test-shard",
		"scripts/test-integration-shard",
	} {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join(repoRoot, path))
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			content := string(data)
			for _, pin := range []string{
				"GIT_CONFIG_NOSYSTEM=1",
				`GIT_CONFIG_GLOBAL="$gc_test_gitconfig"`,
				`gc_test_gitconfig="$("$repo_root/scripts/test-gitconfig-path")"`,
			} {
				if got := strings.Count(content, pin); got != 1 {
					t.Errorf("%s has %d occurrences of %q, want 1", path, got, pin)
				}
			}
		})
	}
}
