package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gastownhall/gascity/internal/beadmeta"
	"github.com/gastownhall/gascity/internal/beads"
	gitpkg "github.com/gastownhall/gascity/internal/git"
)

// gc submit is the polecat's refinery handoff as ONE atomic verb. The shell it
// replaces (mol-polecat-work submit-and-exit) carried three fail-closed gates —
// branch shape, push exit code, ls-remote verify — that an agent could skip on
// its way to the bead write that LOOKS like completion (metadata.branch set,
// assignee=refinery). Here the bead write is unreachable unless the pushed ref
// was read back from origin at the expected SHA, so "handed off" means "the
// branch is on origin" by construction rather than by the agent's cooperation.

type submitOptions struct {
	Dir        string
	BeadID     string
	ConvoyID   string
	BaseBranch string
	Refinery   string
}

type submitOps struct {
	ConvoyChildren     func(store beads.Store, convoyID string) ([]beads.Bead, error)
	RefineryConfigured func(name string) bool
	CurrentBranch      func(dir string) (string, error) // "" when HEAD is detached
	IsClean            func(dir string) (bool, error)
	FetchBase          func(dir, base string) error
	CommitsBeyond      func(dir, base string) (int, error)
	Head               func(dir string) (string, error)
	Push               func(dir, branch string) error
	LsRemote           func(dir, branch string) (string, error) // "" when the ref is absent
	DeleteLocalBranch  func(dir, branch string) error
	Wake               func(target string) error
	Nudge              func(target, message string) error
}

type submitResult struct {
	BeadID     string `json:"bead_id"`
	Branch     string `json:"branch"`
	Target     string `json:"target"`
	Refinery   string `json:"refinery,omitempty"`
	LocalSHA   string `json:"local_sha,omitempty"`
	RemoteSHA  string `json:"remote_sha,omitempty"`
	HandedOff  bool   `json:"handed_off"`
	Halted     bool   `json:"halted"`
	HaltReason string `json:"halt_reason,omitempty"`
}

const submitHaltReasonAutoPushFalse = "auto_push_false"

func newSubmitCmd(stdout, stderr io.Writer) *cobra.Command {
	var opts submitOptions
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "submit",
		Short: "Hand a polecat's work to the refinery atomically (push, verify on origin, then reassign)",
		Long: `Perform the polecat -> refinery handoff as one fail-closed operation.

In order: resolve the work bead (--bead, or the single open member of
--convoy); check the refinery target names a configured agent; assert HEAD is
on the expected branch (metadata.branch, else polecat/<bead>) and not detached;
assert the tree is clean; fetch the base and assert the branch has commits
beyond it; push; read the ref back from origin with ls-remote and require it
at the local HEAD. Only then does it write metadata.branch/target, clear
gc.routed_to and reassign the bead to the refinery. Any failure before that
point leaves the bead exactly as it was.

If the bead carries metadata.auto_push=false the verb halts at branch-ready
without pushing or reassigning, matching the previous formula contract.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cmd.SilenceUsage = true
			if opts.Dir == "" {
				dir, err := os.Getwd()
				if err != nil {
					fmt.Fprintf(stderr, "gc submit: %v\n", err) //nolint:errcheck
					return errExit
				}
				opts.Dir = dir
			}
			res, err := runSubmit(opts, stderr)
			if err != nil {
				fmt.Fprintf(stderr, "gc submit: %v\n", err) //nolint:errcheck
				return errExit
			}
			if jsonOutput {
				return json.NewEncoder(stdout).Encode(res)
			}
			if res.Halted {
				fmt.Fprintf(stdout, "halted at branch-ready: %s (%s, no push, no refinery handoff)\n", res.Branch, res.HaltReason) //nolint:errcheck
				return nil
			}
			fmt.Fprintf(stdout, "handed off %s: %s@%s verified on origin, assigned to %s\n", res.BeadID, res.Branch, res.RemoteSHA, res.Refinery) //nolint:errcheck
			return nil
		},
	}
	cmd.Flags().StringVar(&opts.BeadID, "bead", "", "work bead id (or use --convoy)")
	cmd.Flags().StringVar(&opts.ConvoyID, "convoy", "", "convoy whose single open member is the work bead")
	cmd.Flags().StringVar(&opts.BaseBranch, "base", "main", "base branch the refinery merges into (metadata.target)")
	cmd.Flags().StringVar(&opts.Refinery, "refinery", "", "qualified refinery agent name to assign the bead to (required)")
	cmd.Flags().StringVar(&opts.Dir, "dir", "", "worktree directory (default: cwd)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "JSON output")
	_ = cmd.MarkFlagRequired("refinery")
	return cmd
}

func runSubmit(opts submitOptions, stderr io.Writer) (submitResult, error) {
	cityPath, err := resolveCity()
	if err != nil {
		return submitResult{}, err
	}
	cfg, err := loadCityConfig(cityPath, stderr)
	if err != nil {
		return submitResult{}, err
	}
	store := hookClaimBdStore(opts.Dir, os.Environ(), "gc-submit")
	ops := submitOps{
		ConvoyChildren: func(s beads.Store, convoyID string) ([]beads.Bead, error) {
			return listConvoyChildren(s, convoyID, false)
		},
		RefineryConfigured: func(name string) bool {
			_, ok := findAgentByQualified(cfg, name)
			return ok
		},
		CurrentBranch: func(dir string) (string, error) {
			out, err := submitGit(dir, "rev-parse", "--abbrev-ref", "HEAD")
			if err != nil {
				return "", err
			}
			if out == "HEAD" {
				return "", nil
			}
			return out, nil
		},
		IsClean: func(dir string) (bool, error) {
			out, err := submitGit(dir, "status", "--porcelain")
			return out == "", err
		},
		FetchBase: func(dir, base string) error {
			_, err := submitGit(dir, "fetch", "origin", base)
			return err
		},
		CommitsBeyond: func(dir, base string) (int, error) {
			out, err := submitGit(dir, "rev-list", "--count", "origin/"+base+"..HEAD")
			if err != nil {
				return 0, err
			}
			var n int
			if _, err := fmt.Sscanf(out, "%d", &n); err != nil {
				return 0, fmt.Errorf("parsing rev-list count %q: %w", out, err)
			}
			return n, nil
		},
		Head: func(dir string) (string, error) { return submitGit(dir, "rev-parse", "HEAD") },
		Push: func(dir, branch string) error {
			_, err := submitGit(dir, "push", "origin", "HEAD:refs/heads/"+branch)
			return err
		},
		LsRemote: func(dir, branch string) (string, error) {
			out, err := submitGit(dir, "ls-remote", "origin", "refs/heads/"+branch)
			if err != nil {
				return "", err
			}
			fields := strings.Fields(out)
			if len(fields) == 0 {
				return "", nil
			}
			return fields[0], nil
		},
		DeleteLocalBranch: func(dir, branch string) error {
			if _, err := submitGit(dir, "checkout", "--detach"); err != nil {
				return err
			}
			_, err := submitGit(dir, "branch", "-D", branch)
			return err
		},
		Wake: func(target string) error {
			if cmdSessionWake([]string{target}, io.Discard, stderr) != 0 {
				return errors.New("gc session wake failed")
			}
			return nil
		},
		Nudge: func(target, message string) error {
			if cmdSessionNudge([]string{target, message}, nudgeDeliveryWaitIdle, false, io.Discard, stderr) != 0 {
				return errors.New("gc session nudge failed")
			}
			return nil
		},
	}
	return doSubmit(store, opts, ops, stderr)
}

// doSubmit is the fail-closed core. Every refusal returns before any store
// write; the only writes are the auto_push=false halt and the final handoff,
// and the handoff write is reachable solely through a successful ls-remote
// match against the local HEAD.
func doSubmit(store beads.Store, opts submitOptions, ops submitOps, stderr io.Writer) (submitResult, error) {
	if strings.TrimSpace(opts.Refinery) == "" {
		return submitResult{}, errors.New("--refinery is required")
	}
	base := strings.TrimSpace(opts.BaseBranch)
	if base == "" {
		return submitResult{}, errors.New("--base is required")
	}

	beadID, err := submitResolveBeadID(store, opts, ops)
	if err != nil {
		return submitResult{}, err
	}
	bead, err := store.Get(beadID)
	if err != nil {
		return submitResult{}, fmt.Errorf("reading work bead %s: %w", beadID, err)
	}

	if !ops.RefineryConfigured(opts.Refinery) {
		return submitResult{}, fmt.Errorf("refinery target %q does not name a configured agent; refusing to hand off (the bead would be stranded)", opts.Refinery)
	}

	expected := strings.TrimSpace(bead.Metadata["branch"])
	if expected == "" {
		expected = "polecat/" + beadID
	}
	if expected == base {
		return submitResult{}, fmt.Errorf("work branch %q is the base branch; refusing to hand off from the base branch", expected)
	}
	res := submitResult{BeadID: beadID, Branch: expected, Target: base, Refinery: opts.Refinery}

	current, err := ops.CurrentBranch(opts.Dir)
	if err != nil {
		return submitResult{}, fmt.Errorf("reading current branch: %w", err)
	}
	if current == "" {
		return submitResult{}, fmt.Errorf("HEAD is detached; the refinery merges %s, so check that branch out (or recreate it from origin/%s and cherry-pick) before submitting", expected, base)
	}
	if current != expected {
		return submitResult{}, fmt.Errorf("HEAD is on %q but the refinery merges %q (metadata.branch, else polecat/<bead>); refusing to hand off from another branch", current, expected)
	}

	clean, err := ops.IsClean(opts.Dir)
	if err != nil {
		return submitResult{}, fmt.Errorf("checking working tree: %w", err)
	}
	if !clean {
		return submitResult{}, errors.New("working tree has uncommitted or untracked changes; commit or discard them, then submit again")
	}

	if strings.EqualFold(strings.TrimSpace(bead.Metadata["auto_push"]), "false") {
		res.Halted = true
		res.HaltReason = submitHaltReasonAutoPushFalse
		statusOpen, noAssignee := "open", ""
		if err := store.Update(beadID, beads.UpdateOpts{
			Status:   &statusOpen,
			Assignee: &noAssignee,
			Metadata: map[string]string{
				"branch":                     expected,
				"target":                     base,
				"branch_ready":               "true",
				"halt_reason":                submitHaltReasonAutoPushFalse,
				beadmeta.RoutedToMetadataKey: "",
			},
		}); err != nil {
			return submitResult{}, fmt.Errorf("recording branch-ready halt on %s: %w", beadID, err)
		}
		return res, nil
	}

	if err := ops.FetchBase(opts.Dir, base); err != nil {
		return submitResult{}, fmt.Errorf("fetching origin/%s: %w", base, err)
	}
	ahead, err := ops.CommitsBeyond(opts.Dir, base)
	if err != nil {
		return submitResult{}, fmt.Errorf("counting commits beyond origin/%s: %w", base, err)
	}
	if ahead == 0 {
		return submitResult{}, fmt.Errorf("%s has no commits beyond origin/%s; nothing to hand off", expected, base)
	}
	head, err := ops.Head(opts.Dir)
	if err != nil {
		return submitResult{}, fmt.Errorf("reading HEAD: %w", err)
	}
	res.LocalSHA = head

	if err := ops.Push(opts.Dir, expected); err != nil {
		return submitResult{}, fmt.Errorf("push of %s to origin failed, bead stays with the polecat: %w", expected, err)
	}
	remote, err := ops.LsRemote(opts.Dir, expected)
	if err != nil {
		return submitResult{}, fmt.Errorf("verifying %s on origin after push: %w", expected, err)
	}
	if remote == "" {
		return submitResult{}, fmt.Errorf("push reported success but origin has no ref for %s; branch is local-only, refusing to hand off", expected)
	}
	if remote != head {
		return submitResult{}, fmt.Errorf("origin/%s is at %s but local HEAD is %s; refusing to hand off an unverified tip", expected, remote, head)
	}
	res.RemoteSHA = remote

	// The ONLY path to this write is a successful ls-remote match above.
	statusOpen := "open"
	refinery := opts.Refinery
	if err := store.Update(beadID, beads.UpdateOpts{
		Status:   &statusOpen,
		Assignee: &refinery,
		Metadata: map[string]string{
			"branch":                     expected,
			"target":                     base,
			beadmeta.RoutedToMetadataKey: "",
		},
	}); err != nil {
		return submitResult{}, fmt.Errorf("branch %s is verified on origin at %s but recording the handoff on %s failed: %w", expected, remote, beadID, err)
	}
	res.HandedOff = true

	if err := ops.DeleteLocalBranch(opts.Dir, expected); err != nil {
		fmt.Fprintf(stderr, "gc submit: warning: could not delete local branch %s after handoff: %v\n", expected, err) //nolint:errcheck
	}
	if err := ops.Wake(opts.Refinery); err != nil {
		fmt.Fprintf(stderr, "gc submit: warning: wake %s: %v (refinery finds the work on its next poll)\n", opts.Refinery, err) //nolint:errcheck
	}
	if err := ops.Nudge(opts.Refinery, "Run 'gc prime' to check merge queue and begin processing."); err != nil {
		fmt.Fprintf(stderr, "gc submit: warning: nudge %s: %v (refinery finds the work on its next poll)\n", opts.Refinery, err) //nolint:errcheck
	}
	return res, nil
}

func submitResolveBeadID(store beads.Store, opts submitOptions, ops submitOps) (string, error) {
	beadID := strings.TrimSpace(opts.BeadID)
	convoyID := strings.TrimSpace(opts.ConvoyID)
	switch {
	case beadID != "" && convoyID != "":
		return "", errors.New("pass --bead or --convoy, not both")
	case beadID != "":
		return beadID, nil
	case convoyID == "":
		return "", errors.New("--bead or --convoy is required")
	}
	members, err := ops.ConvoyChildren(store, convoyID)
	if err != nil {
		return "", fmt.Errorf("listing convoy %s members: %w", convoyID, err)
	}
	if len(members) != 1 {
		return "", fmt.Errorf("convoy %s has %d open members; pass --bead to name the work bead", convoyID, len(members))
	}
	return members[0].ID, nil
}

func submitGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitpkg.SanitizedEnv()
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if err != nil {
		if text != "" {
			return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, text)
		}
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return text, nil
}
