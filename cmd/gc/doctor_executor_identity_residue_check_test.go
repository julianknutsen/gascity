package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/doctor"
)

func TestExecutorIdentityResidueCheckFlagsAndFixesStaleStamp(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "stale stamp", Type: "task", Status: "open", Metadata: map[string]string{
			"gc.routed_to":    "gascity/reviewer",
			"gc.session_name": "gascity--builder",
			"gc.work_dir":     "/worktrees/gascity/builder-1",
			"work_dir":        "/legacy/worktrees/gascity/builder-1",
		}},
	}, nil)

	check := newExecutorIdentityResidueCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	})

	result := check.Run(&doctor.CheckContext{})
	if result.Status != doctor.StatusWarning {
		t.Fatalf("status = %v, want warning: %#v", result.Status, result)
	}
	details := strings.Join(result.Details, "\n")
	if !strings.Contains(details, "CITY-1") {
		t.Fatalf("details missing CITY-1:\n%s", details)
	}

	if err := check.Fix(&doctor.CheckContext{}); err != nil {
		t.Fatalf("Fix returned error: %v", err)
	}

	bd, err := cityStore.Get("CITY-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	for _, key := range []string{"gc.session_name", "gc.work_dir", "work_dir"} {
		if v := bd.Metadata[key]; v != "" {
			t.Fatalf("expected %s cleared, got %q", key, v)
		}
	}
	if bd.Metadata["gc.routed_to"] != "gascity/reviewer" {
		t.Fatalf("Fix must not touch gc.routed_to, got %q", bd.Metadata["gc.routed_to"])
	}

	result = check.Run(&doctor.CheckContext{})
	if result.Status != doctor.StatusOK {
		t.Fatalf("status after fix = %v, want ok: %#v", result.Status, result)
	}
}

func TestExecutorIdentityResidueCheckSkipsInProgressBead(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "in flight", Type: "task", Status: "in_progress", Metadata: map[string]string{
			"gc.routed_to":    "gascity/reviewer",
			"gc.session_name": "gascity--builder",
			"gc.work_dir":     "/worktrees/gascity/builder-1",
		}},
	}, nil)

	check := newExecutorIdentityResidueCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	})

	result := check.Run(&doctor.CheckContext{})
	if result.Status != doctor.StatusOK {
		t.Fatalf("status = %v, want ok (in_progress beads must never be flagged): %#v", result.Status, result)
	}

	if err := check.Fix(&doctor.CheckContext{}); err != nil {
		t.Fatalf("Fix returned error: %v", err)
	}

	bd, err := cityStore.Get("CITY-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if bd.Metadata["gc.session_name"] != "gascity--builder" || bd.Metadata["gc.work_dir"] != "/worktrees/gascity/builder-1" {
		t.Fatalf("Fix must not touch an in_progress bead's stamps, got %+v", bd.Metadata)
	}
}

func TestExecutorIdentityResidueCheckFixIsIdempotent(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{}
	cityStore := &residueSetMetadataBatchSpyStore{Store: beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "stale stamp", Type: "task", Status: "open", Metadata: map[string]string{
			"gc.routed_to":    "gascity/reviewer",
			"gc.session_name": "gascity--builder",
		}},
	}, nil)}

	check := newExecutorIdentityResidueCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	})

	if err := check.Fix(&doctor.CheckContext{}); err != nil {
		t.Fatalf("first Fix returned error: %v", err)
	}
	if cityStore.calls != 1 {
		t.Fatalf("expected exactly 1 write after first Fix, got %d", cityStore.calls)
	}

	if err := check.Fix(&doctor.CheckContext{}); err != nil {
		t.Fatalf("second Fix returned error: %v", err)
	}
	if cityStore.calls != 1 {
		t.Fatalf("expected zero additional writes on second Fix, got %d total", cityStore.calls)
	}
}

func TestExecutorIdentityResidueCheckAllowsCanonicalSessionNameEncoding(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "still current", Type: "task", Status: "open", Metadata: map[string]string{
			"gc.routed_to":    "gascity/builder",
			"gc.session_name": "gascity--builder",
		}},
	}, nil)

	check := newExecutorIdentityResidueCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	})

	result := check.Run(&doctor.CheckContext{})
	if result.Status != doctor.StatusOK {
		t.Fatalf("status = %v, want ok (session_name canonicalizes to current routed_to, not drift): %#v", result.Status, result)
	}
}

func TestExecutorIdentityResidueCheckSkipsSessionBeadWithoutSessionName(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "session bead", Type: "session", Status: "open", Metadata: map[string]string{
			"work_dir": "/worktrees/gascity/builder-1",
		}},
	}, nil)

	check := newExecutorIdentityResidueCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	})

	result := check.Run(&doctor.CheckContext{})
	if result.Status != doctor.StatusOK {
		t.Fatalf("status = %v, want ok (a session bead's bare work_dir must not be flagged without gc.session_name): %#v", result.Status, result)
	}
}

func TestExecutorIdentityResidueCheckSkipsDrainStepWithoutSessionName(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "drain step", Type: "task", Status: "open", Metadata: map[string]string{
			"gc.work_dir": "/worktrees/gascity/builder-1",
			"work_dir":    "/worktrees/gascity/builder-1",
		}},
	}, nil)

	check := newExecutorIdentityResidueCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	})

	result := check.Run(&doctor.CheckContext{})
	if result.Status != doctor.StatusOK {
		t.Fatalf("status = %v, want ok (a drain step/member carries both work_dir keys but never gc.session_name, so it must not be flagged): %#v", result.Status, result)
	}
}

func TestExecutorIdentityResidueCheckSkipsOpenPoolBeadWithoutSessionName(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "pool ready", Type: "task", Status: "open", Metadata: map[string]string{
			"gc.work_dir": "/worktrees/gascity/builder-1",
		}},
	}, nil)

	check := newExecutorIdentityResidueCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	})

	result := check.Run(&doctor.CheckContext{})
	if result.Status != doctor.StatusOK {
		t.Fatalf("status = %v, want ok (an open pool-ready bead has gc.work_dir but no gc.session_name, so it must not be flagged): %#v", result.Status, result)
	}
}

func TestExecutorIdentityResidueCheckSkipsWorkflowRunRootDespiteSessionNameStamp(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "workflow run root", Type: "task", Status: "open", Metadata: map[string]string{
			"gc.kind":         "workflow",
			"gc.session_name": "gascity--builder",
			"gc.work_dir":     "/worktrees/gascity/builder-1",
			"gc.routed_to":    "gascity/reviewer",
		}},
	}, nil)

	check := newExecutorIdentityResidueCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	})

	result := check.Run(&doctor.CheckContext{})
	if result.Status != doctor.StatusOK {
		t.Fatalf("status = %v, want ok (a workflow run root is topology, never itself claimed; a completed step's visibility stamp on gc.session_name/gc.work_dir must not read as residue even though it mismatches gc.routed_to): %#v", result.Status, result)
	}

	if err := check.Fix(&doctor.CheckContext{}); err != nil {
		t.Fatalf("Fix returned error: %v", err)
	}
	bd, err := cityStore.Get("CITY-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if bd.Metadata["gc.session_name"] != "gascity--builder" || bd.Metadata["gc.work_dir"] != "/worktrees/gascity/builder-1" {
		t.Fatalf("Fix must not clear a workflow run root's visibility stamp, got %+v", bd.Metadata)
	}
}

func TestExecutorIdentityResidueCheckAllowsCustomSessionTemplateEncoding(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{Workspace: config.Workspace{
		Name:            "acmecity",
		SessionTemplate: "{{.City}}-{{.Name}}",
	}}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "custom template session", Type: "task", Status: "open", Metadata: map[string]string{
			"gc.routed_to":    "gascity/builder",
			"gc.session_name": "acmecity-builder",
		}},
	}, nil)

	check := newExecutorIdentityResidueCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	})

	result := check.Run(&doctor.CheckContext{})
	if result.Status != doctor.StatusOK {
		t.Fatalf("status = %v, want ok (gc.session_name minted via a custom session_template forward-encodes from the current gc.routed_to, so it is current, not drift): %#v", result.Status, result)
	}
}

func TestExecutorIdentityResidueCheckDescribeNamesTriggeringKeys(t *testing.T) {
	cityDir := t.TempDir()
	cfg := &config.City{}
	cityStore := beads.NewMemStoreFrom(0, []beads.Bead{
		{ID: "CITY-1", Title: "stale stamp", Type: "task", Status: "open", Metadata: map[string]string{
			"gc.routed_to":    "gascity/reviewer",
			"gc.session_name": "gascity--builder",
		}},
	}, nil)

	check := newExecutorIdentityResidueCheck(cfg, cityDir, func(path string) (beads.Store, error) {
		if path != cityDir {
			return nil, fmt.Errorf("unexpected store path %q", path)
		}
		return cityStore, nil
	})

	result := check.Run(&doctor.CheckContext{})
	if result.Status != doctor.StatusWarning {
		t.Fatalf("status = %v, want warning: %#v", result.Status, result)
	}
	details := strings.Join(result.Details, "\n")
	if !strings.Contains(details, "gc.session_name") {
		t.Fatalf("describe() must name the triggering key(s), got:\n%s", details)
	}
}

type residueSetMetadataBatchSpyStore struct {
	beads.Store
	calls int
}

func (s *residueSetMetadataBatchSpyStore) SetMetadataBatch(id string, kvs map[string]string) error {
	s.calls++
	return s.Store.SetMetadataBatch(id, kvs)
}
