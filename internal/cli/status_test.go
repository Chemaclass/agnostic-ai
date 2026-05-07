package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestStatus_HumanReadable(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	syncCmd := NewRootCmd("test")
	syncCmd.SetArgs([]string{"sync", "-t", "claude"})
	if err := syncCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"status"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Project:") {
		t.Errorf("expected 'Project:' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Targets:") {
		t.Errorf("expected 'Targets:' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Last sync:") {
		t.Errorf("expected 'Last sync:' in output, got:\n%s", got)
	}
	if !strings.Contains(got, "Drift:") {
		t.Errorf("expected 'Drift:' in output, got:\n%s", got)
	}
}

func TestStatus_JSON(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	syncCmd := NewRootCmd("test")
	syncCmd.SetArgs([]string{"sync", "-t", "claude"})
	if err := syncCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"status", "--json"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("status --json failed: %v", err)
	}

	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\noutput: %s", err, out.String())
	}
	for _, field := range []string{"project", "layers", "specs", "targets", "last_sync", "files_changed_last_sync", "drift_files"} {
		if _, ok := result[field]; !ok {
			t.Errorf("JSON missing field %q; got: %s", field, out.String())
		}
	}
}

func TestStatus_ExitZeroOnDrift(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	syncCmd := NewRootCmd("test")
	syncCmd.SetArgs([]string{"sync", "-t", "claude"})
	if err := syncCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"status"})
	root.SetOut(&bytes.Buffer{})
	if err := root.Execute(); err != nil {
		t.Errorf("status must exit 0 even when drifted, got: %v", err)
	}
}

func TestStatus_Drifted_ShowsCount(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	syncCmd := NewRootCmd("test")
	syncCmd.SetArgs([]string{"sync", "-t", "claude"})
	if err := syncCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("hand-edited\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"status"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	got := out.String()
	if strings.Contains(got, "in sync") {
		t.Errorf("expected drift in output, got:\n%s", got)
	}
	if !strings.Contains(got, "out of date") {
		t.Errorf("expected 'out of date' in output, got:\n%s", got)
	}
}

func TestStatus_NoStateFile_MtimeFallback(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	syncCmd := NewRootCmd("test")
	syncCmd.SetArgs([]string{"sync", "-t", "claude"})
	if err := syncCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(stateFilePath(".")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"status"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "unknown") {
		t.Errorf("expected mtime fallback timestamp, got 'unknown':\n%s", got)
	}
	if !strings.Contains(got, "Last sync:") {
		t.Errorf("expected 'Last sync:' in output, got:\n%s", got)
	}
}

func TestStatus_NoStateFile_NoFiles_ShowsUnknown(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"status"})
	out := &bytes.Buffer{}
	root.SetOut(out)
	if err := root.Execute(); err != nil {
		t.Fatalf("status failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "unknown") {
		t.Errorf("expected 'unknown' when no state and no files, got:\n%s", got)
	}
}
