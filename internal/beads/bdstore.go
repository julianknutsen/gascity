package beads

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// CommandRunner executes a command in the given directory and returns stdout bytes.
// The dir argument sets the working directory; name and args specify the command.
type CommandRunner func(dir, name string, args ...string) ([]byte, error)

// ExecCommandRunner returns a CommandRunner that uses os/exec to run commands.
// Captures stdout for parsing and stderr for error diagnostics.
func ExecCommandRunner() CommandRunner {
	return func(dir, name string, args ...string) ([]byte, error) {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if err != nil && stderr.Len() > 0 {
			return out, fmt.Errorf("%w: %s", err, stderr.String())
		}
		return out, err
	}
}

// BdStore implements Store by shelling out to the bd CLI (beads v0.55.1+).
// It delegates all persistence to bd's embedded Dolt database.
type BdStore struct {
	dir    string        // city root directory (where .beads/ lives)
	runner CommandRunner // injectable for testing
}

// NewBdStore creates a BdStore rooted at dir using the given runner.
func NewBdStore(dir string, runner CommandRunner) *BdStore {
	return &BdStore{dir: dir, runner: runner}
}

// bdIssue is the JSON shape returned by bd CLI commands. We decode only the
// fields Gas City cares about; all others are silently ignored.
type bdIssue struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	IssueType string    `json:"issue_type"`
	CreatedAt time.Time `json:"created_at"`
	Owner     string    `json:"owner"`
}

// toBead converts a bdIssue to a Gas City Bead. CreatedAt is truncated to
// second precision because dolt stores timestamps at second granularity —
// bd create may return sub-second precision that bd show then truncates.
func (b *bdIssue) toBead() Bead {
	return Bead{
		ID:        b.ID,
		Title:     b.Title,
		Status:    mapBdStatus(b.Status),
		Type:      b.IssueType,
		CreatedAt: b.CreatedAt.Truncate(time.Second),
		Assignee:  b.Owner,
	}
}

// mapBdStatus maps bd's 7 statuses to Gas City's 3. bd uses: open,
// in_progress, blocked, review, testing, closed, hooked. Gas City uses:
// open, hooked, closed.
func mapBdStatus(s string) string {
	switch s {
	case "closed":
		return "closed"
	case "hooked":
		return "hooked"
	default:
		return "open"
	}
}

// Create persists a new bead via bd create.
func (s *BdStore) Create(b Bead) (Bead, error) {
	typ := b.Type
	if typ == "" {
		typ = "task"
	}
	out, err := s.runner(s.dir, "bd", "create", "--json", b.Title, "-t", typ)
	if err != nil {
		return Bead{}, fmt.Errorf("bd create: %w", err)
	}
	var issue bdIssue
	if err := json.Unmarshal(out, &issue); err != nil {
		return Bead{}, fmt.Errorf("bd create: parsing JSON: %w", err)
	}
	return issue.toBead(), nil
}

// Get retrieves a bead by ID via bd show.
func (s *BdStore) Get(id string) (Bead, error) {
	out, err := s.runner(s.dir, "bd", "show", "--json", id)
	if err != nil {
		return Bead{}, fmt.Errorf("getting bead %q: %w", id, ErrNotFound)
	}
	var issues []bdIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return Bead{}, fmt.Errorf("bd show: parsing JSON: %w", err)
	}
	if len(issues) == 0 {
		return Bead{}, fmt.Errorf("getting bead %q: %w", id, ErrNotFound)
	}
	return issues[0].toBead(), nil
}

// Close sets a bead's status to closed via bd close.
func (s *BdStore) Close(id string) error {
	_, err := s.runner(s.dir, "bd", "close", "--json", id)
	if err != nil {
		return fmt.Errorf("closing bead %q: %w", id, ErrNotFound)
	}
	return nil
}

// List returns all beads via bd list.
func (s *BdStore) List() ([]Bead, error) {
	out, err := s.runner(s.dir, "bd", "list", "--json", "--limit", "0", "--all")
	if err != nil {
		return nil, fmt.Errorf("bd list: %w", err)
	}
	var issues []bdIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("bd list: parsing JSON: %w", err)
	}
	result := make([]Bead, len(issues))
	for i := range issues {
		result[i] = issues[i].toBead()
	}
	return result, nil
}

// Ready returns all open beads via bd ready.
func (s *BdStore) Ready() ([]Bead, error) {
	out, err := s.runner(s.dir, "bd", "ready", "--json", "--limit", "0")
	if err != nil {
		return nil, fmt.Errorf("bd ready: %w", err)
	}
	var issues []bdIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("bd ready: parsing JSON: %w", err)
	}
	result := make([]Bead, len(issues))
	for i := range issues {
		result[i] = issues[i].toBead()
	}
	return result, nil
}
