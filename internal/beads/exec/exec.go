package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/steveyegge/gascity/internal/beads"
	"github.com/steveyegge/gascity/internal/formula"
)

// Store implements [beads.Store] by delegating each operation to a
// user-supplied script via fork/exec. The script receives the operation
// name as its first argument and communicates via stdin/stdout JSON.
//
// Exit codes: 0 = success, 1 = error (stderr has message), 2 = unknown
// operation (treated as success for forward compatibility).
type Store struct {
	script          string
	timeout         time.Duration
	formulaResolver formula.Resolver
}

// SetFormulaResolver sets the function used by MolCook to load formulas.
// When set, MolCook is composed in Go from Create calls instead of
// delegating to the script.
func (s *Store) SetFormulaResolver(r formula.Resolver) {
	s.formulaResolver = r
}

// NewStore returns a Store that delegates to the given script.
// The script path may be absolute, relative, or a bare name resolved via
// exec.LookPath.
func NewStore(script string) *Store {
	return &Store{
		script:  script,
		timeout: 30 * time.Second,
	}
}

// run executes the script with the given args, optionally piping stdinData
// to its stdin. Returns the trimmed stdout on success.
//
// Exit code 2 is treated as success (unknown operation — forward compatible).
// Any other non-zero exit code returns an error wrapping stderr.
func (s *Store) run(stdinData []byte, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, s.script, args...)
	// WaitDelay ensures Go forcibly closes I/O pipes after the context
	// expires, even if grandchild processes still hold them open.
	cmd.WaitDelay = 2 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if stdinData != nil {
		cmd.Stdin = bytes.NewReader(stdinData)
	}

	err := cmd.Run()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 2 {
				return "", nil
			}
		}
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = err.Error()
		}
		return "", fmt.Errorf("exec beads %s %s: %s", s.script, strings.Join(args, " "), errMsg)
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}

// parseBead parses a single bead from JSON output.
func parseBead(data string) (beads.Bead, error) {
	var w beadWire
	if err := json.Unmarshal([]byte(data), &w); err != nil {
		return beads.Bead{}, fmt.Errorf("parsing JSON: %w", err)
	}
	return w.toBead(), nil
}

// parseBeadList parses a JSON array of beads. Returns empty slice for
// empty input (not nil).
func parseBeadList(data string) ([]beads.Bead, error) {
	if data == "" {
		return []beads.Bead{}, nil
	}
	var ws []beadWire
	if err := json.Unmarshal([]byte(data), &ws); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}
	result := make([]beads.Bead, len(ws))
	for i := range ws {
		result[i] = ws[i].toBead()
	}
	return result, nil
}

// toBead converts the wire format to a Gas City Bead.
func (w *beadWire) toBead() beads.Bead {
	return beads.Bead{
		ID:          w.ID,
		Title:       w.Title,
		Status:      w.Status,
		Type:        w.Type,
		CreatedAt:   w.CreatedAt,
		Assignee:    w.Assignee,
		ParentID:    w.ParentID,
		Ref:         w.Ref,
		Needs:       w.Needs,
		Description: w.Description,
		Labels:      w.Labels,
	}
}

// Create persists a new bead: script create (stdin: JSON)
func (s *Store) Create(b beads.Bead) (beads.Bead, error) {
	if b.Type == "" {
		b.Type = "task"
	}
	data, err := marshalCreate(b)
	if err != nil {
		return beads.Bead{}, fmt.Errorf("exec beads create: marshaling: %w", err)
	}
	out, err := s.run(data, "create")
	if err != nil {
		return beads.Bead{}, fmt.Errorf("exec beads create: %w", err)
	}
	result, err := parseBead(out)
	if err != nil {
		return beads.Bead{}, fmt.Errorf("exec beads create: %w", err)
	}
	return result, nil
}

// Get retrieves a bead by ID: script get <id>
func (s *Store) Get(id string) (beads.Bead, error) {
	out, err := s.run(nil, "get", id)
	if err != nil {
		return beads.Bead{}, fmt.Errorf("getting bead %q: %w", id, beads.ErrNotFound)
	}
	result, err := parseBead(out)
	if err != nil {
		return beads.Bead{}, fmt.Errorf("exec beads get: %w", err)
	}
	return result, nil
}

// Update modifies fields of an existing bead: script update <id> (stdin: JSON)
func (s *Store) Update(id string, opts beads.UpdateOpts) error {
	data, err := marshalUpdate(opts.Description, opts.ParentID, opts.Labels)
	if err != nil {
		return fmt.Errorf("exec beads update: marshaling: %w", err)
	}
	_, err = s.run(data, "update", id)
	if err != nil {
		return fmt.Errorf("updating bead %q: %w", id, err)
	}
	return nil
}

// Close sets a bead's status to "closed": script close <id>
func (s *Store) Close(id string) error {
	_, err := s.run(nil, "close", id)
	if err != nil {
		return fmt.Errorf("closing bead %q: %w", id, err)
	}
	return nil
}

// List returns all beads: script list
func (s *Store) List() ([]beads.Bead, error) {
	out, err := s.run(nil, "list")
	if err != nil {
		return nil, fmt.Errorf("exec beads list: %w", err)
	}
	return parseBeadList(out)
}

// Ready returns all open beads: script ready
func (s *Store) Ready() ([]beads.Bead, error) {
	out, err := s.run(nil, "ready")
	if err != nil {
		return nil, fmt.Errorf("exec beads ready: %w", err)
	}
	return parseBeadList(out)
}

// Children returns all beads whose ParentID matches: script children <parent-id>
func (s *Store) Children(parentID string) ([]beads.Bead, error) {
	out, err := s.run(nil, "children", parentID)
	if err != nil {
		return nil, fmt.Errorf("exec beads children: %w", err)
	}
	return parseBeadList(out)
}

// ListByLabel returns beads matching a label: script list-by-label <label> <limit>
func (s *Store) ListByLabel(label string, limit int) ([]beads.Bead, error) {
	out, err := s.run(nil, "list-by-label", label, fmt.Sprintf("%d", limit))
	if err != nil {
		return nil, fmt.Errorf("exec beads list-by-label: %w", err)
	}
	return parseBeadList(out)
}

// SetMetadata sets a key-value metadata pair: script set-metadata <id> <key> (stdin: value)
func (s *Store) SetMetadata(id, key, value string) error {
	_, err := s.run([]byte(value), "set-metadata", id, key)
	if err != nil {
		return fmt.Errorf("setting metadata on %q: %w", id, err)
	}
	return nil
}

// MolCook instantiates a molecule from a formula. When a formula resolver
// is set (via [SetFormulaResolver]), the molecule is composed in Go from
// Create calls. Otherwise it delegates to the script's mol-cook operation.
// Returns the root bead ID.
func (s *Store) MolCook(formulaName, title string, vars []string) (string, error) {
	if s.formulaResolver != nil {
		return formula.ComposeMolCook(s, s.formulaResolver, formulaName, title, vars)
	}

	data, err := marshalMolCook(formulaName, title, vars)
	if err != nil {
		return "", fmt.Errorf("exec beads mol-cook: marshaling: %w", err)
	}
	out, err := s.run(data, "mol-cook")
	if err != nil {
		return "", fmt.Errorf("exec beads mol-cook: %w", err)
	}
	rootID := strings.TrimSpace(out)
	if rootID == "" {
		return "", fmt.Errorf("exec beads mol-cook produced empty output")
	}
	return rootID, nil
}

// Compile-time interface check.
var _ beads.Store = (*Store)(nil)
