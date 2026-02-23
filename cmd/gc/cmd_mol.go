package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/config"
	"github.com/steveyegge/gascity/internal/events"
	"github.com/steveyegge/gascity/internal/formula"
	"github.com/steveyegge/gascity/internal/fsys"
)

func newMolCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mol",
		Short: "Manage molecules (formula instances)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc mol: missing subcommand (create, list, status, step)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc mol: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(
		newMolCreateCmd(stdout, stderr),
		newMolListCmd(stdout, stderr),
		newMolStatusCmd(stdout, stderr),
		newMolStepCmd(stdout, stderr),
	)
	return cmd
}

func newMolCreateCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "create <formula-name>",
		Short: "Create a molecule from a formula",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdMolCreate(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newMolListCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all molecules",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if cmdMolList(stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newMolStatusCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "status <mol-id>",
		Short: "Show molecule progress and current step",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdMolStatus(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

func newMolStepCmd(stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "step",
		Short: "Manage molecule steps",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if len(args) == 0 {
				fmt.Fprintln(stderr, "gc mol step: missing subcommand (done)") //nolint:errcheck // best-effort stderr
			} else {
				fmt.Fprintf(stderr, "gc mol step: unknown subcommand %q\n", args[0]) //nolint:errcheck // best-effort stderr
			}
			return errExit
		},
	}
	cmd.AddCommand(newMolStepDoneCmd(stdout, stderr))
	return cmd
}

func newMolStepDoneCmd(stdout, stderr io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "done <mol-id> <step-ref>",
		Short: "Mark a molecule step as done",
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if cmdMolStepDone(args, stdout, stderr) != 0 {
				return errExit
			}
			return nil
		},
	}
}

// --- CLI entry points ---

// cmdMolCreate is the CLI entry point for creating a molecule.
func cmdMolCreate(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc mol create: missing formula name") //nolint:errcheck // best-effort stderr
		return 1
	}
	store, code := openCityStore(stderr, "gc mol create")
	if store == nil {
		return code
	}
	rec := openCityRecorder(stderr)

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "gc mol create: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cityPath, err := findCity(cwd)
	if err != nil {
		fmt.Fprintf(stderr, "gc mol create: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	cfg, err := config.Load(fsys.OSFS{}, filepath.Join(cityPath, "city.toml"))
	if err != nil {
		fmt.Fprintf(stderr, "gc mol create: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	return doMolCreate(store, rec, fsys.OSFS{}, filepath.Join(cityPath, cfg.FormulasDir()), args[0], stdout, stderr)
}

// cmdMolList is the CLI entry point for listing molecules.
func cmdMolList(stdout, stderr io.Writer) int {
	store, code := openCityStore(stderr, "gc mol list")
	if store == nil {
		return code
	}
	return doMolList(store, stdout, stderr)
}

// cmdMolStatus is the CLI entry point for showing molecule status.
func cmdMolStatus(args []string, stdout, stderr io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintln(stderr, "gc mol status: missing molecule ID") //nolint:errcheck // best-effort stderr
		return 1
	}
	store, code := openCityStore(stderr, "gc mol status")
	if store == nil {
		return code
	}
	return doMolStatus(store, args[0], stdout, stderr)
}

// cmdMolStepDone is the CLI entry point for completing a step.
func cmdMolStepDone(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		fmt.Fprintln(stderr, "gc mol step done: usage: gc mol step done <mol-id> <step-ref>") //nolint:errcheck // best-effort stderr
		return 1
	}
	store, code := openCityStore(stderr, "gc mol step done")
	if store == nil {
		return code
	}
	rec := openCityRecorder(stderr)
	return doMolStepDone(store, rec, args[0], args[1], stdout, stderr)
}

// --- Testable implementations ---

// doMolCreate parses a formula, creates a root molecule bead plus one step
// bead per formula step. Accepts injected dependencies for testability.
func doMolCreate(store beads.Store, rec events.Recorder, fs fsys.FS, formulasDir, name string, stdout, stderr io.Writer) int {
	path := filepath.Join(formulasDir, name+".formula.toml")
	data, err := fs.ReadFile(path)
	if err != nil {
		fmt.Fprintf(stderr, "gc mol create: formula %q not found\n", name) //nolint:errcheck // best-effort stderr
		return 1
	}

	f, err := formula.Parse(data)
	if err != nil {
		fmt.Fprintf(stderr, "gc mol create: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if err := formula.Validate(f); err != nil {
		fmt.Fprintf(stderr, "gc mol create: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Create root molecule bead.
	root, err := store.Create(beads.Bead{
		Title: f.Name,
		Type:  "molecule",
		Ref:   f.Name,
	})
	if err != nil {
		fmt.Fprintf(stderr, "gc mol create: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Create step beads.
	for _, s := range f.Steps {
		_, err := store.Create(beads.Bead{
			Title:       s.Title,
			Type:        "step",
			ParentID:    root.ID,
			Ref:         s.ID,
			Needs:       s.Needs,
			Description: s.Description,
		})
		if err != nil {
			fmt.Fprintf(stderr, "gc mol create: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
	}

	rec.Record(events.Event{
		Type:    events.MoleculeCreated,
		Actor:   eventActor(),
		Subject: root.ID,
		Message: f.Name,
	})
	fmt.Fprintf(stdout, "Created molecule %s (%s, %d steps)\n", root.ID, f.Name, len(f.Steps)) //nolint:errcheck // best-effort stdout
	return 0
}

// doMolList lists all molecule beads with their progress.
func doMolList(store beads.Store, stdout, stderr io.Writer) int {
	all, err := store.List()
	if err != nil {
		fmt.Fprintf(stderr, "gc mol list: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	var mols []beads.Bead
	for _, b := range all {
		if b.Type == "molecule" {
			mols = append(mols, b)
		}
	}

	if len(mols) == 0 {
		fmt.Fprintln(stdout, "No molecules found.") //nolint:errcheck // best-effort stdout
		return 0
	}

	for _, m := range mols {
		children, err := store.Children(m.ID)
		if err != nil {
			fmt.Fprintf(stderr, "gc mol list: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		completed := formula.CompletedCount(children)
		total := len(children)
		fmt.Fprintf(stdout, "%s  %s  %s  (%d/%d complete)\n", m.ID, m.Status, m.Title, completed, total) //nolint:errcheck // best-effort stdout
	}
	return 0
}

// doMolStatus shows the current step of a molecule.
func doMolStatus(store beads.Store, molID string, stdout, stderr io.Writer) int {
	mol, err := store.Get(molID)
	if err != nil {
		fmt.Fprintf(stderr, "gc mol status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if mol.Type != "molecule" {
		fmt.Fprintf(stderr, "gc mol status: %s is not a molecule (type: %s)\n", molID, mol.Type) //nolint:errcheck // best-effort stderr
		return 1
	}

	children, err := store.Children(molID)
	if err != nil {
		fmt.Fprintf(stderr, "gc mol status: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	completed := formula.CompletedCount(children)
	total := len(children)
	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout

	w(fmt.Sprintf("MOLECULE  %s  %s  (%d/%d complete)", molID, mol.Title, completed, total))

	cur := formula.CurrentStep(children)
	if cur == nil {
		w("")
		w("All steps complete.")
		return 0
	}

	w("")
	w(fmt.Sprintf("Current step: %s — %s", cur.Ref, cur.Title))
	if cur.Description != "" {
		w("")
		w("  " + cur.Description)
	}
	w("")
	w(fmt.Sprintf("When done: gc mol step done %s %s", molID, cur.Ref))
	return 0
}

// doMolStepDone closes a step bead and shows the next step or completion.
func doMolStepDone(store beads.Store, rec events.Recorder, molID, stepRef string, stdout, stderr io.Writer) int {
	mol, err := store.Get(molID)
	if err != nil {
		fmt.Fprintf(stderr, "gc mol step done: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}
	if mol.Type != "molecule" {
		fmt.Fprintf(stderr, "gc mol step done: %s is not a molecule (type: %s)\n", molID, mol.Type) //nolint:errcheck // best-effort stderr
		return 1
	}

	children, err := store.Children(molID)
	if err != nil {
		fmt.Fprintf(stderr, "gc mol step done: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Find step by ref.
	var step *beads.Bead
	for i := range children {
		if children[i].Ref == stepRef {
			step = &children[i]
			break
		}
	}
	if step == nil {
		fmt.Fprintf(stderr, "gc mol step done: step %q not found in molecule %s\n", stepRef, molID) //nolint:errcheck // best-effort stderr
		return 1
	}
	if step.Status == "closed" {
		fmt.Fprintf(stderr, "gc mol step done: step %q is already closed\n", stepRef) //nolint:errcheck // best-effort stderr
		return 1
	}

	// Close the step.
	if err := store.Close(step.ID); err != nil {
		fmt.Fprintf(stderr, "gc mol step done: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	rec.Record(events.Event{
		Type:    events.StepCompleted,
		Actor:   eventActor(),
		Subject: step.ID,
		Message: stepRef,
	})

	// Refresh children to get updated state.
	children, err = store.Children(molID)
	if err != nil {
		fmt.Fprintf(stderr, "gc mol step done: %v\n", err) //nolint:errcheck // best-effort stderr
		return 1
	}

	total := len(children)
	idx := formula.StepIndex(children, stepRef)
	w := func(s string) { fmt.Fprintln(stdout, s) } //nolint:errcheck // best-effort stdout

	w(fmt.Sprintf("Step %d/%d: %s — %s", idx, total, stepRef, step.Title))

	next := formula.CurrentStep(children)
	if next == nil {
		// All steps done — close molecule root.
		if err := store.Close(molID); err != nil {
			fmt.Fprintf(stderr, "gc mol step done: closing molecule: %v\n", err) //nolint:errcheck // best-effort stderr
			return 1
		}
		w("")
		w("All steps complete. Molecule closed.")
		return 0
	}

	w("")
	w(fmt.Sprintf("Current step: %s — %s", next.Ref, next.Title))
	if next.Description != "" {
		w("")
		w("  " + next.Description)
	}
	w("")
	w(fmt.Sprintf("When done: gc mol step done %s %s", molID, next.Ref))
	return 0
}
