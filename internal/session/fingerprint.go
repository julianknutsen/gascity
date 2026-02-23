package session

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

// ConfigFingerprint returns a deterministic hash of the Config fields that
// define an agent's identity: Command and Env. Fields that are observation
// or startup hints (WorkDir, ReadyPromptPrefix, ReadyDelayMs, ProcessNames,
// EmitsPermissionWarning) are excluded — changing them should not trigger
// a restart.
//
// The hash is a hex-encoded SHA-256. Same config always produces the same
// hash regardless of Env map iteration order.
func ConfigFingerprint(cfg Config) string {
	h := sha256.New()
	h.Write([]byte(cfg.Command)) //nolint:errcheck // hash.Write never errors
	h.Write([]byte{0})           //nolint:errcheck // hash.Write never errors

	keys := make([]string, 0, len(cfg.Env))
	for k := range cfg.Env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))          //nolint:errcheck // hash.Write never errors
		h.Write([]byte{'='})        //nolint:errcheck // hash.Write never errors
		h.Write([]byte(cfg.Env[k])) //nolint:errcheck // hash.Write never errors
		h.Write([]byte{0})          //nolint:errcheck // hash.Write never errors
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
