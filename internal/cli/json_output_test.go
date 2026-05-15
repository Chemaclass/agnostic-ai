package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// assertJSONShape verifies the top-level fields expected in every --json output.
func assertJSONShape(t *testing.T, out *bytes.Buffer) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out.String())
	}
	for _, field := range []string{"version", "command", "writes", "skipped", "errors"} {
		if _, ok := result[field]; !ok {
			t.Errorf("JSON missing field %q; got: %s", field, out.String())
		}
	}
	if v, _ := result["version"].(string); v != "1" {
		t.Errorf("expected version=1, got %q", v)
	}
	return result
}

func TestSyncJSON_Shape(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--json"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --json failed: %v", err)
	}

	result := assertJSONShape(t, out)
	if cmd, _ := result["command"].(string); cmd != "sync" {
		t.Errorf("expected command=sync, got %q", cmd)
	}

	writes, _ := result["writes"].([]any)
	if len(writes) == 0 {
		t.Fatal("expected at least one write record")
	}
	rec, _ := writes[0].(map[string]any)
	for _, field := range []string{"target", "path", "action", "bytes"} {
		if _, ok := rec[field]; !ok {
			t.Errorf("write record missing field %q; got: %v", field, rec)
		}
	}
	if target, _ := rec["target"].(string); target != "claude" {
		t.Errorf("expected target=claude, got %q", target)
	}
}

func TestSyncJSON_SkipUnchanged(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// First sync: creates the file.
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--json"})
	root.SetOut(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	// Second sync: file is unchanged, should be skipped.
	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--json"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("second sync --json failed: %v", err)
	}

	result := assertJSONShape(t, out)
	writes, _ := result["writes"].([]any)
	skipped, _ := result["skipped"].([]any)
	if len(writes) != 0 {
		t.Errorf("expected 0 writes on second sync, got %d", len(writes))
	}
	if len(skipped) == 0 {
		t.Error("expected at least one skipped record on second sync")
	}
}

func TestSyncJSON_Actions(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// First sync: all files should be "create".
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--json"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	result := assertJSONShape(t, out)
	writes, _ := result["writes"].([]any)
	for _, w := range writes {
		rec := w.(map[string]any)
		if action, _ := rec["action"].(string); action != "create" {
			t.Errorf("expected action=create on first sync, got %q for %v", action, rec["path"])
		}
	}

	// Modify the file, then sync again: should be "update".
	rulePath := filepath.Join(dir, ".claude/rules/r1.md")
	if err := os.WriteFile(rulePath, []byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--json"})
	out = &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	result = assertJSONShape(t, out)
	writes, _ = result["writes"].([]any)
	found := false
	for _, w := range writes {
		rec := w.(map[string]any)
		path, _ := rec["path"].(string)
		if filepath.Base(path) == "r1.md" {
			found = true
			if action, _ := rec["action"].(string); action != "update" {
				t.Errorf("expected action=update for r1.md, got %q", action)
			}
		}
	}
	if !found {
		t.Error("r1.md not found in writes after modification")
	}
}

func TestSyncCheckJSON_Shape(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// Sync first so the check passes.
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--check", "--json"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --check --json failed: %v", err)
	}

	result := assertJSONShape(t, out)
	if cmd, _ := result["command"].(string); cmd != "sync --check" {
		t.Errorf("expected command=sync --check, got %q", cmd)
	}
}

func TestSyncCheckJSON_ReportsDrift(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--check", "--json"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	err := root.Execute()
	// Should fail because files don't exist.
	if err == nil {
		t.Fatal("sync --check --json should fail when files are missing")
	}

	result := assertJSONShape(t, out)
	writes, _ := result["writes"].([]any)
	if len(writes) == 0 {
		t.Error("expected drift entries in writes")
	}
	rec, _ := writes[0].(map[string]any)
	if action, _ := rec["action"].(string); action != "missing" {
		t.Errorf("expected action=missing for un-synced file, got %q", action)
	}
}

func TestDoctorJSON_Shape(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude", "--json"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor --json failed: %v", err)
	}

	result := assertJSONShape(t, out)
	if cmd, _ := result["command"].(string); cmd != "doctor" {
		t.Errorf("expected command=doctor, got %q", cmd)
	}
}

func TestDoctorJSON_ReportsDrift(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"doctor", "-t", "claude", "--json"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	err := root.Execute()
	if err == nil {
		t.Fatal("doctor --json should fail when files are missing")
	}

	result := assertJSONShape(t, out)
	writes, _ := result["writes"].([]any)
	if len(writes) == 0 {
		t.Error("expected drift entries in writes")
	}
}

func TestRevertJSON_Shape(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude", "--json", "--force"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("revert --json failed: %v", err)
	}

	result := assertJSONShape(t, out)
	if cmd, _ := result["command"].(string); cmd != "revert" {
		t.Errorf("expected command=revert, got %q", cmd)
	}
	writes, _ := result["writes"].([]any)
	if len(writes) == 0 {
		t.Error("expected at least one write record in revert output")
	}
	rec, _ := writes[0].(map[string]any)
	if action, _ := rec["action"].(string); action != "remove" {
		t.Errorf("expected action=remove, got %q", action)
	}
}

func TestSyncJSON_EmptyArraysWhenClean(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// Pre-sync so second sync skips everything.
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--json"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	// All three arrays must be present (not null) even when empty.
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"writes", "skipped", "errors"} {
		v, ok := result[field]
		if !ok {
			t.Errorf("field %q missing", field)
			continue
		}
		if v == nil {
			t.Errorf("field %q is null, expected empty array", field)
		}
	}
}
