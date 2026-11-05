package cli

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_DryRunDiffPrintsChangedLinesOnly(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	syncThenEditRule(t, dir)
	before := snapshotTree(t, dir)

	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"sync", "-t", "claude", "--dry-run", "--diff"})
	if err := root.Execute(); err != nil {
		t.Fatalf("a preview exits zero on drift: %v", err)
	}
	got := out.String()
	for _, want := range []string{"--- .claude/rules/r1.md (on disk)", "@@", "-hand-edited", "+rule body"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "dry-run: write") || strings.Contains(got, "--- .claude/rules/r1.md ---") {
		t.Errorf("--diff replaces the full file bodies:\n%s", got)
	}
	if after := snapshotTree(t, dir); !maps.Equal(before, after) {
		t.Error("--dry-run --diff must change no files")
	}
}

func TestSync_DryRunDiffIsSilentWhenInSync(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	root = NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"sync", "-t", "claude", "--dry-run", "--diff"})
	if err := root.Execute(); err != nil || out.Len() > 0 {
		t.Errorf("an in-sync preview prints nothing, got err = %v, out:\n%s", err, out.String())
	}
}

func TestSyncGlobal_DryRunDiffPrintsChangesAndWritesNothing(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "# Agreements\n\n- Rule one.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".claude", "CLAUDE.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, path, strings.Replace(string(data), "- Rule one.", "- Rule one, edited by hand.", 1))
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "tidy", "SKILL.md"), "---\nname: tidy\ndescription: Tidy\n---\nTidy.\n")
	before := snapshotTree(t, home)

	out, _, err := runGlobalAgentTest("--only", "claude", "--dry-run", "--diff")
	if err != nil {
		t.Fatalf("a preview exits zero on drift: %v", err)
	}
	for _, want := range []string{
		"--- " + filepath.ToSlash(path) + " (on disk)",
		"-- Rule one, edited by hand.",
		"+- Rule one.",
		"would create " + filepath.ToSlash(filepath.Join(home, ".claude", "skills", "tidy", "SKILL.md")),
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if after := snapshotTree(t, home); !maps.Equal(before, after) {
		t.Error("--dry-run --diff must change no user files, backups, or state")
	}

	if _, _, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
		t.Fatal(err)
	}
	out, _, err = runGlobalAgentTest("--only", "claude", "--dry-run", "--diff")
	if err != nil || out != "" {
		t.Errorf("after sync the preview prints nothing, got err = %v, out:\n%s", err, out)
	}
}

func TestSyncGlobal_DryRunDiffNamesARemoval(t *testing.T) {
	home, source := globalAgentTestHome(t)
	src := filepath.Join(source, "skills", "tidy", "SKILL.md")
	mustWriteGlobalTest(t, src, "---\nname: tidy\ndescription: Tidy\n---\nTidy.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Dir(src)); err != nil {
		t.Fatal(err)
	}
	removed := filepath.Join(home, ".claude", "skills", "tidy", "SKILL.md")

	out, _, err := runGlobalAgentTest("--only", "claude", "--dry-run", "--diff")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "would remove "+filepath.ToSlash(removed)) {
		t.Errorf("the preview must name the removal:\n%s", out)
	}
	if _, err := os.Stat(removed); err != nil {
		t.Errorf("previews must not remove anything: %v", err)
	}
}

// A full sync removes outputs whose spec is gone; a -t run keeps them.
func TestSync_DryRunDiffNamesARemoval(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai", "rules", "r1.md")); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	root = NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"sync", "--dry-run", "--diff"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "would remove .claude/rules/r1.md") {
		t.Errorf("the preview must name the removal:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "rules", "r1.md")); err != nil {
		t.Errorf("previews must not remove anything: %v", err)
	}
}
