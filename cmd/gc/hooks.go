package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// beadHooks maps bd hook filenames to the Gas City event types they emit.
var beadHooks = map[string]string{
	"on_create": "bead.created",
	"on_close":  "bead.closed",
	"on_update": "bead.updated",
}

// hookScript returns the shell script content for a bd hook that forwards
// events to the Gas City event log via gc event emit.
func hookScript(eventType string) string {
	return fmt.Sprintf(`#!/bin/sh
# Installed by gc — forwards bd events to Gas City event log.
# Args: $1=issue_id  $2=event_type  stdin=issue JSON
DATA=$(cat)
title=$(echo "$DATA" | grep -o '"title":"[^"]*"' | head -1 | cut -d'"' -f4)
gc event emit %s --subject "$1" --message "$title" --payload "$DATA" 2>/dev/null || true
`, eventType)
}

// closeHookScript returns the on_close hook script. It forwards the
// bead.closed event AND triggers convoy autoclose for the closed bead's
// parent convoy (if any).
func closeHookScript() string {
	return `#!/bin/sh
# Installed by gc — forwards bd close events and auto-closes completed convoys.
# Args: $1=issue_id  $2=event_type  stdin=issue JSON
DATA=$(cat)
title=$(echo "$DATA" | grep -o '"title":"[^"]*"' | head -1 | cut -d'"' -f4)
gc event emit bead.closed --subject "$1" --message "$title" --payload "$DATA" 2>/dev/null || true
# Auto-close parent convoy if all siblings are now closed.
gc convoy autoclose "$1" 2>/dev/null || true
`
}

// installBeadHooks writes bd hook scripts into dir/.beads/hooks/ so that
// bd mutations (create, close, update) emit events to the Gas City event
// log. Idempotent — overwrites existing hooks. Returns nil on success.
func installBeadHooks(dir string) error {
	hooksDir := filepath.Join(dir, ".beads", "hooks")
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("creating hooks directory: %w", err)
	}

	for filename, eventType := range beadHooks {
		path := filepath.Join(hooksDir, filename)
		content := hookScript(eventType)
		if filename == "on_close" {
			content = closeHookScript()
		}
		if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
			return fmt.Errorf("writing hook %s: %w", filename, err)
		}
	}
	return nil
}
