package doctor

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	removeRetryAttempts = 10
	removeRetryDelay    = 50 * time.Millisecond
)

// retryRemoveAllForTest retries os.RemoveAll briefly to absorb a lingering
// embedded-dolt background writer that can hold files open a few dozen ms
// past the owning bd subprocess's apparent exit — which otherwise races
// t.TempDir()'s single-shot RemoveAll cleanup with an intermittent
// "directory not empty" error. Falls through silently on final failure so
// TempDir's own best-effort cleanup still gets the last word.
func retryRemoveAllForTest(t *testing.T, dir string) {
	t.Helper()
	retryRemoveAll(dir, os.RemoveAll, removeRetryAttempts, removeRetryDelay)
}

// retryRemoveAll calls remove(dir) until it reports success or the attempt
// budget runs out, pausing delay between tries.
func retryRemoveAll(dir string, remove func(string) error, attempts int, delay time.Duration) {
	for i := 0; i < attempts; i++ {
		if err := remove(dir); err == nil {
			return
		}
		time.Sleep(delay)
	}
}

// guardedTempDir returns a t.TempDir() whose removal is retried by
// retryRemoveAllForTest. Registering the cleanup after t.TempDir() has
// registered its own means LIFO ordering runs the retrying removal first,
// leaving TempDir's single-shot RemoveAll nothing to trip over. Every temp
// dir a bd subprocess writes into needs this, so bead ga-531fk — where the
// dir pinned as HOME was the one missing the guard and failed a Mac CI run
// after its test body had already passed — cannot repeat: the guard now
// comes with the directory instead of being a second line to remember.
func guardedTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Cleanup(func() { retryRemoveAllForTest(t, dir) })
	return dir
}

// testOwnedHome pins HOME to a fresh guarded temp dir for the duration of the
// test and returns it. bd's config precedence falls through, as a last
// resort, to $HOME/.beads/config.yaml, so only a test-owned HOME keeps a
// machine-level dolt.shared-server setting out of the bd subprocesses these
// tests spawn (ga-zxpfic). bd then writes $HOME/.beads/ itself, which is why
// that dir needs the same retrying removal as the working dir.
func testOwnedHome(t *testing.T) string {
	t.Helper()
	home := guardedTempDir(t)
	t.Setenv("HOME", home)
	return home
}

func TestRetryRemoveAllRetriesUntilRemovalSucceeds(t *testing.T) {
	calls := 0
	retryRemoveAll(t.TempDir(), func(string) error {
		calls++
		if calls < 3 {
			return errors.New("directory not empty")
		}
		return nil
	}, removeRetryAttempts, 0)
	if calls != 3 {
		t.Fatalf("remove called %d times, want 3 (two failures, then success)", calls)
	}
}

func TestRetryRemoveAllStopsAtItsAttemptBudget(t *testing.T) {
	calls := 0
	retryRemoveAll(t.TempDir(), func(string) error {
		calls++
		return errors.New("directory not empty")
	}, 4, 0)
	if calls != 4 {
		t.Fatalf("remove called %d times, want 4 (the attempt budget)", calls)
	}
}

// TestGuardedTempDirRemovesItsDirWhenTheTestEnds pins the structural half of
// the contract — the returned dir is test-scoped and gone once the owning
// test finishes. The retry half is covered by the retryRemoveAll tests above,
// since a single RemoveAll of an idle dir succeeds on the first attempt.
func TestGuardedTempDirRemovesItsDirWhenTheTestEnds(t *testing.T) {
	var dir string
	t.Run("guarded", func(t *testing.T) {
		dir = guardedTempDir(t)
		if err := os.WriteFile(filepath.Join(dir, "leftover"), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) after the subtest returned err=%v, want the dir removed", dir, err)
	}
}

func TestTestOwnedHomePinsHOMEToAGuardedTempDir(t *testing.T) {
	var home string
	t.Run("pinned", func(t *testing.T) {
		home = testOwnedHome(t)
		if got := os.Getenv("HOME"); got != home {
			t.Fatalf("HOME = %q, want the test-owned dir %q", got, home)
		}
		if err := os.MkdirAll(filepath.Join(home, ".beads"), 0o700); err != nil {
			t.Fatal(err)
		}
	})
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("Stat(%s) after the subtest returned err=%v, want the test-owned HOME removed", home, err)
	}
}
