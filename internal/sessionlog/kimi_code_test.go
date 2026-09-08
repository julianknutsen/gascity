package sessionlog

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestKimiCodeDiscovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	workDir := "/tmp/kimi-probe-ws"
	path := writeKimiContext(t, filepath.Join(root, "sessions", "wd_kimi-probe-ws_87061d3d7a56", "session_native", "agents", "main", "wire.jsonl"), []string{`{"type":"metadata","protocol_version":"1.5","created_at":1787252689149}`})
	for _, search := range []string{root, filepath.Join(root, "sessions")} {
		if got := FindKimiSessionFile([]string{search}, workDir); got != path {
			t.Errorf("newest = %q, want %q", got, path)
		}
		if got := FindKimiSessionFileIfUnambiguous([]string{search}, workDir); got != path {
			t.Errorf("unambiguous = %q, want %q", got, path)
		}
		if got := FindKimiSessionFileByID([]string{search}, workDir, "session_native"); got != path {
			t.Errorf("keyed = %q, want %q", got, path)
		}
	}
	for _, key := range []string{"missing", "../session_native", "session_native/agents"} {
		if got := FindKimiSessionFileByID([]string{root}, workDir, key); got != "" {
			t.Errorf("key %q matched %q", key, got)
		}
	}
	if got := FindKimiSessionFile([]string{root}, "/some/other/workdir"); got != "" {
		t.Errorf("wrong workdir matched %q", got)
	}
	legacy := writeKimiContext(t, filepath.Join(root, "sessions", kimiWorkDirHash(workDir), "legacy", "context.jsonl"), []string{`{"role":"user","content":"old"}`})
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(legacy, past, past); err != nil {
		t.Fatal(err)
	}
	if got := FindKimiSessionFileIfUnambiguous([]string{root}, workDir); got != "" {
		t.Fatalf("mixed layouts are ambiguous, got %q", got)
	}
	if got := FindKimiSessionFile([]string{root}, workDir); got != path {
		t.Fatalf("newest mixed layout = %q", got)
	}
}

func TestReadKimiCodeWire(t *testing.T) {
	path := writeKimiContext(t, filepath.Join(t.TempDir(), "session_native", "agents", "main", "wire.jsonl"), []string{
		`{"type":"metadata","protocol_version":"1.5","created_at":1787252689000}`,
		`{"type":"turn.prompt","input":[{"type":"text","text":"inspect"}],"time":1787252689001}`,
		`{"type":"context.append_message","message":{"id":"msg_user","role":"user","content":[{"type":"text","text":"inspect"}],"toolCalls":[]},"time":1787252689002}`,
		`{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"step1"},"time":1787252689003}`,
		`{"type":"context.append_loop_event","event":{"type":"content.part","uuid":"think1","stepUuid":"step1","part":{"type":"think","think":"checking"}},"time":1787252689004}`,
		`{"type":"context.append_loop_event","event":{"type":"tool.call","uuid":"call1","stepUuid":"step1","toolCallId":"tool1","name":"Read","args":{"path":"main.go"}},"time":1787252689005}`,
		`{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"step1","finishReason":"tool_use"},"time":1787252689006}`,
		`{"type":"context.append_loop_event","event":{"type":"tool.result","parentUuid":"call1","toolCallId":"tool1","result":{"output":"missing","isError":true}},"time":1787252689007}`,
		`{"type":"context.append_loop_event","event":{"type":"step.begin","uuid":"step2"},"time":1787252689008}`,
		`{"type":"context.append_loop_event","event":{"type":"content.part","uuid":"text1","stepUuid":"step2","part":{"type":"text","text":"File missing."}},"time":1787252689009}`,
		`{"type":"context.append_loop_event","event":{"type":"step.end","uuid":"step2","finishReason":"end_turn"},"time":1787252689010}`,
		`{"type":"turn.ended","reason":"completed","time":1787252689011}`,
	})
	sess, err := ReadKimiFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sess.ID != "session_native" {
		t.Errorf("session id = %q", sess.ID)
	}
	if len(sess.Messages) < 5 {
		t.Fatalf("missing native conversation: %d messages", len(sess.Messages))
	}
	var texts, uses, results int
	for _, e := range sess.Messages {
		if e.Timestamp.IsZero() {
			t.Errorf("missing timestamp on %s", e.UUID)
		}
		for _, b := range e.ContentBlocks() {
			switch b.Type {
			case "text":
				texts++
			case "tool_use":
				uses++
				if b.ID != "tool1" || b.Name != "Read" {
					t.Errorf("tool=%+v", b)
				}
			case "tool_result":
				results++
				if b.ToolUseID != "tool1" || !b.IsError {
					t.Errorf("result=%+v", b)
				}
			}
		}
	}
	if texts != 2 || uses != 1 || results != 1 {
		t.Errorf("text/use/result = %d/%d/%d", texts, uses, results)
	}
	if len(sess.OrphanedToolUseIDs) != 0 {
		t.Errorf("orphans = %v", sess.OrphanedToolUseIDs)
	}
	before := kimiEntryIDs(sess.Messages)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("{\"type\":\"unknown.future.event\"}\n{bad\n")
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	after, err := ReadKimiFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, kimiEntryIDs(after.Messages)) {
		t.Fatal("entry IDs changed on append")
	}
	if !after.Diagnostics.MalformedTail || after.Diagnostics.MalformedLineCount != 1 {
		t.Errorf("diagnostics=%+v", after.Diagnostics)
	}
}

func TestKimiCodeTailActivity(t *testing.T) {
	for _, tc := range []struct{ name, line, want string }{
		{"prompt", `{"type":"turn.prompt","input":[]}`, "in-turn"},
		{"stream", `{"type":"context.append_loop_event","event":{"type":"content.part","part":{"type":"text","text":"working"}}}`, "in-turn"},
		{"tools", `{"type":"context.append_loop_event","event":{"type":"step.end","finishReason":"tool_use"}}`, "in-turn"},
		{"finished step still in turn", `{"type":"context.append_loop_event","event":{"type":"step.end","finishReason":"end_turn"}}`, "in-turn"},
		{"ended", `{"type":"turn.ended","reason":"completed"}`, "idle"},
		{"cancel", `{"type":"turn.cancel","target":"active","reason":"aborted"}`, "idle"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeKimiContext(t, filepath.Join(t.TempDir(), "wire.jsonl"), []string{tc.line, `{"type":"usage.record","model":"k3"}`})
			meta, err := ExtractTailMeta(path)
			if err != nil {
				t.Fatal(err)
			}
			if meta == nil || meta.Activity != tc.want {
				t.Fatalf("meta=%+v, want activity %s", meta, tc.want)
			}
		})
	}
}

func TestKimiCodeRootsAndWorkspaceIdentity(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	t.Setenv("KIMI_CODE_HOME", root)
	workDir := t.TempDir()
	alias := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(workDir, alias); err != nil {
		t.Fatal(err)
	}
	path := writeKimiContext(t, filepath.Join(root, "sessions", kimiCodeWorkDirKey(workDir), "session_main", "agents", "main", "wire.jsonl"), []string{`{"type":"metadata","protocol_version":"1.5"}`})
	writeKimiContext(t, filepath.Join(root, "sessions", kimiCodeWorkDirKey(workDir), "session_main", "agents", "agent-0", "wire.jsonl"), []string{`{"type":"metadata","protocol_version":"1.5"}`})
	if got := FindKimiSessionFileByID(nil, alias, "session_main"); got != path {
		t.Fatalf("custom home / canonical workspace = %q", got)
	}
	rootAlias := filepath.Join(t.TempDir(), "account")
	if err := os.Symlink(root, rootAlias); err != nil {
		t.Fatal(err)
	}
	if got := FindKimiSessionFileIfUnambiguous([]string{rootAlias, root}, workDir); !samePath(got, path) {
		t.Fatalf("symlink duplicated the session: %q", got)
	}
	writeKimiContext(t, filepath.Join(root, "sessions", kimiCodeWorkDirKey(workDir), "session_other", "agents", "main", "wire.jsonl"), []string{`{"type":"metadata","protocol_version":"1.5"}`})
	if got := FindKimiSessionFileIfUnambiguous(nil, workDir); got != "" {
		t.Fatalf("ambiguous native sessions matched %q", got)
	}
	if got := FindKimiSessionFileByID(nil, workDir, "session_main"); got != path {
		t.Fatalf("exact native key = %q", got)
	}
	for _, wd := range []string{"", " "} {
		if got := FindKimiSessionFile(nil, wd); got != "" {
			t.Errorf("empty cwd = %q", got)
		}
	}
}

func TestKimiCodeCompactionPagination(t *testing.T) {
	path := writeKimiContext(t, filepath.Join(t.TempDir(), "session_native", "agents", "main", "wire.jsonl"), []string{
		`{"type":"context.append_message","message":{"id":"before","role":"user","content":"before"},"time":1787252689000}`,
		`{"type":"context.apply_compaction","summary":"Previous work summary","compactedCount":1,"time":1787252689001}`,
		`{"type":"context.append_message","message":{"id":"after","role":"user","content":"after"},"time":1787252689002}`,
	})
	full, err := ReadKimiFile(path, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(full.Messages) != 3 || !full.Messages[1].IsCompactBoundary() {
		t.Fatalf("missing compaction boundary: %+v", full.Messages)
	}
	page, err := ReadKimiFilePage(path, 0, "", "before")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 || page.Messages[1].UUID != "after" {
		t.Fatalf("newer page=%+v", page.Messages)
	}
}

func TestKimiCodeSuccessfulLookupDoesNotWarnAboutLegacyLayout(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	workDir := "/tmp/kimi-probe-ws"
	path := writeKimiContext(t, filepath.Join(root, "sessions", "wd_kimi-probe-ws_87061d3d7a56", "session_native", "agents", "main", "wire.jsonl"), []string{`{"type":"metadata"}`})
	// An unrelated legacy store is expected after migration and is not a failed
	// discovery when the native journal is available in another search root.
	writeKimiContext(t, filepath.Join(os.Getenv("HOME"), ".kimi", "sessions", "unrelated", "session_old", "context.jsonl"), []string{`{"role":"user","content":"old"}`})
	var logs bytes.Buffer
	old := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(old)
	if got := FindKimiSessionFile([]string{root}, workDir); got != path {
		t.Fatalf("lookup=%q", got)
	}
	if got := FindKimiSessionFileByID([]string{root}, workDir, "session_native"); got != path {
		t.Fatalf("keyed lookup=%q", got)
	}
	if logs.Len() != 0 {
		t.Fatalf("successful native lookup emitted missing-layout warnings: %s", logs.String())
	}
}
