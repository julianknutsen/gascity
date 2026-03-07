package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// secretsDir is the subdirectory under .gc/ for session key storage.
const secretsDir = "secrets"

// secretsDirPerm is the permission for the .gc/secrets/ directory.
const secretsDirPerm = 0o700

// secretFilePerm is the permission for individual .key files.
const secretFilePerm = 0o600

// readSessionKey reads a session key (provider resume token) from
// .gc/secrets/<sessionID>.key. Returns ("", nil) if the file does not exist.
func readSessionKey(cityPath, sessionID string) (string, error) {
	path := sessionKeyPath(cityPath, sessionID)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading session key %s: %w", sessionID, err)
	}
	return string(data), nil
}

// writeSessionKey writes a session key to .gc/secrets/<sessionID>.key
// with 0600 permissions. Creates the secrets directory if needed.
func writeSessionKey(cityPath, sessionID, key string) error {
	dir := filepath.Join(cityPath, ".gc", secretsDir)
	if err := os.MkdirAll(dir, secretsDirPerm); err != nil {
		return fmt.Errorf("creating secrets dir: %w", err)
	}
	path := sessionKeyPath(cityPath, sessionID)
	if err := os.WriteFile(path, []byte(key), secretFilePerm); err != nil {
		return fmt.Errorf("writing session key %s: %w", sessionID, err)
	}
	return nil
}

// removeSessionKey removes a session key file.
func removeSessionKey(cityPath, sessionID string) error {
	path := sessionKeyPath(cityPath, sessionID)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing session key %s: %w", sessionID, err)
	}
	return nil
}

// sessionKeyPath returns the path to a session key file.
func sessionKeyPath(cityPath, sessionID string) string {
	return filepath.Join(cityPath, ".gc", secretsDir, sessionID+".key")
}
