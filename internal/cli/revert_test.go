package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestRevertOne_RestoresFromBak(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path+".bak", []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := revertOne(path, false, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original" {
		t.Errorf("expected restored content, got %q", got)
	}
	if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
		t.Errorf("expected .bak removed, got err=%v", err)
	}
}

// TestRevertOne_PreservesWhenNoBak regresses #217. The pre-#217 default
// removed any adapter-emitted file lacking a .bak, which also wiped
// user-authored helpers (check.mjs, *.template) that share a path with
// adapter output. The new default preserves those files; pass --force
// to opt back into delete-without-bak.
func TestRevertOne_PreservesWhenNoBak(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("user-authored"), 0o644); err != nil {
		t.Fatal(err)
	}
	action, err := revertOne(path, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if action != "preserve" {
		t.Errorf("expected preserve, got %q", action)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file was removed despite no --force: %v", err)
	}
	if string(got) != "user-authored" {
		t.Errorf("file content changed: %q", got)
	}
}

func TestRevertOne_ForceRemovesWhenNoBak(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	action, err := revertOne(path, false, true)
	if err != nil {
		t.Fatal(err)
	}
	if action != "remove" {
		t.Errorf("expected remove, got %q", action)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file removed under --force, got err=%v", err)
	}
}

func TestRevertOne_DryRunNoSideEffects(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	path := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(path, []byte("generated"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := revertOne(path, true, true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("dry-run removed the file: %v", err)
	}
}

func TestRevertOne_MissingFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	if _, err := revertOne(filepath.Join(dir, "ghost.md"), false, false); err != nil {
		t.Errorf("unexpected error reverting missing file: %v", err)
	}
}

func TestRevertCmd_ForceRemovesGeneratedRules(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	if err := NewRootCmd("test").Execute(); err != nil {
		// no-op execute to make sure command tree builds
		_ = err
	}

	rulePath := filepath.Join(dir, ".claude/rules/r1.md")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rulePath); err != nil {
		t.Fatalf("expected claude rule after sync: %v", err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude", "--force"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rulePath); !os.IsNotExist(err) {
		t.Errorf("expected claude rule removed after revert --force, err=%v", err)
	}
}

// TestRevertCmd_PreservesUserAuthoredHelpers regresses #217. A helper
// file that lives in a skill folder alongside SKILL.md (e.g. check.mjs)
// gets propagated to the emit dir by sync and therefore appears in the
// capture list at revert time. Without a .bak, the old default
// silently deleted those files. The new default leaves them in place.
func TestRevertCmd_PreservesUserAuthoredHelpers(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	skillSrc := filepath.Join(dir, ".agnostic-ai", "skills", "i18n-parity")
	if err := os.MkdirAll(skillSrc, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillSrc, "SKILL.md"),
		[]byte("---\nname: i18n-parity\ndescription: parity\n---\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillSrc, "check.mjs"),
		[]byte("// user helper\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	emitted := filepath.Join(dir, ".claude/skills/i18n-parity/check.mjs")
	if _, err := os.Stat(emitted); err != nil {
		t.Fatalf("sync did not propagate helper: %v", err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(emitted); err != nil {
		t.Errorf("revert deleted user-authored helper without --force: %v", err)
	}
}

func TestRevertCmd_RestoresFromBackup(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	rulePath := filepath.Join(dir, ".claude/rules/r1.md")
	if err := os.MkdirAll(filepath.Dir(rulePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulePath, []byte("hand-written"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--backup"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(rulePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "hand-written" {
		t.Errorf("expected restored content, got %q", got)
	}
	if _, err := os.Stat(rulePath + ".bak"); !os.IsNotExist(err) {
		t.Errorf("expected .bak removed after restore")
	}
}

// Regression for #389 repro A: a `sync --backup` over a hand-authored
// entry-point file must be undone by `revert`, restoring the original
// content from the .bak and removing the .bak. Before the fix revert never
// touched entry-point files, orphaning the user's content in CLAUDE.md.bak.
func TestRevertCmd_RestoresEntryPointFromBackup(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	claudeMd := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(claudeMd, []byte("MY ORIGINAL NOTES\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--backup"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(claudeMd)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "MY ORIGINAL NOTES\n" {
		t.Errorf("entry-point not restored, got %q", got)
	}
	if _, err := os.Stat(claudeMd + ".bak"); !os.IsNotExist(err) {
		t.Errorf("expected CLAUDE.md.bak removed after restore")
	}
}

// Regression for #389 repro B: `revert --force` must delete generated
// entry-point files, not just adapter-emitted ones.
func TestRevertCmd_ForceRemovesEntryPoints(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	claudeMd := filepath.Join(dir, "CLAUDE.md")
	if _, err := os.Stat(claudeMd); err != nil {
		t.Fatalf("expected CLAUDE.md after sync: %v", err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude", "--force"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(claudeMd); !os.IsNotExist(err) {
		t.Errorf("expected CLAUDE.md removed after revert --force, err=%v", err)
	}
}

func TestRevertCmd_DryRunNoSideEffects(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	rulePath := filepath.Join(dir, ".claude/rules/r1.md")

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(rulePath); err != nil {
		t.Errorf("dry-run revert removed the file: %v", err)
	}
}
