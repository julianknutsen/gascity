package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/doctor"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newDoctorCmd(stdout, stderr io.Writer) *cobra.Command {
	var fix, verbose bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check workspace health",
		Long: `Run diagnostic health checks on the city workspace.

Checks city structure, config validity, binary dependencies (tmux, git,
bd, dolt), controller status, agent sessions, zombie/orphan sessions,
bead stores, Dolt server health, event log integrity, and per-rig
health. Use --fix to attempt automatic repairs.`,
		Example: `  gc doctor
  gc doctor --fix
  gc doctor --verbose`,
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if doDoctor(fix, verbose, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "attempt to fix issues automatically")
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show extra diagnostic details")
	return cmd
}

// doDoctor runs all health checks and prints results.
func doDoctor(fix, verbose bool, stdout, stderr io.Writer) int {
	cityPath, err := resolveCity()
	if err != nil {
		fmt.Fprintf(stderr, "gc doctor: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	d := &doctor.Doctor{}
	ctx := &doctor.CheckContext{CityPath: cityPath, Verbose: verbose}

	// Core checks — always run.
	d.Register(&doctor.CityStructureCheck{})
	d.Register(&doctor.CityConfigCheck{})

	// Load config for deeper checks. If it fails, we still run the core
	// checks above (which will report the parse error).
	cfg, cfgErr := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if cfgErr == nil {
		d.Register(doctor.NewConfigValidCheck(cfg))
	}

	// Topology cache check (if config has remote topologies).
	if cfgErr == nil && len(cfg.Topologies) > 0 {
		d.Register(doctor.NewTopologyCacheCheck(cfg.Topologies, cityPath))
	}

	// Infrastructure checks.
	d.Register(doctor.NewBinaryCheck("tmux", "", exec.LookPath))
	d.Register(doctor.NewBinaryCheck("git", "", exec.LookPath))

	beadsProv := beadsProvider(cityPath)
	doltSkip := os.Getenv("GC_DOLT") == "skip"
	if beadsProv == "file" {
		d.Register(doctor.NewBinaryCheck("bd", "skipped (GC_BEADS=file)", exec.LookPath))
		d.Register(doctor.NewBinaryCheck("dolt", "skipped (GC_BEADS=file)", exec.LookPath))
	} else {
		d.Register(doctor.NewBinaryCheck("bd", "", exec.LookPath))
		if doltSkip {
			d.Register(doctor.NewBinaryCheck("dolt", "skipped (GC_DOLT=skip)", exec.LookPath))
		} else {
			d.Register(doctor.NewBinaryCheck("dolt", "", exec.LookPath))
		}
	}

	// Controller check + session checks (gated by controller state).
	controllerRunning := doctor.IsControllerRunning(cityPath)
	d.Register(doctor.NewControllerCheck(cityPath, controllerRunning))

	if cfgErr == nil && !controllerRunning {
		cityName := cfg.Workspace.Name
		if cityName == "" {
			cityName = filepath.Base(cityPath)
		}
		st := cfg.Workspace.SessionTemplate
		sp := newSessionProvider()

		d.Register(doctor.NewAgentSessionsCheck(cfg, cityName, st, sp))
		d.Register(doctor.NewZombieSessionsCheck(cfg, cityName, st, sp))
		d.Register(doctor.NewOrphanSessionsCheck(cfg, cityName, st, sp))
	}

	// Data checks.
	if cfgErr == nil {
		d.Register(doctor.NewBeadsStoreCheck(cityPath, openStore))
	}
	skipDolt := beadsProv == "file" || doltSkip
	d.Register(doctor.NewDoltServerCheck(cityPath, skipDolt))
	d.Register(&doctor.EventsLogCheck{})

	// Per-rig checks.
	if cfgErr == nil {
		for _, rig := range cfg.Rigs {
			d.Register(doctor.NewRigPathCheck(rig))
			d.Register(doctor.NewRigGitCheck(rig))
			d.Register(doctor.NewRigBeadsCheck(rig, openStore))
		}
	}

	report := d.Run(ctx, stdout, fix)
	doctor.PrintSummary(stdout, report)

	if report.Failed > 0 {
		return 1
	}
	return 0
}

// openStore creates a beads.Store from a directory path. Used as a factory
// for doctor checks that need to verify store accessibility.
func openStore(dirPath string) (beads.Store, error) {
	prov := beadsProvider(dirPath)
	switch prov {
	case "file":
		return beads.OpenFileStore(fsys.OSFS{}, filepath.Join(dirPath, ".gc", "beads.json"))
	default: // "bd"
		if _, err := exec.LookPath("bd"); err != nil {
			return nil, fmt.Errorf("bd not found in PATH")
		}
		return beads.NewBdStore(dirPath, beads.ExecCommandRunner()), nil
	}
}
