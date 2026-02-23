package beads

import "strings"

const attachedMolKey = "attached_molecule: "

// GetAttachedMol extracts the attached_molecule value from a bead description.
// Returns "" if not present.
func GetAttachedMol(description string) string {
	for _, line := range strings.Split(description, "\n") {
		if strings.HasPrefix(line, attachedMolKey) {
			return strings.TrimSpace(line[len(attachedMolKey):])
		}
	}
	return ""
}

// SetAttachedMol returns a new description with attached_molecule set.
// Replaces an existing attached_molecule line if present; appends if not.
func SetAttachedMol(description, molID string) string {
	newLine := attachedMolKey + molID
	var lines []string
	found := false
	for _, line := range strings.Split(description, "\n") {
		if strings.HasPrefix(line, attachedMolKey) {
			lines = append(lines, newLine)
			found = true
		} else {
			lines = append(lines, line)
		}
	}
	if !found {
		lines = append(lines, newLine)
	}
	return strings.Join(lines, "\n")
}
