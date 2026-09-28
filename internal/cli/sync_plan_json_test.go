package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func runSyncJSONArgs(t *testing.T, args ...string) jsonOutput {
	t.Helper()
	buf := &strings.Builder{}
	root := NewRootCmd("test")
	root.SetArgs(append([]string{"sync"}, args...))
	root.SetOut(buf)
	root.SetErr(io.Discard)
	if err := root.Execute(); err != nil {
		t.Fatalf("sync %v: %v", args, err)
	}
	var out jsonOutput
	if err := json.Unmarshal([]byte(buf.String()), &out); err != nil {
		t.Fatalf("sync %v printed invalid JSON %q: %v", args, buf.String(), err)
	}
	return out
}

func syncClaudeFixture(t *testing.T) string {
	t.Helper()
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--gitignore", "off"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func recordFor(records []fileRecord, path string) (fileRecord, bool) {
	i := slices.IndexFunc(records, func(r fileRecord) bool { return r.Path == path })
	if i < 0 {
		return fileRecord{}, false
	}
	return records[i], true
}

func TestSyncPlanJSON_ListsEachFileASyncWouldCreate(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	out := runSyncJSONArgs(t, "--plan", "--json", "-t", "claude")

	if out.Version != "1" || out.Command != "sync --plan" {
		t.Errorf("version/command = %q/%q, want 1/sync --plan", out.Version, out.Command)
	}
	rec, ok := recordFor(out.Writes, "CLAUDE.md")
	if !ok || rec.Action != "create" || rec.Bytes == 0 {
		t.Errorf("CLAUDE.md record = %+v (found %v), want a create with its size", rec, ok)
	}
	if !slices.ContainsFunc(out.Writes, func(r fileRecord) bool { return r.Target == "claude" }) {
		t.Errorf("writes = %+v, want a claude file", out.Writes)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Error("sync --plan --json wrote CLAUDE.md")
	}
}

func TestSyncPlanJSON_ListsOnlyChangedFiles(t *testing.T) {
	syncClaudeFixture(t)
	if err := os.WriteFile("CLAUDE.md", []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runSyncJSONArgs(t, "--plan", "--json", "-t", "claude")

	if len(out.Writes) != 1 || out.Writes[0].Path != "CLAUDE.md" || out.Writes[0].Action != "update" || out.Writes[0].Target != "agnostic-ai" {
		t.Errorf("writes = %+v, want only the CLAUDE.md update", out.Writes)
	}
	if len(out.Skipped) != 0 {
		t.Errorf("skipped = %+v, want none", out.Skipped)
	}
}

func TestSyncDryRunJSON_ListsEveryOutputWithoutWriting(t *testing.T) {
	syncClaudeFixture(t)
	if err := os.WriteFile("CLAUDE.md", []byte("hand edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runSyncJSONArgs(t, "--dry-run", "--json", "-t", "claude")

	if out.Command != "sync --dry-run" {
		t.Errorf("command = %q, want sync --dry-run", out.Command)
	}
	if rec, ok := recordFor(out.Writes, "CLAUDE.md"); !ok || rec.Action != "update" {
		t.Errorf("CLAUDE.md record = %+v (found %v), want an update", rec, ok)
	}
	rec, ok := recordFor(out.Skipped, ".agnostic-ai/AGNOSTIC_AI.md")
	if !ok || rec.Action != "skip" || rec.Target != "agnostic-ai" || rec.Bytes == 0 {
		t.Errorf("AGNOSTIC_AI.md record = %+v (found %v), want an unchanged skip", rec, ok)
	}
	if !slices.ContainsFunc(out.Skipped, func(r fileRecord) bool { return r.Target == "claude" && r.Action == "skip" }) {
		t.Errorf("skipped = %+v, want the unchanged claude files", out.Skipped)
	}
	data, err := os.ReadFile("CLAUDE.md")
	if err != nil || string(data) != "hand edit\n" {
		t.Errorf("CLAUDE.md = %q, %v; dry run must not write", data, err)
	}
}

func TestSyncJSON_RejectsModesWithoutJSONOutput(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	for _, args := range [][]string{
		{"--watch", "--json"},
		{"--check", "--diff", "--json"},
	} {
		root := NewRootCmd("test")
		root.SetArgs(append([]string{"sync"}, args...))
		root.SetOut(io.Discard)
		root.SetErr(io.Discard)
		err := root.Execute()
		if err == nil || !strings.Contains(err.Error(), "--json") {
			t.Errorf("sync %v: err = %v, want a --json conflict", args, err)
		}
	}
}
