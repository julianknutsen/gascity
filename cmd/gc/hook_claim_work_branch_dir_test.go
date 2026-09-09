package main

import (
	"bytes"
	"context"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
)

// workBranchDirSpy records the directory a claim asks ResolveWorkBranch about.
type workBranchDirSpy struct{ dir string }

func (s *workBranchDirSpy) resolve(dir string) string {
	s.dir = dir
	return "gp-1"
}

func workBranchDirClaimOps(spy *workBranchDirSpy) hookClaimOps {
	return hookClaimOps{
		Runner: func(string, string) (string, error) {
			return `[{"id":"gp-1","status":"open","metadata":{"gc.routed_to":"worker"}}]`, nil
		},
		Claim: func(_ context.Context, _ string, _ []string, id, assignee string) (beads.Bead, bool, error) {
			return beads.Bead{ID: id, Status: "in_progress", Assignee: assignee, Metadata: map[string]string{"gc.routed_to": "worker"}}, true, nil
		},
		ResolveWorkBranch: spy.resolve,
		StampWorkMeta:     noopStampWorkMeta,
		PublishRunMap:     noopPublishRunMap,
	}
}

// A session that starts in its own worktree (agent work_dir) exports GC_DIR;
// the claim must read the work branch there, not from the store dir, which for
// a rig-scoped bead is the rig root and usually sits on main. Before the fix a
// worker whose lane was on gp-1 got gc.work_branch=main stamped.
func TestDoHookClaimResolvesWorkBranchFromSessionWorkDir(t *testing.T) {
	storeDir := t.TempDir()
	sessionDir := t.TempDir()
	missingDir := storeDir + "/does-not-exist"

	cases := map[string]struct {
		env  []string
		want string
	}{
		"GC_DIR names the session worktree": {
			env:  []string{"GC_SESSION_ID=s1", "GC_DIR=" + sessionDir},
			want: sessionDir,
		},
		"no GC_DIR falls back to the store dir": {
			env:  []string{"GC_SESSION_ID=s1"},
			want: storeDir,
		},
		"GC_DIR that does not exist falls back to the store dir": {
			env:  []string{"GC_SESSION_ID=s1", "GC_DIR=" + missingDir},
			want: storeDir,
		},
		"empty GC_DIR falls back to the store dir": {
			env:  []string{"GC_SESSION_ID=s1", "GC_DIR="},
			want: storeDir,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			spy := &workBranchDirSpy{}
			opts := hookClaimOptions{
				Assignee:           "s1",
				IdentityCandidates: []string{"s1"},
				RouteTargets:       []string{"worker"},
				Env:                tc.env,
				JSON:               true,
			}
			var stdout, stderr bytes.Buffer
			if code := doHookClaim("bd ready --json", storeDir, opts, workBranchDirClaimOps(spy), &stdout, &stderr); code != 0 {
				t.Fatalf("doHookClaim = %d, want 0; stderr=%s", code, stderr.String())
			}
			if spy.dir != tc.want {
				t.Fatalf("ResolveWorkBranch dir = %q, want %q", spy.dir, tc.want)
			}
		})
	}
}
