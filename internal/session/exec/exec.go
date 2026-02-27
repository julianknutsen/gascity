package exec

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/steveyegge/gascity/internal/session"
)

// Provider implements [session.Provider] by delegating each operation to
// a user-supplied script via fork/exec. The script receives the operation
// name as its first argument, following the Git credential helper pattern.
//
// Exit codes: 0 = success, 1 = error (stderr has message), 2 = unknown
// operation (treated as success for forward compatibility).
type Provider struct {
	script  string
	timeout time.Duration
}

// NewProvider returns an exec [Provider] that delegates to the given script.
// The script path may be absolute, relative, or a bare name resolved via
// exec.LookPath.
func NewProvider(script string) *Provider {
	return &Provider{
		script:  script,
		timeout: 30 * time.Second,
	}
}

// run executes the script with the given args, optionally piping stdinData
// to its stdin. Returns the trimmed stdout on success.
//
// Exit code 2 is treated as success (unknown operation — forward compatible).
// Any other non-zero exit code returns an error wrapping stderr.
func (p *Provider) run(stdinData []byte, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, p.script, args...)
	// WaitDelay ensures Go forcibly closes I/O pipes after the context
	// expires, even if grandchild processes (e.g. sleep in a shell script)
	// still hold them open.
	cmd.WaitDelay = 2 * time.Second

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if stdinData != nil {
		cmd.Stdin = bytes.NewReader(stdinData)
	}

	err := cmd.Run()
	if err != nil {
		// Check for exit code 2 → unknown operation → success.
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
		return "", fmt.Errorf("exec provider %s %s: %s", p.script, strings.Join(args, " "), errMsg)
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}

// runWithTTY executes the script with the terminal inherited (for Attach).
func (p *Provider) runWithTTY(args ...string) error {
	cmd := exec.Command(p.script, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Start creates a new session by invoking: script start <name>
// with the session config as JSON on stdin.
func (p *Provider) Start(name string, cfg session.Config) error {
	data, err := marshalStartConfig(cfg)
	if err != nil {
		return fmt.Errorf("exec provider: marshaling start config: %w", err)
	}
	_, err = p.run(data, "start", name)
	return err
}

// Stop destroys the named session: script stop <name>
func (p *Provider) Stop(name string) error {
	_, err := p.run(nil, "stop", name)
	return err
}

// Interrupt sends an interrupt to the session: script interrupt <name>
func (p *Provider) Interrupt(name string) error {
	_, err := p.run(nil, "interrupt", name)
	return err
}

// IsRunning checks if the session is alive: script is-running <name>
// Returns true only if stdout is "true". Errors → false.
func (p *Provider) IsRunning(name string) bool {
	out, err := p.run(nil, "is-running", name)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

// Attach connects the terminal to the session: script attach <name>
func (p *Provider) Attach(name string) error {
	return p.runWithTTY("attach", name)
}

// ProcessAlive checks for a live agent process: script process-alive <name>
// Process names are sent on stdin, one per line.
// Returns true if processNames is empty (per interface contract).
func (p *Provider) ProcessAlive(name string, processNames []string) bool {
	if len(processNames) == 0 {
		return true
	}
	stdin := []byte(strings.Join(processNames, "\n"))
	out, err := p.run(stdin, "process-alive", name)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

// Nudge sends a message to the session: script nudge <name>
// The message is sent on stdin.
func (p *Provider) Nudge(name, message string) error {
	_, err := p.run([]byte(message), "nudge", name)
	return err
}

// SetMeta stores a key-value pair: script set-meta <name> <key>
// The value is sent on stdin.
func (p *Provider) SetMeta(name, key, value string) error {
	_, err := p.run([]byte(value), "set-meta", name, key)
	return err
}

// GetMeta retrieves a metadata value: script get-meta <name> <key>
// Returns ("", nil) if stdout is empty.
func (p *Provider) GetMeta(name, key string) (string, error) {
	return p.run(nil, "get-meta", name, key)
}

// RemoveMeta removes a metadata key: script remove-meta <name> <key>
func (p *Provider) RemoveMeta(name, key string) error {
	_, err := p.run(nil, "remove-meta", name, key)
	return err
}

// Peek captures output from the session: script peek <name> <lines>
func (p *Provider) Peek(name string, lines int) (string, error) {
	return p.run(nil, "peek", name, strconv.Itoa(lines))
}

// ListRunning returns sessions matching a prefix: script list-running <prefix>
// Returns one name per stdout line. Empty stdout → empty slice (not nil).
func (p *Provider) ListRunning(prefix string) ([]string, error) {
	out, err := p.run(nil, "list-running", prefix)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// GetLastActivity returns the last activity time: script get-last-activity <name>
// Expects RFC3339 on stdout, or empty for unsupported. Malformed → zero time.
func (p *Provider) GetLastActivity(name string) (time.Time, error) {
	out, err := p.run(nil, "get-last-activity", name)
	if err != nil {
		return time.Time{}, err
	}
	if out == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, out)
	if err != nil {
		// Malformed timestamp → zero time, no error.
		return time.Time{}, nil
	}
	return t, nil
}
