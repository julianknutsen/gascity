// Package exec implements [mail.Provider] by delegating each operation to
// a user-supplied script via fork/exec. This follows the Git credential
// helper pattern: a single script receives the operation name as its first
// argument and communicates via JSON on stdin/stdout.
package exec //nolint:revive // internal package, always imported with alias

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/steveyegge/gascity/internal/mail"
)

// Provider implements [mail.Provider] by delegating to a user-supplied script.
type Provider struct {
	script  string
	timeout time.Duration
	ready   sync.Once // ensure-running called once
}

// NewProvider returns an exec mail provider that delegates to the given script.
func NewProvider(script string) *Provider {
	return &Provider{
		script:  script,
		timeout: 30 * time.Second,
	}
}

// Send delegates to: script send <to> with JSON {"from":"...","body":"..."} on stdin.
func (p *Provider) Send(from, to, body string) (mail.Message, error) {
	p.ensureRunning()
	data, err := marshalSendInput(from, body)
	if err != nil {
		return mail.Message{}, err
	}
	out, err := p.run(data, "send", to)
	if err != nil {
		return mail.Message{}, err
	}
	return unmarshalMessage(out)
}

// Inbox delegates to: script inbox <recipient>
func (p *Provider) Inbox(recipient string) ([]mail.Message, error) {
	p.ensureRunning()
	out, err := p.run(nil, "inbox", recipient)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return unmarshalMessages(out)
}

// Read delegates to: script read <id>
func (p *Provider) Read(id string) (mail.Message, error) {
	p.ensureRunning()
	out, err := p.run(nil, "read", id)
	if err != nil {
		return mail.Message{}, err
	}
	return unmarshalMessage(out)
}

// Archive delegates to: script archive <id>
// If the script writes "already archived" to stderr and exits non-zero,
// the error wraps [mail.ErrAlreadyArchived].
func (p *Provider) Archive(id string) error {
	p.ensureRunning()
	_, err := p.run(nil, "archive", id)
	if err != nil && strings.Contains(err.Error(), "already archived") {
		return fmt.Errorf("exec mail archive: %w", mail.ErrAlreadyArchived)
	}
	return err
}

// Check delegates to: script check <recipient>
func (p *Provider) Check(recipient string) ([]mail.Message, error) {
	p.ensureRunning()
	out, err := p.run(nil, "check", recipient)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return unmarshalMessages(out)
}

// ensureRunning calls "ensure-running" on the script once per provider
// lifetime. Exit 2 (unknown op) is treated as success.
func (p *Provider) ensureRunning() {
	p.ready.Do(func() {
		_, _ = p.run(nil, "ensure-running")
	})
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
		return "", fmt.Errorf("exec mail provider %s %s: %s", p.script, strings.Join(args, " "), errMsg)
	}

	return strings.TrimRight(stdout.String(), "\n"), nil
}

// Compile-time interface check.
var _ mail.Provider = (*Provider)(nil)
