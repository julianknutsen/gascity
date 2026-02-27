package main

import (
	"testing"
)

func TestMergeEnvEmptyMaps(t *testing.T) {
	got := mergeEnv(map[string]string{}, map[string]string{})
	if got != nil {
		t.Errorf("mergeEnv(empty, empty) = %v, want nil", got)
	}
}

func TestMergeEnvNilAndValues(t *testing.T) {
	got := mergeEnv(nil, map[string]string{"A": "1"})
	if got["A"] != "1" {
		t.Errorf("mergeEnv[A] = %q, want %q", got["A"], "1")
	}
}

func TestPassthroughEnvIncludesPath(t *testing.T) {
	// PATH is always set in a normal environment.
	got := passthroughEnv()
	if _, ok := got["PATH"]; !ok {
		t.Error("passthroughEnv() missing PATH")
	}
}

func TestPassthroughEnvPicksUpGCBeads(t *testing.T) {
	t.Setenv("GC_BEADS", "file")
	got := passthroughEnv()
	if got["GC_BEADS"] != "file" {
		t.Errorf("passthroughEnv()[GC_BEADS] = %q, want %q", got["GC_BEADS"], "file")
	}
}

func TestPassthroughEnvOmitsUnset(t *testing.T) {
	t.Setenv("GC_DOLT", "")
	got := passthroughEnv()
	if _, ok := got["GC_DOLT"]; ok {
		t.Error("passthroughEnv() should omit empty GC_DOLT")
	}
}

func TestMergeEnvOverrideOrder(t *testing.T) {
	a := map[string]string{"KEY": "first", "A": "a"}
	b := map[string]string{"KEY": "second", "B": "b"}
	got := mergeEnv(a, b)
	if got["KEY"] != "second" {
		t.Errorf("mergeEnv override: KEY = %q, want %q", got["KEY"], "second")
	}
	if got["A"] != "a" {
		t.Errorf("mergeEnv: A = %q, want %q", got["A"], "a")
	}
	if got["B"] != "b" {
		t.Errorf("mergeEnv: B = %q, want %q", got["B"], "b")
	}
}

func TestMergeEnvAllNil(t *testing.T) {
	got := mergeEnv(nil, nil, nil)
	if got != nil {
		t.Errorf("mergeEnv(nil, nil, nil) = %v, want nil", got)
	}
}

func TestPassthroughEnvDoltConnectionVars(t *testing.T) {
	t.Setenv("GC_DOLT_HOST", "dolt.gc.svc.cluster.local")
	t.Setenv("GC_DOLT_PORT", "3307")
	t.Setenv("GC_DOLT_USER", "agent")
	t.Setenv("GC_DOLT_PASSWORD", "s3cret")

	got := passthroughEnv()

	for _, key := range []string{"GC_DOLT_HOST", "GC_DOLT_PORT", "GC_DOLT_USER", "GC_DOLT_PASSWORD"} {
		if _, ok := got[key]; !ok {
			t.Errorf("passthroughEnv() missing %s", key)
		}
	}
	if got["GC_DOLT_HOST"] != "dolt.gc.svc.cluster.local" {
		t.Errorf("GC_DOLT_HOST = %q, want %q", got["GC_DOLT_HOST"], "dolt.gc.svc.cluster.local")
	}
	if got["GC_DOLT_PORT"] != "3307" {
		t.Errorf("GC_DOLT_PORT = %q, want %q", got["GC_DOLT_PORT"], "3307")
	}
}

func TestPassthroughEnvOmitsUnsetDoltVars(t *testing.T) {
	// Ensure the vars are NOT set.
	for _, key := range []string{"GC_DOLT_HOST", "GC_DOLT_PORT", "GC_DOLT_USER", "GC_DOLT_PASSWORD"} {
		t.Setenv(key, "")
	}

	got := passthroughEnv()

	for _, key := range []string{"GC_DOLT_HOST", "GC_DOLT_PORT", "GC_DOLT_USER", "GC_DOLT_PASSWORD"} {
		if _, ok := got[key]; ok {
			t.Errorf("passthroughEnv() should omit empty %s", key)
		}
	}
}
