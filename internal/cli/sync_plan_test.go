package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncPlan_ShowsNoDrift(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// First sync to bring files in sync.
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	// --plan should show "no changes".
	var buf bytes.Buffer
	root2 := NewRootCmd("test")
	root2.SetOut(&buf)
	root2.SetArgs([]string{"sync", "--plan", "-t", "claude"})
	if err := root2.Execute(); err != nil {
		t.Fatalf("sync --plan should not error when in sync: %v", err)
	}
	if !strings.Contains(buf.String(), "no changes") {
		t.Errorf("expected 'no changes' in plan output, got:\n%s", buf.String())
	}
}

func TestSyncPlan_ShowsDrift(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// Do not sync first — files are missing.
	var buf bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&buf)
	root.SetArgs([]string{"sync", "--plan", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync --plan failed: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "added:") {
		t.Errorf("expected 'added:' in plan output for missing files, got:\n%s", out)
	}
}

func TestSyncPlan_ExitsZeroOnDrift(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	// --plan is informational; exits 0 even when drift exists.
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--plan", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Errorf("sync --plan should exit 0 on drift (informational), got: %v", err)
	}
}

func TestSyncPlan_DoesNotWriteFiles(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--plan", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
		t.Error("sync --plan should not write files, but CLAUDE.md was created")
	}
}
