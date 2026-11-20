package approvals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadLog_ReadsTheOlderFileFirstAndSkipsBrokenLines(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	path := LogPath(home)
	if path != filepath.Join(home, "local", "approvals", "decisions.jsonl") {
		t.Fatalf("LogPath = %s", path)
	}
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	older := Log{Path: path + ".1"}
	if err := older.Append(Entry{Time: now, Tool: "claude", Session: "s1", Call: "Bash", Decision: Approved}); err != nil {
		t.Fatal(err)
	}
	log := Log{Path: path}
	if err := log.Append(Entry{Time: now.Add(time.Second), Tool: "claude", Session: "s1", Call: "Bash", Decision: Asked, Rule: RulePublish}); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	long := `{"decision":"approved","detail":"` + strings.Repeat("x", maxLogLine) + `"}`
	if _, err := f.WriteString("not json\n{}\n" + long + "\n" + `{"time":"2026-11-19T10:00:02Z","tool":"codex","call":"shell","decision":"approved"}`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	entries, err := ReadLog(path)

	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 || entries[0].Decision != Approved || entries[1].Rule != RulePublish || entries[2].Tool != "codex" {
		t.Errorf("entries = %+v", entries)
	}
}

func TestReadLog_MissingLogIsEmpty(t *testing.T) {
	t.Parallel()
	entries, err := ReadLog(LogPath(t.TempDir()))
	if err != nil || len(entries) != 0 {
		t.Errorf("ReadLog = %v, %v", entries, err)
	}
}

func TestReadLog_LeavesOutSensitiveDetails(t *testing.T) {
	t.Parallel()
	log := Log{Path: LogPath(t.TempDir())}
	for _, detail := range []string{"go test ./internal/approvals", "Use password example-only-value here."} {
		if err := log.Append(Entry{Decision: Approved, Detail: detail}); err != nil {
			t.Fatal(err)
		}
	}
	entries, err := ReadLog(log.Path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Detail != "go test ./internal/approvals" || entries[1].Detail != "" {
		t.Error("expected the ordinary detail and an empty sensitive detail")
	}
}
