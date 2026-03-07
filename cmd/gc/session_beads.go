package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"

	"github.com/gastownhall/gascity/internal/agent"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/clock"
	"github.com/gastownhall/gascity/internal/runtime"
)

// sessionBeadLabel is the label applied to all session beads for config agents.
const sessionBeadLabel = "gc:agent_session"

// sessionBeadType is the bead type for config agent session beads.
const sessionBeadType = "agent_session"

// syncSessionBeads ensures every config agent has a corresponding session bead.
// This is an additive side-effect — it creates beads for agents that don't have
// them and updates metadata for those that do. It does NOT change agent behavior;
// the existing reconciler continues to manage agent lifecycle.
//
// This is Phase 1 of the unified session model: beads record reality alongside
// the existing reconciler. Phase 2 switches to a bead-driven reconciler.
func syncSessionBeads(
	store beads.Store,
	agents []agent.Agent,
	sp runtime.Provider,
	clk clock.Clock,
	stderr io.Writer,
) {
	if store == nil {
		return
	}

	// Load existing session beads.
	existing, err := store.ListByLabel(sessionBeadLabel, 0)
	if err != nil {
		fmt.Fprintf(stderr, "session beads: listing existing: %v\n", err) //nolint:errcheck
		return
	}

	// Index by session_name for O(1) lookup. Skip closed beads — a closed
	// bead is a completed lifecycle record, not a live session. If an agent
	// restarts after its bead was closed, we create a fresh bead.
	bySessionName := make(map[string]beads.Bead, len(existing))
	for _, b := range existing {
		if b.Status == "closed" {
			continue
		}
		if sn := b.Metadata["session_name"]; sn != "" {
			bySessionName[sn] = b
		}
	}

	// Build a set of desired session names for orphan detection.
	desired := make(map[string]bool, len(agents))

	now := clk.Now().UTC()

	for _, a := range agents {
		sn := a.SessionName()
		desired[sn] = true

		agentCfg := a.SessionConfig()
		coreHash := runtime.CoreFingerprint(agentCfg)
		liveHash := runtime.LiveFingerprint(agentCfg)

		b, exists := bySessionName[sn]
		if !exists {
			// Create a new session bead.
			newBead, createErr := store.Create(beads.Bead{
				Title:  a.Name(),
				Type:   sessionBeadType,
				Labels: []string{sessionBeadLabel, "agent:" + a.Name()},
				Metadata: map[string]string{
					"session_name":   sn,
					"agent_name":     a.Name(),
					"config_hash":    coreHash,
					"live_hash":      liveHash,
					"generation":     "1",
					"instance_token": generateToken(),
					"state":          agentState(sp, sn),
					"synced_at":      now.Format("2006-01-02T15:04:05Z07:00"),
				},
			})
			if createErr != nil {
				fmt.Fprintf(stderr, "session beads: creating bead for %s: %v\n", a.Name(), createErr) //nolint:errcheck
			} else {
				_ = newBead // created successfully
			}
			continue
		}

		// Update existing bead — check for drift.
		// TODO(phase2): replace per-field SetMetadata calls with a single
		// batch update to avoid inconsistent state on partial failure.
		changed := false

		if b.Metadata["config_hash"] != coreHash {
			// Core config changed — bump generation and token.
			gen, _ := strconv.Atoi(b.Metadata["generation"])
			gen++
			setMeta(store, b.ID, "config_hash", coreHash, stderr)
			setMeta(store, b.ID, "generation", strconv.Itoa(gen), stderr)
			setMeta(store, b.ID, "instance_token", generateToken(), stderr)
			changed = true
		}

		if b.Metadata["live_hash"] != liveHash {
			setMeta(store, b.ID, "live_hash", liveHash, stderr)
			changed = true
		}

		// Update state.
		currentState := agentState(sp, sn)
		if b.Metadata["state"] != currentState {
			setMeta(store, b.ID, "state", currentState, stderr)
			changed = true
		}

		// Always update synced_at.
		_ = changed
		setMeta(store, b.ID, "synced_at", now.Format("2006-01-02T15:04:05Z07:00"), stderr)
	}

	// Mark orphans — beads with no matching agent. Do NOT close them
	// (that's Phase 2). Just update state to "orphaned".
	for _, b := range existing {
		sn := b.Metadata["session_name"]
		if sn == "" || desired[sn] {
			continue
		}
		if b.Status == "closed" {
			continue
		}
		if b.Metadata["state"] != "orphaned" {
			setMeta(store, b.ID, "state", "orphaned", stderr)
		}
	}
}

// agentState returns "active" if the agent's session is running, "stopped" otherwise.
func agentState(sp runtime.Provider, sessionName string) string {
	if sp != nil && sp.IsRunning(sessionName) {
		return "active"
	}
	return "stopped"
}

// setMeta wraps store.SetMetadata with error logging.
func setMeta(store beads.Store, id, key, value string, stderr io.Writer) {
	if err := store.SetMetadata(id, key, value); err != nil {
		fmt.Fprintf(stderr, "session beads: setting %s on %s: %v\n", key, id, err) //nolint:errcheck
	}
}

// generateToken returns a cryptographically random hex token.
func generateToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
