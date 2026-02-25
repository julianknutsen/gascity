package config

import (
	"crypto/sha256"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/steveyegge/gascity/internal/fsys"
)

// Revision computes a deterministic bundle hash from all resolved config
// source files. This serves as a revision identifier — if the revision
// changes, the effective config may have changed and a reload is warranted.
//
// The hash covers the content of all source files listed in Provenance,
// plus topology directory contents for any rigs with topologies.
func Revision(fs fsys.FS, prov *Provenance, rigs []Rig, cityRoot string) string {
	h := sha256.New()

	// Hash all config source files in stable order.
	sources := make([]string, len(prov.Sources))
	copy(sources, prov.Sources)
	sort.Strings(sources)
	for _, path := range sources {
		data, err := fs.ReadFile(path)
		if err != nil {
			continue
		}
		h.Write([]byte(path)) //nolint:errcheck // hash.Write never errors
		h.Write([]byte{0})    //nolint:errcheck // hash.Write never errors
		h.Write(data)         //nolint:errcheck // hash.Write never errors
		h.Write([]byte{0})    //nolint:errcheck // hash.Write never errors
	}

	// Hash topology directory contents.
	for _, r := range rigs {
		if r.Topology == "" {
			continue
		}
		topoDir := resolveConfigPath(r.Topology, cityRoot, cityRoot)
		topoHash := TopologyContentHashRecursive(fs, topoDir)
		h.Write([]byte("topo:" + r.Name)) //nolint:errcheck // hash.Write never errors
		h.Write([]byte{0})                //nolint:errcheck // hash.Write never errors
		h.Write([]byte(topoHash))         //nolint:errcheck // hash.Write never errors
		h.Write([]byte{0})                //nolint:errcheck // hash.Write never errors
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

// WatchDirs returns the set of directories that should be watched for
// config changes. This includes the directory of each source file and
// topology root directories. Returns deduplicated, sorted paths.
func WatchDirs(prov *Provenance, rigs []Rig, cityRoot string) []string {
	seen := make(map[string]bool)
	var dirs []string

	addDir := func(dir string) {
		if dir != "" && !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}

	// Config source file directories.
	for _, src := range prov.Sources {
		addDir(filepath.Dir(src))
	}

	// Topology root directories.
	for _, r := range rigs {
		if r.Topology == "" {
			continue
		}
		topoDir := resolveConfigPath(r.Topology, cityRoot, cityRoot)
		addDir(topoDir)
	}

	sort.Strings(dirs)
	return dirs
}
