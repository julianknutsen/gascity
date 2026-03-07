package main

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/gastownhall/gascity/internal/clock"
	"github.com/spf13/cobra"
)

// newMigrationCmd creates the "gc migration" command group.
func newMigrationCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migration",
		Short: "Migration tools for the unified session model",
	}
	cmd.AddCommand(newMigrationPlanCmd(stdout, stderr))
	return cmd
}

// newMigrationPlanCmd creates the "gc migration plan" dry-run command.
func newMigrationPlanCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "plan",
		Short: "Show what the adoption barrier would do without making changes",
		Long: `Dry-run the adoption barrier: list running sessions, check which ones
already have session beads, and report what beads would be created.

No changes are made. Run "gc start" to execute the actual adoption.`,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runMigrationPlan(stdout, stderr)
		},
	}
}

func runMigrationPlan(stdout, stderr io.Writer) error {
	cityPath, err := resolveCity()
	if err != nil {
		return err
	}

	cfg, err := loadCityConfig(cityPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	cityName := cfg.Workspace.Name
	if cityName == "" {
		cityName = filepath.Base(cityPath)
	}

	provName := sessionProviderName()
	sp, err := newSessionProviderByName(provName, cfg.Session, cityName)
	if err != nil {
		return fmt.Errorf("creating session provider: %w", err)
	}

	store, err := openCityStoreAt(cityPath)
	if err != nil {
		fmt.Fprintf(stderr, "Warning: bead store unavailable: %v\n", err)       //nolint:errcheck
		fmt.Fprintln(stdout, "Cannot run migration plan without a bead store.") //nolint:errcheck
		return nil
	}

	result, passed := runAdoptionBarrier(store, sp, cfg, cityName, clock.Real{}, stderr, true)

	// Display results.
	if result.Total == 0 {
		fmt.Fprintln(stdout, "No running sessions found. Nothing to adopt.") //nolint:errcheck
		return nil
	}

	wouldAdopt := 0
	alreadyHave := 0
	for _, d := range result.Details {
		if d.HasBead {
			alreadyHave++
		} else {
			wouldAdopt++
		}
	}

	if wouldAdopt > 0 {
		fmt.Fprintf(stdout, "Would adopt %d running session(s):\n", wouldAdopt) //nolint:errcheck
		for _, d := range result.Details {
			if d.HasBead {
				continue
			}
			line := fmt.Sprintf("  %-30s -> create bead {template: %q", d.SessionName, d.AgentName)
			if d.PoolSlot > 0 {
				line += fmt.Sprintf(", pool_slot: %d", d.PoolSlot)
			}
			line += "}"
			if d.OutOfBounds {
				line += " [OUT OF BOUNDS]"
			}
			fmt.Fprintln(stdout, line) //nolint:errcheck
		}
	} else {
		fmt.Fprintln(stdout, "No sessions need adoption.") //nolint:errcheck
	}

	fmt.Fprintln(stdout)                                         //nolint:errcheck
	fmt.Fprintf(stdout, "Already have beads: %d\n", alreadyHave) //nolint:errcheck

	if passed {
		fmt.Fprintln(stdout, "Orphan cleanup: would be enabled after adoption") //nolint:errcheck
	} else {
		fmt.Fprintln(stdout, "Orphan cleanup: would be DISABLED (some sessions failed validation)") //nolint:errcheck
	}

	fmt.Fprintln(stdout)                                                //nolint:errcheck
	fmt.Fprintln(stdout, "No changes made. Run `gc start` to execute.") //nolint:errcheck

	return nil
}
