package exec //nolint:revive // internal package, always imported with alias

import (
	"os"
	osexec "os/exec"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gascity/internal/mail"
	"github.com/steveyegge/gascity/internal/mail/mailtest"
)

// TestMCPMailConformance runs the mail conformance suite against the
// gc-mail-mcp-agent-mail contrib script with a mock curl. This validates
// that the MCP bridge script conforms to the mail.Provider contract when
// run through the exec provider. Requires jq on PATH.
func TestMCPMailConformance(t *testing.T) {
	if _, err := osexec.LookPath("jq"); err != nil {
		t.Skip("jq not on PATH")
	}

	// Locate the real MCP mail script relative to the module root.
	scriptPath, err := findMCPScript()
	if err != nil {
		t.Skipf("MCP mail script not found: %v", err)
	}

	mailtest.RunProviderTests(t, func(t *testing.T) mail.Provider {
		dir := t.TempDir()

		// State directory for the mock curl.
		stateDir := filepath.Join(dir, "state")
		for _, sub := range []string{"agents", "messages"} {
			if err := os.MkdirAll(filepath.Join(stateDir, sub), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(stateDir, "next_id"), []byte("1"), 0o644); err != nil {
			t.Fatal(err)
		}

		// Write mock curl.
		binDir := filepath.Join(dir, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "curl"), []byte(mcpMockCurl(stateDir)), 0o755); err != nil {
			t.Fatal(err)
		}

		// Write wrapper script that sets env and delegates to the real script.
		wrapperPath := filepath.Join(dir, "mail-provider")
		if err := os.WriteFile(wrapperPath, []byte(mcpWrapper(binDir, scriptPath, stateDir)), 0o755); err != nil {
			t.Fatal(err)
		}

		return NewProvider(wrapperPath)
	})
}

// findMCPScript locates contrib/mail-scripts/gc-mail-mcp-agent-mail by
// walking up from the working directory to find the module root.
func findMCPScript() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, "contrib", "mail-scripts", "gc-mail-mcp-agent-mail")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// mcpWrapper returns a shell script that sets up the mock environment and
// delegates to the real gc-mail-mcp-agent-mail script.
func mcpWrapper(binDir, scriptPath, stateDir string) string {
	return `#!/usr/bin/env bash
set -euo pipefail
export PATH="` + binDir + `:$PATH"
export GC_MOCK_STATE_DIR="` + stateDir + `"
export GC_MCP_MAIL_URL="http://127.0.0.1:8765"
export GC_MCP_MAIL_PROJECT="/test/conformance"
exec "` + scriptPath + `" "$@"
`
}

// mcpMockCurl returns a mock curl script that simulates mcp_agent_mail.
// Enhanced over the contrib .test version: acknowledge_message detects
// already-acknowledged messages and returns "already archived" errors.
func mcpMockCurl(stateDir string) string {
	return `#!/usr/bin/env bash
set -euo pipefail

STATE_DIR="` + stateDir + `"

next_id() {
  local id
  id=$(cat "$STATE_DIR/next_id")
  echo $((id + 1)) > "$STATE_DIR/next_id"
  echo "$id"
}

now_ts() {
  date -u "+%Y-%m-%dT%H:%M:%SZ" 2>/dev/null || echo "2026-02-28T12:00:00Z"
}

# Parse curl args to extract URL and data.
url="" data=""
while [ $# -gt 0 ]; do
  case "$1" in
    -s) shift ;;
    -X) shift 2 ;;
    -H) shift 2 ;;
    -d) data="$2"; shift 2 ;;
    -o) shift 2 ;;
    -w) shift 2 ;;
    *) url="$1"; shift ;;
  esac
done

# Health check.
if [[ "$url" == */health/liveness ]]; then
  echo "OK"
  exit 0
fi

# MCP endpoint.
if [[ "$url" == */mcp ]] && [ -n "$data" ]; then
  tool=$(echo "$data" | jq -r '.params.name')
  args=$(echo "$data" | jq -c '.params.arguments')

  case "$tool" in
    register_agent)
      name=$(echo "$args" | jq -r '.agent_name')
      echo "$name" > "$STATE_DIR/agents/$name"
      jq -n --arg name "$name" '{
        jsonrpc: "2.0", id: 1,
        result: { content: [{type: "text", text: ({"agent_id": 1, "name": $name} | tojson)}] }
      }'
      ;;

    ensure_project)
      jq -n '{
        jsonrpc: "2.0", id: 1,
        result: { content: [{type: "text", text: "{\"status\": \"ready\"}"}] }
      }'
      ;;

    send_message)
      id=$(next_id)
      sender=$(echo "$args" | jq -r '.sender_name')
      to=$(echo "$args" | jq -r '.to[0]')
      body_md=$(echo "$args" | jq -r '.body_md')
      ts=$(now_ts)

      jq -n --argjson id "$id" --arg sender "$sender" --arg to "$to" \
        --arg body_md "$body_md" --arg ts "$ts" \
        '{
          id: $id,
          sender: {name: $sender},
          recipients: [{name: $to}],
          body_md: $body_md,
          created_ts: $ts,
          acknowledged: false
        }' > "$STATE_DIR/messages/$id.json"

      msg=$(cat "$STATE_DIR/messages/$id.json")
      jq -n --argjson msg "$msg" '{
        jsonrpc: "2.0", id: 1,
        result: { content: [{type: "text", text: ($msg | tojson)}] }
      }'
      ;;

    fetch_inbox)
      name=$(echo "$args" | jq -r '.agent_name')
      msgs="[]"
      for f in "$STATE_DIR/messages/"*.json; do
        [ -f "$f" ] || continue
        msg=$(cat "$f")
        rcpt=$(echo "$msg" | jq -r '.recipients[0].name')
        acked=$(echo "$msg" | jq -r '.acknowledged')
        if [ "$rcpt" = "$name" ] && [ "$acked" = "false" ]; then
          msgs=$(echo "$msgs" | jq --argjson m "$msg" '. + [$m]')
        fi
      done
      jq -n --argjson msgs "$msgs" '{
        jsonrpc: "2.0", id: 1,
        result: { content: [{type: "text", text: ($msgs | tojson)}] }
      }'
      ;;

    acknowledge_message)
      mid=$(echo "$args" | jq -r '.message_id')
      file="$STATE_DIR/messages/$mid.json"
      if [ ! -f "$file" ]; then
        jq -n '{
          jsonrpc: "2.0", id: 1,
          error: {code: -32000, message: "message not found"}
        }'
        exit 0
      fi
      acked=$(jq -r '.acknowledged' "$file")
      if [ "$acked" = "true" ]; then
        jq -n '{
          jsonrpc: "2.0", id: 1,
          error: {code: -32000, message: "already archived"}
        }'
        exit 0
      fi
      contents=$(jq '.acknowledged = true' "$file")
      echo "$contents" > "$file"
      jq -n '{
        jsonrpc: "2.0", id: 1,
        result: { content: [{type: "text", text: "{\"status\": \"acknowledged\"}"}] }
      }'
      ;;

    get_message)
      mid=$(echo "$args" | jq -r '.message_id')
      file="$STATE_DIR/messages/$mid.json"
      if [ ! -f "$file" ]; then
        jq -n '{
          jsonrpc: "2.0", id: 1,
          error: {code: -32000, message: "message not found"}
        }'
        exit 0
      fi
      msg=$(cat "$file")
      jq -n --argjson msg "$msg" '{
        jsonrpc: "2.0", id: 1,
        result: { content: [{type: "text", text: ($msg | tojson)}] }
      }'
      ;;

    *)
      jq -n --arg tool "$tool" '{
        jsonrpc: "2.0", id: 1,
        error: {code: -32601, message: ("unknown tool: " + $tool)}
      }'
      ;;
  esac
  exit 0
fi

echo "mock curl: unhandled: $url" >&2
exit 1
`
}
