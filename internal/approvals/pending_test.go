package approvals

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestPending_RoundTripAndModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "local", "approvals", "pending")
	id, err := NewPendingID()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)
	p := Pending{ID: id, Tool: "claude", Session: "s1", Call: "Bash", Input: PendingInput{Command: "touch a.txt"}, Digest: "d", Created: now, Expires: now.Add(time.Minute), PID: 42}
	if err := WritePending(dir, p); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		for path, want := range map[string]os.FileMode{dir: 0o700, filepath.Join(dir, id+".json"): 0o600} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if got := info.Mode().Perm(); got != want {
				t.Errorf("%s mode = %o, want %o", path, got, want)
			}
		}
	}
	listed, err := ListPending(dir)
	if err != nil || len(listed) != 1 || listed[0].Input.Command != "touch a.txt" || !listed[0].Expires.Equal(p.Expires) {
		t.Fatalf("listed = %+v, %v", listed, err)
	}
	if _, ok := ReadDecision(dir, id); ok {
		t.Error("a decision reads before one was written")
	}
	if err := WriteDecision(dir, id, PendingDecision{Decision: DecideDeny, Digest: "d"}); err != nil {
		t.Fatal(err)
	}
	if d, ok := ReadDecision(dir, id); !ok || d.Decision != DecideDeny || d.Digest != "d" {
		t.Errorf("decision = %+v, %v", d, ok)
	}
	if listed, _ := ListPending(dir); len(listed) != 0 {
		t.Errorf("a decided request is still listed: %+v", listed)
	}
	RemovePending(dir, id)
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("remove left %d files", len(entries))
	}
}

func TestPending_ListsOldestFirstAndSkipsOtherFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	ids := make([]string, 2)
	for i := range ids {
		ids[i], _ = NewPendingID()
		created := now.Add(time.Duration(-i) * time.Minute)
		if err := WritePending(dir, Pending{ID: ids[i], Created: created, Expires: now.Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string]string{"notes.json": "{}", "broken0123456789abcdef0123456789ab.json": "x", ".tmp.json": "{}"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := ListPending(dir)
	if err != nil || len(listed) != 2 || listed[0].ID != ids[1] || listed[1].ID != ids[0] {
		t.Fatalf("listed = %+v, %v", listed, err)
	}
	if missing, err := ListPending(filepath.Join(dir, "none")); err != nil || missing != nil {
		t.Errorf("missing folder lists %v, %v", missing, err)
	}
}

func TestPending_RefusesIDsOutsideTheFolder(t *testing.T) {
	for _, id := range []string{"", "../decisions", strings.Repeat("A", 32), strings.Repeat("g", 32), strings.Repeat("a", 31)} {
		if ValidPendingID(id) {
			t.Errorf("ValidPendingID(%q) = true", id)
		}
	}
	if err := WriteDecision(t.TempDir(), "../x", PendingDecision{Decision: DecideApprove}); err == nil {
		t.Error("a decision for an invalid id was written")
	}
	id, _ := NewPendingID()
	if err := WriteDecision(t.TempDir(), id, PendingDecision{Decision: "always"}); err == nil {
		t.Error("an unknown decision was written")
	}
}

func TestPendingDigest_IsStableAndTiedToEveryPart(t *testing.T) {
	base := PendingDigest("claude", "s", "Bash", []byte(`{"command":"ls"}`))
	if base != PendingDigest("claude", "s", "Bash", []byte(`{"command":"ls"}`)) {
		t.Error("digest changes between runs")
	}
	for name, other := range map[string]string{
		"tool":    PendingDigest("codex", "s", "Bash", []byte(`{"command":"ls"}`)),
		"session": PendingDigest("claude", "t", "Bash", []byte(`{"command":"ls"}`)),
		"call":    PendingDigest("claude", "s", "Read", []byte(`{"command":"ls"}`)),
		"input":   PendingDigest("claude", "s", "Bash", []byte(`{"command":"ls -a"}`)),
		"shift":   PendingDigest("claude", "sB", "ash", []byte(`{"command":"ls"}`)),
	} {
		if other == base {
			t.Errorf("digest ignores the %s", name)
		}
	}
}

func TestShowableInput(t *testing.T) {
	cases := []struct {
		name  string
		call  string
		input map[string]any
		want  PendingInput
		ok    bool
	}{
		{"bash", "Bash", map[string]any{"command": "touch a.txt", "description": "x"}, PendingInput{Command: "touch a.txt"}, true},
		{"empty command", "Bash", map[string]any{"command": " "}, PendingInput{}, false},
		{"read", "Read", map[string]any{"file_path": "/p/a.go"}, PendingInput{FilePath: "/p/a.go"}, true},
		{"write", "Write", map[string]any{"file_path": "/p/a.go", "content": "package a\n"}, PendingInput{FilePath: "/p/a.go", New: "package a\n"}, true},
		{"edit", "Edit", map[string]any{"file_path": "/p/a.go", "old_string": "a", "new_string": "b"}, PendingInput{FilePath: "/p/a.go", Old: "a", New: "b"}, true},
		{"multi edit", "MultiEdit", map[string]any{"file_path": "/p/a.go", "edits": []any{map[string]any{"old_string": "a", "new_string": "b"}, map[string]any{"old_string": "c", "new_string": "d"}}}, PendingInput{FilePath: "/p/a.go", Old: "a\n…\nc", New: "b\n…\nd"}, true},
		{"notebook", "NotebookEdit", map[string]any{"notebook_path": "/p/n.ipynb", "new_source": "print(1)"}, PendingInput{FilePath: "/p/n.ipynb", New: "print(1)"}, true},
		{"question", "AskUserQuestion", map[string]any{"questions": []any{}}, PendingInput{}, false},
		{"mcp", "mcp__notion__fetch", map[string]any{"id": "x"}, PendingInput{}, false},
		{"web", "WebFetch", map[string]any{"url": "https://example.test"}, PendingInput{}, false},
		{"too large", "Write", map[string]any{"file_path": "/p/a", "content": strings.Repeat("a", MaxPendingInput)}, PendingInput{}, false},
		{"credential", "Bash", map[string]any{"command": "export API_KEY=sk-live-0123456789abcdef0123"}, PendingInput{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ShowableInput(tc.call, tc.input)
			if ok != tc.ok || got != tc.want {
				t.Errorf("ShowableInput = %+v, %v; want %+v, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestCompanionLive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local", "approvals", "companion.live")
	now := time.Now()
	if CompanionLive(path, now) {
		t.Error("a missing file reads as live")
	}
	if err := TouchLive(path, now); err != nil {
		t.Fatal(err)
	}
	if !CompanionLive(path, now.Add(LiveFresh-time.Second)) {
		t.Error("a fresh file reads as stale")
	}
	if CompanionLive(path, now.Add(LiveFresh)) {
		t.Error("a stale file reads as live")
	}
}
