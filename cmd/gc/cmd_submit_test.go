package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
)

type submitTestHarness struct {
	store  *beads.MemStore
	ops    submitOps
	events []string
	branch string
	clean  bool
	ahead  int
	remote string
	head   string
}

func newSubmitTestHarness(t *testing.T, meta map[string]string) *submitTestHarness {
	t.Helper()
	h := &submitTestHarness{branch: "polecat/sys-1", clean: true, ahead: 2, head: "abc123", remote: "abc123"}
	h.store = beads.NewMemStoreFrom(1, []beads.Bead{{ID: "sys-1", Title: "work", Status: "in_progress", Assignee: "sysadmin/polecat-x", Metadata: meta}}, nil)
	rec := func(name string) { h.events = append(h.events, name) }
	h.ops = submitOps{
		ConvoyChildren: func(_ beads.Store, _ string) ([]beads.Bead, error) {
			b, _ := h.store.Get("sys-1")
			return []beads.Bead{b}, nil
		},
		RefineryConfigured: func(name string) bool { rec("refinery-check"); return name == "sysadmin/gastown.refinery" },
		CurrentBranch:      func(string) (string, error) { rec("current-branch"); return h.branch, nil },
		IsClean:            func(string) (bool, error) { rec("is-clean"); return h.clean, nil },
		FetchBase:          func(string, string) error { rec("fetch"); return nil },
		CommitsBeyond:      func(string, string) (int, error) { rec("commits-beyond"); return h.ahead, nil },
		Head:               func(string) (string, error) { return h.head, nil },
		Push:               func(string, string) error { rec("push"); return nil },
		LsRemote:           func(string, string) (string, error) { rec("ls-remote"); return h.remote, nil },
		DeleteLocalBranch:  func(string, string) error { rec("delete-branch"); return nil },
		Wake:               func(string) error { rec("wake"); return nil },
		Nudge:              func(string, string) error { rec("nudge"); return nil },
	}
	return h
}

func (h *submitTestHarness) run(t *testing.T, opts submitOptions) (submitResult, string, error) {
	t.Helper()
	if opts.Refinery == "" {
		opts.Refinery = "sysadmin/gastown.refinery"
	}
	if opts.BaseBranch == "" {
		opts.BaseBranch = "main"
	}
	if opts.BeadID == "" && opts.ConvoyID == "" {
		opts.BeadID = "sys-1"
	}
	var stderr bytes.Buffer
	res, err := doSubmit(h.store, opts, h.ops, &stderr)
	return res, stderr.String(), err
}

func (h *submitTestHarness) bead(t *testing.T) beads.Bead {
	t.Helper()
	b, err := h.store.Get("sys-1")
	if err != nil {
		t.Fatalf("get bead: %v", err)
	}
	return b
}

func (h *submitTestHarness) saw(name string) bool {
	for _, e := range h.events {
		if e == name {
			return true
		}
	}
	return false
}

func TestSubmitHappyPathWritesBeadOnlyAfterRemoteVerify(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	// Bead write is observed by the store; record its position relative to
	// push/ls-remote through the ops that bracket it.
	res, _, err := h.run(t, submitOptions{})
	if err != nil {
		t.Fatalf("doSubmit: %v", err)
	}
	b := h.bead(t)
	if b.Assignee != "sysadmin/gastown.refinery" || b.Status != "open" {
		t.Fatalf("bead not handed off: assignee=%q status=%q", b.Assignee, b.Status)
	}
	if b.Metadata["branch"] != "polecat/sys-1" || b.Metadata["target"] != "main" || b.Metadata["gc.routed_to"] != "" {
		t.Fatalf("metadata = %v", b.Metadata)
	}
	if !res.HandedOff || res.Branch != "polecat/sys-1" || res.RemoteSHA != "abc123" {
		t.Fatalf("result = %+v", res)
	}
	want := []string{"refinery-check", "current-branch", "is-clean", "fetch", "commits-beyond", "push", "ls-remote", "delete-branch", "wake", "nudge"}
	if got := strings.Join(h.events, ","); got != strings.Join(want, ",") {
		t.Fatalf("op order = %s, want %s", got, strings.Join(want, ","))
	}
}

func TestSubmitRefusesDetachedHead(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	h.branch = ""
	_, _, err := h.run(t, submitOptions{})
	if err == nil || !strings.Contains(err.Error(), "detached") {
		t.Fatalf("err = %v, want detached HEAD refusal", err)
	}
	assertSubmitUntouched(t, h)
}

func TestSubmitRefusesWrongBranch(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	h.branch = "main"
	_, _, err := h.run(t, submitOptions{})
	if err == nil || !strings.Contains(err.Error(), "polecat/sys-1") {
		t.Fatalf("err = %v, want branch-shape refusal naming the expected branch", err)
	}
	assertSubmitUntouched(t, h)
}

func TestSubmitHonoursPresetMetadataBranch(t *testing.T) {
	h := newSubmitTestHarness(t, map[string]string{"branch": "feature/custom"})
	h.branch = "feature/custom"
	res, _, err := h.run(t, submitOptions{})
	if err != nil {
		t.Fatalf("doSubmit: %v", err)
	}
	if res.Branch != "feature/custom" || h.bead(t).Metadata["branch"] != "feature/custom" {
		t.Fatalf("expected preset branch honored, got %+v", res)
	}
}

func TestSubmitRefusesBaseBranchAsWorkBranch(t *testing.T) {
	h := newSubmitTestHarness(t, map[string]string{"branch": "main"})
	h.branch = "main"
	_, _, err := h.run(t, submitOptions{})
	if err == nil || !strings.Contains(err.Error(), "base branch") {
		t.Fatalf("err = %v, want base-branch refusal", err)
	}
	assertSubmitUntouched(t, h)
}

func TestSubmitRefusesDirtyTree(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	h.clean = false
	_, _, err := h.run(t, submitOptions{})
	if err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Fatalf("err = %v, want dirty-tree refusal", err)
	}
	assertSubmitUntouched(t, h)
}

func TestSubmitRefusesNoCommitsBeyondBase(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	h.ahead = 0
	_, _, err := h.run(t, submitOptions{})
	if err == nil || !strings.Contains(err.Error(), "no commits") {
		t.Fatalf("err = %v, want no-commits refusal", err)
	}
	assertSubmitUntouched(t, h)
}

func TestSubmitRefusesUnconfiguredRefineryBeforePushing(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	_, _, err := h.run(t, submitOptions{Refinery: "sysadmin/refinery"})
	if err == nil || !strings.Contains(err.Error(), "sysadmin/refinery") {
		t.Fatalf("err = %v, want unconfigured-refinery refusal", err)
	}
	assertSubmitUntouched(t, h)
}

func TestSubmitPushFailureLeavesBeadUntouched(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	h.ops.Push = func(string, string) error { return errors.New("remote: permission denied") }
	_, _, err := h.run(t, submitOptions{})
	if err == nil || !strings.Contains(err.Error(), "push") {
		t.Fatalf("err = %v, want push failure", err)
	}
	assertSubmitUntouched(t, h)
}

func TestSubmitRemoteMismatchLeavesBeadUntouched(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	h.remote = "def456"
	_, _, err := h.run(t, submitOptions{})
	if err == nil || !strings.Contains(err.Error(), "def456") {
		t.Fatalf("err = %v, want remote-mismatch refusal", err)
	}
	if !h.saw("push") {
		t.Fatalf("push should have been attempted before verification")
	}
	assertSubmitUntouched(t, h)
}

func TestSubmitRemoteMissingLeavesBeadUntouched(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	h.remote = ""
	_, _, err := h.run(t, submitOptions{})
	if err == nil || !strings.Contains(err.Error(), "no ref") {
		t.Fatalf("err = %v, want remote-missing refusal", err)
	}
	assertSubmitUntouched(t, h)
}

func TestSubmitAutoPushFalseHaltsAtBranchReadyWithoutPushing(t *testing.T) {
	h := newSubmitTestHarness(t, map[string]string{"auto_push": "false", "gc.routed_to": "sysadmin/polecat"})
	res, _, err := h.run(t, submitOptions{})
	if err != nil {
		t.Fatalf("doSubmit: %v", err)
	}
	if res.HandedOff || !res.Halted {
		t.Fatalf("result = %+v, want halted", res)
	}
	if h.saw("push") || h.saw("ls-remote") {
		t.Fatalf("auto_push=false must not push: %v", h.events)
	}
	b := h.bead(t)
	if b.Assignee != "" || b.Status != "open" || b.Metadata["branch_ready"] != "true" || b.Metadata["halt_reason"] != "auto_push_false" || b.Metadata["gc.routed_to"] != "" || b.Metadata["branch"] != "polecat/sys-1" || b.Metadata["target"] != "main" {
		t.Fatalf("halt state wrong: assignee=%q status=%q meta=%v", b.Assignee, b.Status, b.Metadata)
	}
}

func TestSubmitResolvesSingleConvoyChild(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	res, _, err := h.run(t, submitOptions{ConvoyID: "sys-convoy"})
	if err != nil {
		t.Fatalf("doSubmit: %v", err)
	}
	if res.BeadID != "sys-1" {
		t.Fatalf("bead = %q", res.BeadID)
	}
}

func TestSubmitRefusesAmbiguousConvoy(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	h.ops.ConvoyChildren = func(_ beads.Store, _ string) ([]beads.Bead, error) {
		return []beads.Bead{{ID: "sys-1"}, {ID: "sys-2"}}, nil
	}
	_, _, err := h.run(t, submitOptions{ConvoyID: "sys-convoy"})
	if err == nil || !strings.Contains(err.Error(), "2 open members") {
		t.Fatalf("err = %v, want ambiguous-convoy refusal", err)
	}
	assertSubmitUntouched(t, h)
}

func TestSubmitWakeNudgeFailuresAreNonFatal(t *testing.T) {
	h := newSubmitTestHarness(t, nil)
	h.ops.Wake = func(string) error { return errors.New("no session") }
	h.ops.Nudge = func(string, string) error { return errors.New("no session") }
	res, stderr, err := h.run(t, submitOptions{})
	if err != nil || !res.HandedOff {
		t.Fatalf("handoff must succeed despite wake/nudge failure: err=%v res=%+v", err, res)
	}
	if !strings.Contains(stderr, "wake") {
		t.Fatalf("expected a wake warning on stderr, got %q", stderr)
	}
}

func assertSubmitUntouched(t *testing.T, h *submitTestHarness) {
	t.Helper()
	b := h.bead(t)
	if b.Assignee != "sysadmin/polecat-x" || b.Status != "in_progress" {
		t.Fatalf("bead was written despite refusal: assignee=%q status=%q", b.Assignee, b.Status)
	}
	if _, ok := b.Metadata["target"]; ok {
		t.Fatalf("metadata.target was written despite refusal: %v", b.Metadata)
	}
}
