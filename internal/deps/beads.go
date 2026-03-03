package deps

import (
	"context"
	"os/exec"
	"regexp"
	"time"
)

// MinBeadsVersion is the minimum compatible beads (bd) version for this
// Gas City release. Update this when Gas City requires new beads features.
const MinBeadsVersion = "0.58.0"

// BeadsInstallPath is the go install path for beads.
const BeadsInstallPath = "github.com/steveyegge/beads/cmd/bd@latest"

// BeadsStatus represents the state of the beads installation.
type BeadsStatus int

// BeadsStatus values.
const (
	BeadsOK       BeadsStatus = iota // bd found, version compatible
	BeadsNotFound                    // bd not in PATH
	BeadsTooOld                      // bd found but version too old
	BeadsUnknown                     // bd found but couldn't parse version
)

// CheckBeads checks if bd is installed and compatible.
// Returns status and the installed version (if found).
func CheckBeads() (BeadsStatus, string) {
	_, err := exec.LookPath("bd")
	if err != nil {
		return BeadsNotFound, ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "bd", "version")
	output, err := cmd.Output()
	if err != nil {
		return BeadsUnknown, ""
	}

	version := parseBeadsVersion(string(output))
	if version == "" {
		return BeadsUnknown, ""
	}

	if CompareVersions(version, MinBeadsVersion) < 0 {
		return BeadsTooOld, version
	}

	return BeadsOK, version
}

var beadsVersionRe = regexp.MustCompile(`bd version (\d+\.\d+\.\d+)`)

// parseBeadsVersion extracts version from "bd version X.Y.Z ..." output.
func parseBeadsVersion(output string) string {
	matches := beadsVersionRe.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
