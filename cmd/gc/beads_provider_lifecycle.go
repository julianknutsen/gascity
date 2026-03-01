package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/dolt"
)

// ensureBeadsProvider starts the bead store's backing service if needed.
// Like Docker Compose starting database containers — convenience, not
// contract. Called by gc start before agents are launched.
func ensureBeadsProvider(cityPath string) error {
	provider := beadsProvider(cityPath)
	switch {
	case strings.HasPrefix(provider, "exec:"):
		return runProviderOp(strings.TrimPrefix(provider, "exec:"), "ensure-ready")
	case provider == "bd":
		if os.Getenv("GC_DOLT") == "skip" {
			return nil
		}
		if err := dolt.EnsureDoltIdentity(); err != nil {
			return fmt.Errorf("dolt identity: %w", err)
		}
		return dolt.EnsureRunning(cityPath)
	}
	return nil // file: always available
}

// shutdownBeadsProvider stops the bead store's backing service.
// Called by gc stop after agents have been terminated.
func shutdownBeadsProvider(cityPath string) error {
	provider := beadsProvider(cityPath)
	switch {
	case strings.HasPrefix(provider, "exec:"):
		return runProviderOp(strings.TrimPrefix(provider, "exec:"), "shutdown")
	case provider == "bd":
		if os.Getenv("GC_DOLT") == "skip" {
			return nil
		}
		return dolt.StopCity(cityPath)
	}
	return nil
}

// initBeadsForDir initializes bead store infrastructure in a directory.
// Idempotent — skips if already initialized. Called during gc start for
// each rig and during gc rig add.
func initBeadsForDir(cityPath, dir, prefix string) error {
	provider := beadsProvider(cityPath)
	switch {
	case strings.HasPrefix(provider, "exec:"):
		return runProviderOp(strings.TrimPrefix(provider, "exec:"), "init", dir, prefix)
	case provider == "bd":
		if os.Getenv("GC_DOLT") == "skip" {
			return nil
		}
		store := beads.NewBdStore(dir, beads.ExecCommandRunner())
		cfg := dolt.GasCityConfig(cityPath)
		return dolt.InitRigBeads(store, dir, prefix, "localhost", cfg.Port)
	}
	return nil
}

// runProviderOp runs a lifecycle operation against an exec beads script.
// Exit 2 = not needed (treated as success, no-op). Used for ensure-ready,
// shutdown, and init operations.
func runProviderOp(script string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, script, args...)
	cmd.WaitDelay = 2 * time.Second

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
			return nil // Not needed
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("exec beads %s: %s", args[0], msg)
	}
	return nil
}
