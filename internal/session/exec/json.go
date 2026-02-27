// Package exec implements [session.Provider] by delegating each operation
// to a user-supplied script via fork/exec. This follows the Git credential
// helper pattern: a single script receives the operation name as its first
// argument and communicates via stdin/stdout.
//
// See examples/session-scripts/README.md for the protocol specification.
package exec

import (
	"encoding/json"

	"github.com/steveyegge/gascity/internal/session"
)

// startConfig is the JSON wire format sent to the script's stdin on Start.
// It is intentionally separate from [session.Config] to own the serialization
// contract — the script sees stable JSON field names regardless of Go struct
// changes.
type startConfig struct {
	WorkDir      string            `json:"work_dir,omitempty"`
	Command      string            `json:"command,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	ProcessNames []string          `json:"process_names,omitempty"`
	Nudge        string            `json:"nudge,omitempty"`
}

// marshalStartConfig converts a [session.Config] to JSON for the exec script.
func marshalStartConfig(cfg session.Config) ([]byte, error) {
	sc := startConfig{
		WorkDir:      cfg.WorkDir,
		Command:      cfg.Command,
		Env:          cfg.Env,
		ProcessNames: cfg.ProcessNames,
		Nudge:        cfg.Nudge,
	}
	return json.Marshal(sc)
}
