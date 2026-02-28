package exec //nolint:revive // internal package, always imported with alias

import (
	"fmt"
	"net/http"
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/steveyegge/gascity/internal/mail"
	"github.com/steveyegge/gascity/internal/mail/mailtest"
)

// TestMCPMailConformanceLive runs the conformance suite against a real
// mcp_agent_mail server. Skips if the server is not reachable or jq/curl
// are missing. Each test gets a unique project_key for isolation.
//
// Start the server before running:
//
//	am  # or: uv run python -m mcp_agent_mail.http
//
// Run with:
//
//	go test ./internal/mail/exec/ -run TestMCPMailConformanceLive -v
//
// Override the server URL:
//
//	GC_MCP_MAIL_URL=http://host:port go test ...
func TestMCPMailConformanceLive(t *testing.T) {
	for _, tool := range []string{"jq", "curl"} {
		if _, err := osexec.LookPath(tool); err != nil {
			t.Skipf("%s not on PATH", tool)
		}
	}

	serverURL := os.Getenv("GC_MCP_MAIL_URL")
	if serverURL == "" {
		serverURL = "http://127.0.0.1:8765"
	}

	// Check if server is reachable.
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(serverURL + "/health/liveness")
	if err != nil {
		t.Skipf("mcp_agent_mail not reachable at %s: %v", serverURL, err)
	}
	_ = resp.Body.Close()

	scriptPath, err := findMCPScript()
	if err != nil {
		t.Skipf("MCP mail script not found: %v", err)
	}

	mailtest.RunProviderTests(t, func(t *testing.T) mail.Provider {
		dir := t.TempDir()

		// Unique project per test for isolation.
		project := fmt.Sprintf("gctest-%s-%d", t.Name(), time.Now().UnixNano())

		wrapperPath := filepath.Join(dir, "mail-provider")
		wrapper := fmt.Sprintf("#!/usr/bin/env bash\n"+
			"export GC_MCP_MAIL_URL=%q\n"+
			"export GC_MCP_MAIL_PROJECT=%q\n"+
			"exec %q \"$@\"\n",
			serverURL, project, scriptPath)
		if err := os.WriteFile(wrapperPath, []byte(wrapper), 0o755); err != nil {
			t.Fatal(err)
		}

		return NewProvider(wrapperPath)
	})
}
