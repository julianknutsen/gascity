package beads

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gascity/internal/telemetry"
)

// CommandRunner executes a command in the given directory and returns stdout bytes.
// The dir argument sets the working directory; name and args specify the command.
type CommandRunner func(dir, name string, args ...string) ([]byte, error)

// ExecCommandRunner returns a CommandRunner that uses os/exec to run commands.
// Captures stdout for parsing and stderr for error diagnostics.
// When the command is "bd", records telemetry (duration, status, output).
func ExecCommandRunner() CommandRunner {
	return func(dir, name string, args ...string) ([]byte, error) {
		start := time.Now()
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		if name == "bd" {
			telemetry.RecordBDCall(context.Background(),
				args, float64(time.Since(start).Milliseconds()),
				err, out, stderr.String())
		}
		if err != nil && stderr.Len() > 0 {
			return out, fmt.Errorf("%w: %s", err, stderr.String())
		}
		return out, err
	}
}

// PurgeRunnerFunc executes a bd purge command with custom dir and env.
// Unlike CommandRunner, this supports environment variable manipulation
// needed by bd purge (BEADS_DIR override).
type PurgeRunnerFunc func(dir string, env []string, args ...string) ([]byte, error)

// PurgeResult holds the outcome of a bd purge operation.
type PurgeResult struct {
	Purged int
}

// BdStore implements Store by shelling out to the bd CLI (beads v0.55.1+).
// It delegates all persistence to bd's embedded Dolt database.
type BdStore struct {
	dir         string          // city root directory (where .beads/ lives)
	runner      CommandRunner   // injectable for testing
	purgeRunner PurgeRunnerFunc // injectable for testing; nil uses exec default
}

// NewBdStore creates a BdStore rooted at dir using the given runner.
func NewBdStore(dir string, runner CommandRunner) *BdStore {
	return &BdStore{dir: dir, runner: runner}
}

// Init initializes a beads database via bd init --server. This is an admin
// operation on BdStore directly, not part of the Store interface (MemStore/
// FileStore don't need it). If host is non-empty, --server-host (and
// optionally --server-port) are added to connect to a remote dolt server.
func (s *BdStore) Init(prefix, host, port string) error {
	args := []string{"init", "--server", "-p", prefix, "--skip-hooks"}
	if host != "" {
		args = append(args, "--server-host", host)
	}
	if port != "" {
		args = append(args, "--server-port", port)
	}
	_, err := s.runner(s.dir, "bd", args...)
	if err != nil {
		return fmt.Errorf("bd init: %w", err)
	}
	return nil
}

// ConfigSet sets a bd config key/value pair via bd config set.
func (s *BdStore) ConfigSet(key, value string) error {
	_, err := s.runner(s.dir, "bd", "config", "set", key, value)
	if err != nil {
		return fmt.Errorf("bd config set: %w", err)
	}
	return nil
}

// MolCook instantiates an ephemeral molecule (wisp) from a formula and returns
// the root bead ID. Uses "bd mol cook" to create the molecule.
func (s *BdStore) MolCook(formula, title string, vars []string) (string, error) {
	args := []string{"mol", "cook", "--formula=" + formula}
	if title != "" {
		args = append(args, "--title="+title)
	}
	for _, v := range vars {
		args = append(args, "--var", v)
	}
	out, err := s.runner(s.dir, "bd", args...)
	if err != nil {
		return "", fmt.Errorf("bd mol cook: %w", err)
	}
	rootID := strings.TrimSpace(string(out))
	if rootID == "" {
		return "", fmt.Errorf("bd mol cook produced empty output")
	}
	return rootID, nil
}

// SetPurgeRunner overrides the default exec-based purge implementation.
// Used in tests to inject a fake runner.
func (s *BdStore) SetPurgeRunner(fn PurgeRunnerFunc) {
	s.purgeRunner = fn
}

// Purge runs "bd purge" to remove closed ephemeral beads from the given
// beads directory. Uses a 60-second timeout as a safety circuit breaker.
// The beadsDir is the .beads/ directory path; bd runs from its parent.
func (s *BdStore) Purge(beadsDir string, dryRun bool) (PurgeResult, error) {
	args := []string{"--allow-stale", "purge", "--json"}
	if dryRun {
		args = append(args, "--dry-run")
	}

	dir := filepath.Dir(beadsDir)
	env := envWithout(os.Environ(), "BEADS_DIR")
	env = append(env, "BEADS_DIR="+beadsDir)

	var out []byte
	var err error
	if s.purgeRunner != nil {
		out, err = s.purgeRunner(dir, env, args...)
	} else {
		out, err = execPurge(dir, env, args)
	}
	if err != nil {
		return PurgeResult{}, fmt.Errorf("bd purge: %w", err)
	}

	// Parse JSON output to get purged count.
	jsonBytes := extractJSON(out)
	var result struct {
		PurgedCount *int `json:"purged_count"`
	}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return PurgeResult{}, fmt.Errorf("bd purge: unexpected output format: %s", strings.TrimSpace(string(out)))
	}

	purged := 0
	if result.PurgedCount != nil {
		purged = *result.PurgedCount
	}
	return PurgeResult{Purged: purged}, nil
}

// execPurge runs bd purge via exec.CommandContext with a 60-second timeout.
func execPurge(dir string, env, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bd", args...)
	cmd.Dir = dir
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("timed out after 60s")
	}
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg == "" {
			errMsg = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("%w (%s)", err, errMsg)
	}
	return stdout.Bytes(), nil
}

// extractJSON finds the first JSON object in raw output that may contain
// non-JSON preamble (warnings, debug lines).
func extractJSON(data []byte) []byte {
	start := bytes.IndexByte(data, '{')
	if start < 0 {
		return data
	}
	return data[start:]
}

// envWithout returns a copy of environ with all entries for the given key removed.
func envWithout(environ []string, key string) []string {
	prefix := key + "="
	out := make([]string, 0, len(environ))
	for _, e := range environ {
		if !strings.HasPrefix(e, prefix) {
			out = append(out, e)
		}
	}
	return out
}

// bdIssue is the JSON shape returned by bd CLI commands. We decode only the
// fields Gas City cares about; all others are silently ignored.
type bdIssue struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	IssueType string    `json:"issue_type"`
	CreatedAt time.Time `json:"created_at"`
	Assignee  string    `json:"assignee"`
	Labels    []string  `json:"labels"`
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
		Assignee:  b.Assignee,
		Labels:    b.Labels,
	}
}

// mapBdStatus maps bd's statuses to Gas City's 3. bd uses: open,
// in_progress, blocked, review, testing, closed. Gas City uses:
// open, in_progress, closed.
func mapBdStatus(s string) string {
	switch s {
	case "closed":
		return "closed"
	case "in_progress":
		return "in_progress"
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
	args := []string{"create", "--json", b.Title, "-t", typ}
	for _, l := range b.Labels {
		args = append(args, "--label", l)
	}
	if b.ParentID != "" {
		args = append(args, "--parent", b.ParentID)
	}
	out, err := s.runner(s.dir, "bd", args...)
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

// Update modifies fields of an existing bead via bd update.
func (s *BdStore) Update(id string, opts UpdateOpts) error {
	args := []string{"update", "--json", id}
	if opts.Description != nil {
		args = append(args, "--description", *opts.Description)
	}
	if opts.ParentID != nil {
		args = append(args, "--parent", *opts.ParentID)
	}
	for _, l := range opts.Labels {
		args = append(args, "--label", l)
	}
	_, err := s.runner(s.dir, "bd", args...)
	if err != nil {
		return fmt.Errorf("updating bead %q: %w", id, err)
	}
	return nil
}

// SetMetadata sets a key-value metadata pair on a bead via bd update.
func (s *BdStore) SetMetadata(id, key, value string) error {
	_, err := s.runner(s.dir, "bd", "update", "--json", id,
		"--set-metadata", key+"="+value)
	if err != nil {
		return fmt.Errorf("setting metadata on %q: %w", id, err)
	}
	return nil
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

// ListByLabel returns beads matching an exact label via bd list --label.
// Limit controls max results (0 = unlimited). Results are ordered by bd's
// default sort (newest first).
func (s *BdStore) ListByLabel(label string, limit int) ([]Bead, error) {
	args := []string{"list", "--json", "--label=" + label, "--all", "--limit", fmt.Sprintf("%d", limit)}
	out, err := s.runner(s.dir, "bd", args...)
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

// Children returns all beads whose ParentID matches the given ID. The bd CLI
// does not know about ParentID, so this filters List() results client-side.
// Returns empty for now since Tutorial 06 uses FileStore.
func (s *BdStore) Children(parentID string) ([]Bead, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	var result []Bead
	for _, b := range all {
		if b.ParentID == parentID {
			result = append(result, b)
		}
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
