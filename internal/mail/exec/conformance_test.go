package exec //nolint:revive // internal package, always imported with alias

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/steveyegge/gascity/internal/mail"
	"github.com/steveyegge/gascity/internal/mail/mailtest"
)

// statefulScript returns a shell script body that maintains message state
// in a temp directory. Each message is stored as a file with line-based
// format: id\nfrom\nto\nbody\ntimestamp\nstatus
func statefulScript(stateDir string) string {
	return `#!/bin/sh
set -e
STATE="` + stateDir + `"
op="$1"
shift

# Initialize next_id if missing.
if [ ! -f "$STATE/next_id" ]; then
  echo 1 > "$STATE/next_id"
fi
mkdir -p "$STATE/messages"

case "$op" in
  ensure-running)
    ;; # no-op
  send)
    to="$1"
    # Read JSON from stdin, extract from and body fields (no jq dependency).
    input=$(cat)
    from=$(echo "$input" | sed 's/.*"from":"\([^"]*\)".*/\1/')
    body=$(echo "$input" | sed 's/.*"body":"\([^"]*\)".*/\1/')
    id=$(cat "$STATE/next_id")
    echo $((id + 1)) > "$STATE/next_id"
    msgid="msg-$id"
    ts=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    printf '%s\n%s\n%s\n%s\n%s\n%s\n' "$msgid" "$from" "$to" "$body" "$ts" "open" > "$STATE/messages/$msgid"
    printf '{"id":"%s","from":"%s","to":"%s","body":"%s","created_at":"%s"}\n' "$msgid" "$from" "$to" "$body" "$ts"
    ;;
  inbox|check)
    recipient="$1"
    # Collect matching messages into a variable first.
    result=""
    for f in "$STATE"/messages/*; do
      [ -f "$f" ] || continue
      status=$(sed -n '6p' "$f")
      [ "$status" = "open" ] || continue
      msg_to=$(sed -n '3p' "$f")
      [ "$msg_to" = "$recipient" ] || continue
      msgid=$(sed -n '1p' "$f")
      from=$(sed -n '2p' "$f")
      body=$(sed -n '4p' "$f")
      ts=$(sed -n '5p' "$f")
      if [ -n "$result" ]; then
        result="$result,"
      fi
      result="${result}{\"id\":\"$msgid\",\"from\":\"$from\",\"to\":\"$msg_to\",\"body\":\"$body\",\"created_at\":\"$ts\"}"
    done
    # Empty stdout signals no messages (exec provider returns nil).
    if [ -n "$result" ]; then
      printf '[%s]\n' "$result"
    fi
    ;;
  read)
    msgid="$1"
    f="$STATE/messages/$msgid"
    if [ ! -f "$f" ]; then
      echo "message \"$msgid\" not found" >&2
      exit 1
    fi
    from=$(sed -n '2p' "$f")
    msg_to=$(sed -n '3p' "$f")
    body=$(sed -n '4p' "$f")
    ts=$(sed -n '5p' "$f")
    # Mark as read.
    sed -i '6s/.*/read/' "$f"
    printf '{"id":"%s","from":"%s","to":"%s","body":"%s","created_at":"%s"}\n' "$msgid" "$from" "$msg_to" "$body" "$ts"
    ;;
  archive)
    msgid="$1"
    f="$STATE/messages/$msgid"
    if [ ! -f "$f" ]; then
      echo "message \"$msgid\" not found" >&2
      exit 1
    fi
    status=$(sed -n '6p' "$f")
    if [ "$status" = "archived" ]; then
      echo "already archived" >&2
      exit 1
    fi
    sed -i '6s/.*/archived/' "$f"
    ;;
  *)
    exit 2 ;; # unknown operation
esac
`
}

func TestExecConformance(t *testing.T) {
	mailtest.RunProviderTests(t, func(t *testing.T) mail.Provider {
		dir := t.TempDir()
		stateDir := filepath.Join(dir, "state")
		if err := os.MkdirAll(filepath.Join(stateDir, "messages"), 0o755); err != nil {
			t.Fatal(err)
		}

		scriptPath := filepath.Join(dir, "mail-provider")
		if err := os.WriteFile(scriptPath, []byte(statefulScript(stateDir)), 0o755); err != nil {
			t.Fatal(err)
		}

		return NewProvider(scriptPath)
	})
}
