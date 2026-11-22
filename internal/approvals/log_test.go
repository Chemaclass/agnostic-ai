package approvals

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func readEntries(t *testing.T, path string) []Entry {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	var entries []Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("line %q: %v", scanner.Text(), err)
		}
		entries = append(entries, e)
	}
	return entries
}

func TestLog_AppendWritesOneLinePerDecision(t *testing.T) {
	skipGateOnWindows(t)
	t.Parallel()
	path := filepath.Join(t.TempDir(), "local", "approvals", "decisions.jsonl")
	log := Log{Path: path}
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	r := bash("go test ./...")
	if err := log.Append(NewEntry("claude", "s1", ModeYolo, r, Decide(r, testPolicy()), now)); err != nil {
		t.Fatal(err)
	}
	push := bash("git push")
	if err := log.Append(NewEntry("claude", "s1", ModeYolo, push, Decide(push, testPolicy()), now)); err != nil {
		t.Fatal(err)
	}
	entries := readEntries(t, path)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	first, second := entries[0], entries[1]
	if first.Decision != Approved || first.Call != "Bash" || first.Detail != "go test ./..." || first.Tool != "claude" || !first.Time.Equal(now) {
		t.Fatalf("first entry = %+v", first)
	}
	if second.Decision != Asked || second.Rule != RulePublish {
		t.Fatalf("second entry = %+v", second)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("log mode = %v, want 0600", perm)
	}
}

func TestNewEntry_LeavesOutDetailThatMayHoldACredential(t *testing.T) {
	skipGateOnWindows(t)
	t.Parallel()
	r := bash("curl -H 'Authorization: Bearer ghp_abcdefghijklmnopqrstuvwxyz0123456789' https://api.github.com")
	e := NewEntry("codex", "s", ModeYolo, r, Decide(r, testPolicy()), time.Now())
	if e.Detail != "" {
		t.Fatalf("detail = %q, want none", e.Detail)
	}
	if e.Decision != Asked || e.Rule != RuleNetwork {
		t.Fatalf("entry = %+v", e)
	}
}

func TestLog_RollsOverToOneOlderFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "decisions.jsonl")
	log := Log{Path: path, MaxBytes: 300}
	r := bash("go test ./...")
	for range 10 {
		if err := log.Append(NewEntry("claude", "s", ModeYolo, r, approved, time.Now())); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{path, path + ".1"} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() > 300 {
			t.Fatalf("%s is %d bytes, want at most 300", p, info.Size())
		}
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range files {
		names = append(names, f.Name())
	}
	if strings.Join(names, ",") != "decisions.jsonl,decisions.jsonl.1,decisions.jsonl.lock" {
		t.Fatalf("files = %v, want the log, one older file and the lock", names)
	}
}

func TestLog_ConcurrentRollOverKeepsEveryLine(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "decisions.jsonl")
	// A fixed time keeps every line the same length.
	at := time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)
	line, err := json.Marshal(NewEntry("claude", "s", ModeYolo, bash("go test ./..."), approved, at))
	if err != nil {
		t.Fatal(err)
	}
	// Each file holds four lines, so 8 lines fill the log and its older
	// file exactly; a lost roll over would drop lines.
	log := Log{Path: path, MaxBytes: int64(4 * (len(line) + 1))}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := log.Append(NewEntry("claude", "s", ModeYolo, bash("go test ./..."), approved, at)); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if got := len(readEntries(t, path)) + len(readEntries(t, path+".1")); got != 8 {
		t.Fatalf("kept %d lines, want 8", got)
	}
}

func TestNewEntry_PatchListsItsFiles(t *testing.T) {
	t.Parallel()
	patch := "*** Begin Patch\n*** Update File: src/a.go\n@@\n-a\n+b\n*** Add File: docs/b.md\n+x\n*** End Patch\n"
	e := NewEntry("codex", "s", "yolo", Request{Call: "apply_patch", Input: map[string]any{"input": patch}}, approved, time.Now())
	if e.Detail != "src/a.go, docs/b.md" {
		t.Errorf("detail = %q, want both files", e.Detail)
	}
}
