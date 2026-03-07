package main

import (
	"fmt"
	"io"
	"regexp"
	"strconv"

	"github.com/gastownhall/gascity/internal/agent"
	"github.com/gastownhall/gascity/internal/beads"
	"github.com/gastownhall/gascity/internal/clock"
	"github.com/gastownhall/gascity/internal/config"
	"github.com/gastownhall/gascity/internal/runtime"
)

// adoptionResult holds the outcome of an adoption barrier run.
type adoptionResult struct {
	Adopted        int
	AlreadyHadBead int
	Skipped        int // sessions that failed bead creation
	Total          int // total running sessions
	// Details records per-session info for dry-run display.
	Details []adoptionDetail
}

// adoptionDetail describes what would happen for a single session.
type adoptionDetail struct {
	SessionName string
	AgentName   string
	PoolSlot    int  // 0 if not a pool instance
	OutOfBounds bool // pool slot exceeds max
	HasBead     bool // already has an open bead
}

// poolSlotPattern extracts the numeric suffix from pool instance session names.
// e.g., "s-worker-3" -> "3"
var poolSlotPattern = regexp.MustCompile(`-(\d+)$`)

// runAdoptionBarrier ensures every running session has a corresponding open
// session bead. This is rerunnable and crash-safe: if the controller crashes
// mid-adoption, the next startup re-runs it. The per-instance dedup key
// (session_name) prevents duplicate beads.
//
// Config hashes are NOT set by the adoption barrier — the subsequent
// syncSessionBeads call populates them from the built agent objects.
//
// When dryRun is true, no beads are created — the function only reports
// what would happen. This powers the `gc migration plan` command.
//
// Returns the adoption result and whether the barrier passed (all running
// sessions have beads).
func runAdoptionBarrier(
	store beads.Store,
	sp runtime.Provider,
	cfg *config.City,
	cityName string,
	clk clock.Clock,
	stderr io.Writer,
	dryRun bool,
) (adoptionResult, bool) {
	var result adoptionResult

	if store == nil {
		return result, false
	}

	// Step 1: List all running sessions.
	running, err := sp.ListRunning("")
	if err != nil {
		fmt.Fprintf(stderr, "adoption barrier: listing running sessions: %v\n", err) //nolint:errcheck
		return result, false
	}
	result.Total = len(running)
	if len(running) == 0 {
		return result, true // nothing to adopt
	}

	// Step 2: Load existing open session beads, indexed by session_name.
	existing, err := store.ListByLabel(sessionBeadLabel, 0)
	if err != nil {
		fmt.Fprintf(stderr, "adoption barrier: listing beads: %v\n", err) //nolint:errcheck
		return result, false
	}
	bySessionName := make(map[string]bool, len(existing))
	for _, b := range existing {
		if b.Status == "closed" {
			continue // closed beads don't count for dedup
		}
		if sn := b.Metadata["session_name"]; sn != "" {
			bySessionName[sn] = true
		}
	}

	// Build config agent lookup: session_name -> agent config.
	st := cfg.Workspace.SessionTemplate
	agentBySession := make(map[string]*config.Agent, len(cfg.Agents))
	for i := range cfg.Agents {
		a := &cfg.Agents[i]
		sn := agent.SessionNameFor(cityName, a.QualifiedName(), st)
		agentBySession[sn] = a
	}

	now := clk.Now().UTC()

	// Step 3: For each running session, adopt if no open bead exists.
	for _, sessionName := range running {
		if bySessionName[sessionName] {
			result.AlreadyHadBead++
			result.Details = append(result.Details, adoptionDetail{
				SessionName: sessionName,
				HasBead:     true,
			})
			continue
		}

		// Find matching config agent.
		cfgAgent, isConfigAgent := agentBySession[sessionName]

		// Build bead metadata. Config/live hashes are left empty —
		// syncSessionBeads populates them from built agent objects.
		meta := map[string]string{
			"session_name": sessionName,
			"state":        "active",
			"generation":   "1",
			"synced_at":    now.Format("2006-01-02T15:04:05Z07:00"),
		}

		detail := adoptionDetail{SessionName: sessionName}

		if isConfigAgent {
			detail.AgentName = cfgAgent.QualifiedName()
			meta["agent_name"] = cfgAgent.QualifiedName()
		} else {
			detail.AgentName = sessionName
			meta["agent_name"] = sessionName
		}

		// Detect pool instances from session name suffix.
		if slot := parsePoolSlot(sessionName); slot > 0 {
			detail.PoolSlot = slot
			meta["pool_slot"] = strconv.Itoa(slot)
			if isConfigAgent && cfgAgent.Pool != nil && slot > cfgAgent.Pool.Max {
				detail.OutOfBounds = true
				fmt.Fprintf(stderr, "adoption barrier: %s pool slot %d exceeds max %d (adopt-then-drain)\n", //nolint:errcheck
					sessionName, slot, cfgAgent.Pool.Max)
			}
		}

		if dryRun {
			result.Adopted++
			result.Details = append(result.Details, detail)
			continue
		}

		_, createErr := store.Create(beads.Bead{
			Title:    detail.AgentName,
			Type:     sessionBeadType,
			Labels:   []string{sessionBeadLabel},
			Metadata: meta,
		})
		if createErr != nil {
			fmt.Fprintf(stderr, "adoption barrier: creating bead for %s: %v\n", sessionName, createErr) //nolint:errcheck
			result.Skipped++
			continue
		}
		result.Adopted++
		result.Details = append(result.Details, detail)
	}

	// Step 4: Barrier gate — all running sessions must have beads.
	passed := result.Skipped == 0
	return result, passed
}

// parsePoolSlot extracts the numeric pool slot from a session name suffix.
// Returns 0 if no slot suffix is found.
func parsePoolSlot(sessionName string) int {
	matches := poolSlotPattern.FindStringSubmatch(sessionName)
	if len(matches) < 2 {
		return 0
	}
	slot, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return slot
}
