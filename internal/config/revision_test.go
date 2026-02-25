package config

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/steveyegge/gascity/internal/fsys"
)

func TestRevision_Deterministic(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "city.toml", `[workspace]
name = "test"
`)

	prov := &Provenance{
		Sources: []string{filepath.Join(dir, "city.toml")},
	}

	h1 := Revision(fsys.OSFS{}, prov, nil, dir)
	h2 := Revision(fsys.OSFS{}, prov, nil, dir)
	if h1 != h2 {
		t.Errorf("not deterministic: %q vs %q", h1, h2)
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestRevision_ChangesOnFileModification(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "city.toml", `[workspace]
name = "test"
`)

	prov := &Provenance{
		Sources: []string{filepath.Join(dir, "city.toml")},
	}

	h1 := Revision(fsys.OSFS{}, prov, nil, dir)

	writeFile(t, dir, "city.toml", `[workspace]
name = "changed"
`)

	h2 := Revision(fsys.OSFS{}, prov, nil, dir)
	if h1 == h2 {
		t.Error("hash should change when file content changes")
	}
}

func TestRevision_IncludesFragments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "city.toml", `[workspace]
name = "test"
`)
	writeFile(t, dir, "agents.toml", `[[agents]]
name = "mayor"
`)

	prov := &Provenance{
		Sources: []string{
			filepath.Join(dir, "city.toml"),
			filepath.Join(dir, "agents.toml"),
		},
	}

	h1 := Revision(fsys.OSFS{}, prov, nil, dir)

	// Change fragment.
	writeFile(t, dir, "agents.toml", `[[agents]]
name = "worker"
`)

	h2 := Revision(fsys.OSFS{}, prov, nil, dir)
	if h1 == h2 {
		t.Error("hash should change when fragment changes")
	}
}

func TestRevision_IncludesTopology(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "city.toml", `[workspace]
name = "test"
`)
	writeFile(t, dir, "topologies/gt/topology.toml", `[topology]
name = "gastown"
schema = 1
`)

	prov := &Provenance{
		Sources: []string{filepath.Join(dir, "city.toml")},
	}
	rigs := []Rig{{Name: "hw", Path: "/hw", Topology: "topologies/gt"}}

	h1 := Revision(fsys.OSFS{}, prov, rigs, dir)

	// Change topology file.
	writeFile(t, dir, "topologies/gt/topology.toml", `[topology]
name = "gastown-v2"
schema = 1
`)

	h2 := Revision(fsys.OSFS{}, prov, rigs, dir)
	if h1 == h2 {
		t.Error("hash should change when topology file changes")
	}
}

func TestWatchDirs_ConfigOnly(t *testing.T) {
	dir := t.TempDir()
	prov := &Provenance{
		Sources: []string{filepath.Join(dir, "city.toml")},
	}

	dirs := WatchDirs(prov, nil, dir)
	if len(dirs) != 1 {
		t.Fatalf("got %d dirs, want 1", len(dirs))
	}
	if dirs[0] != dir {
		t.Errorf("dir = %q, want %q", dirs[0], dir)
	}
}

func TestWatchDirs_WithFragments(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "conf/agents.toml", "")

	prov := &Provenance{
		Sources: []string{
			filepath.Join(dir, "city.toml"),
			filepath.Join(dir, "conf", "agents.toml"),
		},
	}

	dirs := WatchDirs(prov, nil, dir)
	sort.Strings(dirs)

	expected := []string{dir, filepath.Join(dir, "conf")}
	sort.Strings(expected)

	if len(dirs) != 2 {
		t.Fatalf("got %d dirs, want 2: %v", len(dirs), dirs)
	}
	for i := range expected {
		if dirs[i] != expected[i] {
			t.Errorf("dirs[%d] = %q, want %q", i, dirs[i], expected[i])
		}
	}
}

func TestWatchDirs_WithTopology(t *testing.T) {
	dir := t.TempDir()
	prov := &Provenance{
		Sources: []string{filepath.Join(dir, "city.toml")},
	}
	rigs := []Rig{{Name: "hw", Path: "/hw", Topology: "topologies/gt"}}

	dirs := WatchDirs(prov, rigs, dir)

	// Should include city dir + topology dir.
	if len(dirs) != 2 {
		t.Fatalf("got %d dirs, want 2: %v", len(dirs), dirs)
	}

	found := false
	for _, d := range dirs {
		if d == filepath.Join(dir, "topologies", "gt") {
			found = true
		}
	}
	if !found {
		t.Errorf("topology dir not in watch list: %v", dirs)
	}
}

func TestWatchDirs_Deduplicates(t *testing.T) {
	dir := t.TempDir()
	prov := &Provenance{
		Sources: []string{
			filepath.Join(dir, "city.toml"),
			filepath.Join(dir, "agents.toml"),
		},
	}

	dirs := WatchDirs(prov, nil, dir)
	if len(dirs) != 1 {
		t.Errorf("got %d dirs, want 1 (deduplicated): %v", len(dirs), dirs)
	}
}
