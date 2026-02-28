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
	// Use stateDir as the project key — unique per test, and an absolute
	// path as required by mcp_agent_mail's human_key validation.
	return `#!/usr/bin/env bash
set -euo pipefail
export PATH="` + binDir + `:$PATH"
export GC_MOCK_STATE_DIR="` + stateDir + `"
export GC_MCP_MAIL_URL="http://127.0.0.1:8765"
export GC_MCP_MAIL_PROJECT="` + stateDir + `"
exec "` + scriptPath + `" "$@"
`
}

// mcpMockCurl returns a mock curl script that simulates mcp_agent_mail v0.3.0.
// Matches the real API: ensure_project uses human_key, register_agent accepts
// name+program+model, send_message returns deliveries format,
// acknowledge_message requires agent_name, get_message is removed.
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
      name=$(echo "$args" | jq -r '.name // empty')
      if [ -z "$name" ]; then
        name="AutoAgent$(next_id)"
      fi
      echo "$name" > "$STATE_DIR/agents/$name"
      jq -n --arg name "$name" '{
        jsonrpc: "2.0", id: 1,
        result: { content: [{type: "text", text: ({"id": 1, "name": $name, "program": "gc", "model": "agent"} | tojson)}] }
      }'
      ;;

    ensure_project)
      key=$(echo "$args" | jq -r '.human_key')
      jq -n --arg key "$key" '{
        jsonrpc: "2.0", id: 1,
        result: { content: [{type: "text", text: ({"id": 1, "slug": "test", "human_key": $key, "created_at": "2026-01-01T00:00:00Z"} | tojson)}] }
      }'
      ;;

    send_message)
      id=$(next_id)
      sender=$(echo "$args" | jq -r '.sender_name')
      to=$(echo "$args" | jq -r '.to[0]')
      subject=$(echo "$args" | jq -r '.subject')
      body_md=$(echo "$args" | jq -r '.body_md')
      ts=$(now_ts)

      # Store message for inbox/read.
      jq -n --argjson id "$id" --arg sender "$sender" --arg to "$to" \
        --arg subject "$subject" --arg body_md "$body_md" --arg ts "$ts" \
        '{
          id: $id,
          from: $sender,
          to: $to,
          subject: $subject,
          body_md: $body_md,
          created_ts: $ts,
          acknowledged: false
        }' > "$STATE_DIR/messages/$id.json"

      # Return deliveries format (matches mcp_agent_mail v0.3.0).
      msg=$(cat "$STATE_DIR/messages/$id.json")
      jq -n --argjson msg "$msg" --arg project "/test/conformance" '{
        jsonrpc: "2.0", id: 1,
        result: { content: [{type: "text", text: ({
          deliveries: [{project: $project, payload: $msg}],
          count: 1
        } | tojson)}] }
      }'
      ;;

    fetch_inbox)
      name=$(echo "$args" | jq -r '.agent_name')
      include_bodies=$(echo "$args" | jq -r '.include_bodies // false')
      msgs="[]"
      for f in "$STATE_DIR/messages/"*.json; do
        [ -f "$f" ] || continue
        msg=$(cat "$f")
        rcpt=$(echo "$msg" | jq -r '.to')
        acked=$(echo "$msg" | jq -r '.acknowledged')
        if [ "$rcpt" = "$name" ] && [ "$acked" = "false" ]; then
          if [ "$include_bodies" = "true" ]; then
            msgs=$(echo "$msgs" | jq --argjson m "$msg" '. + [$m]')
          else
            msgs=$(echo "$msgs" | jq --argjson m "$msg" '. + [($m | del(.body_md))]')
          fi
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
      ts=$(now_ts)
      jq -n --argjson mid "$mid" --arg ts "$ts" '{
        jsonrpc: "2.0", id: 1,
        result: { content: [{type: "text", text: ({"message_id": $mid, "acknowledged": true, "acknowledged_at": $ts, "read_at": $ts} | tojson)}] }
      }'
      ;;

    mark_message_read)
      mid=$(echo "$args" | jq -r '.message_id')
      file="$STATE_DIR/messages/$mid.json"
      if [ ! -f "$file" ]; then
        jq -n '{
          jsonrpc: "2.0", id: 1,
          error: {code: -32000, message: "message not found"}
        }'
        exit 0
      fi
      ts=$(now_ts)
      jq -n --argjson mid "$mid" --arg ts "$ts" '{
        jsonrpc: "2.0", id: 1,
        result: { content: [{type: "text", text: ({"message_id": $mid, "read": true, "read_at": $ts} | tojson)}] }
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
