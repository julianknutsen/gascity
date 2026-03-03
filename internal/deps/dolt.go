package deps

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// MinDoltVersion is the minimum compatible dolt version for this Gas City release.
// Update this when Gas City requires new dolt features.
const MinDoltVersion = "1.83.1"

// DoltInstallURL is the installation page for dolt.
const DoltInstallURL = "https://github.com/dolthub/dolt#installation"

// DoltStatus represents the state of the dolt installation.
type DoltStatus int

// DoltStatus values.
const (
	DoltOK         DoltStatus = iota // dolt found, version compatible
	DoltNotFound                     // dolt not in PATH
	DoltTooOld                       // dolt found but version too old
	DoltExecFailed                   // dolt found but 'dolt version' failed
	DoltUnknown                      // dolt version ran but output couldn't be parsed
)

// CheckDolt checks if dolt is installed and compatible.
// Returns status, the installed version (if found), and diagnostic detail
// for failure cases.
func CheckDolt() (DoltStatus, string, string) {
	path, err := exec.LookPath("dolt")
	if err != nil {
		return DoltNotFound, "", ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return DoltExecFailed, "", fmt.Sprintf("at %s: %s", path, detail)
	}

	version := parseDoltVersion(string(output))
	if version == "" {
		return DoltUnknown, "", strings.TrimSpace(string(output))
	}

	if CompareVersions(version, MinDoltVersion) < 0 {
		return DoltTooOld, version, ""
	}

	return DoltOK, version, ""
}

var doltVersionRe = regexp.MustCompile(`dolt version (\d+\.\d+\.\d+)`)

// parseDoltVersion extracts version from "dolt version X.Y.Z" output.
func parseDoltVersion(output string) string {
	matches := doltVersionRe.FindStringSubmatch(output)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}
