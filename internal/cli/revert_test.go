package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// sync removes a nested CLAUDE.md that only imports its AGENTS.md; with
// --backup, revert must bring it back like any file sync replaced.
func TestRevertCmd_RestoresRemovedAgentsCompanion(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	files := map[string]string{
		"agnostic-ai.yaml":          "version: 1\ntargets: [claude, codex]\n",
		".agnostic-ai/rules/api.md": "---\nname: api\nscope: services/api\n---\nUse integer minor units.\n",
		"services/api/AGENTS.md":    "Use integer minor units.\n",
		"services/api/CLAUDE.md":    "@AGENTS.md\n",
	}
	for name, body := range files {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude,codex", "--backup"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("services/api/CLAUDE.md"); !os.IsNotExist(err) {
		t.Fatalf("sync should remove the companion, stat err = %v", err)
	}

	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude,codex"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	if got, err := os.ReadFile("services/api/CLAUDE.md"); err != nil || string(got) != "@AGENTS.md\n" {
		t.Errorf("companion not restored: %q, %v", got, err)
	}
	if _, err := os.Stat("services/api/CLAUDE.md.bak"); !os.IsNotExist(err) {
		t.Errorf("expected CLAUDE.md.bak removed after restore, stat err = %v", err)
	}
}

func TestRevert_RecordedMissingBackupRequiresIntactSafePath(t *testing.T) {
	handEditProject(t, "claude")
	edited := readFile(t, handEditSkill) + "edit\n"
	writeFile(t, handEditSkill, edited)
	if err := os.Remove(".agnostic-ai/skills/review/SKILL.md"); err != nil {
		t.Fatal(err)
	}
	runSyncOK(t)
	writeFile(t, handEditSkill+".bak", "changed backup\n")
	if out, err := runCLI(t, "revert"); err != nil {
		t.Fatalf("revert: %v\n%s", err, out)
	}
	if _, err := os.Lstat(handEditSkill); !os.IsNotExist(err) {
		t.Errorf("restored changed backup: %v", err)
	}
	if got := readFile(t, handEditSkill+".bak"); got != "changed backup\n" {
		t.Errorf("changed backup = %q", got)
	}
}

func TestRevertCmd_ForceKeepsSourceNote(t *testing.T) {
	for _, args := range [][]string{
		{"revert", "--force"},
		{"revert", "--force", "--json"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := setupFixture(t)
			testutil.Chdir(t, dir)
			silence(t)
			note := filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md")
			if err := os.WriteFile(note, []byte("Run tests.\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			root := NewRootCmd("test")
			root.SetArgs([]string{"sync", "-t", "claude"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			root = NewRootCmd("test")
			root.SetOut(io.Discard)
			root.SetArgs(append(args, "-t", "claude"))
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}

			got, err := os.ReadFile(note)
			if err != nil {
				t.Fatalf("revert --force removed the source note: %v", err)
			}
			if string(got) != "Run tests.\n" {
				t.Errorf("source note changed: %q", got)
			}
			if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
				t.Errorf("expected CLAUDE.md removed, err=%v", err)
			}
		})
	}
}

// An output path that resolves into the source tree, by a letter-case
// variant, a `..` segment, or a configured folder, is never reverted.
func TestRevertCmd_ForceKeepsFilesInTheSourceTree(t *testing.T) {
	for _, tc := range []struct {
		name, outputs, keep string
	}{
		{"case variant", "  codex:\n    file: .agnostic-ai/agnostic_ai.md\n", ".agnostic-ai/AGNOSTIC_AI.md"},
		{"dot dot", "  codex:\n    file: .agnostic-ai/rules/../AGNOSTIC_AI.md\n", ".agnostic-ai/AGNOSTIC_AI.md"},
		{"rule file", "  codex:\n    file: .agnostic-ai/rules/style.md\n", ".agnostic-ai/rules/style.md"},
		{"skills dir", "  claude:\n    skills-dir: .agnostic-ai/skills\n", ".agnostic-ai/skills/s1/SKILL.md"},
	} {
		for _, json := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s json=%v", tc.name, json), func(t *testing.T) {
				dir := setupFixture(t)
				testutil.Chdir(t, dir)
				silence(t)
				cfg := "version: 1\ntargets: [claude, codex]\noutputs:\n" + tc.outputs
				files := map[string]string{
					"agnostic-ai.yaml":                cfg,
					".agnostic-ai/AGNOSTIC_AI.md":     "Run tests.\n",
					".agnostic-ai/rules/style.md":     "---\nname: style\n---\nUse tabs.\n",
					".agnostic-ai/skills/s1/SKILL.md": "---\nname: s1\ndescription: d\n---\nbody\n",
				}
				for path, body := range files {
					if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
						t.Fatal(err)
					}
				}
				// sync may refuse the layout, or write into the source tree
				// itself; revert must leave whatever it finds there.
				root := NewRootCmd("test")
				root.SetArgs([]string{"sync"})
				_ = root.Execute()
				want, err := os.ReadFile(tc.keep)
				if err != nil {
					t.Fatal(err)
				}

				args := []string{"revert", "--force"}
				if json {
					args = append(args, "--json")
				}
				root = NewRootCmd("test")
				root.SetOut(io.Discard)
				root.SetArgs(args)
				_ = root.Execute()

				got, err := os.ReadFile(tc.keep)
				if err != nil {
					t.Fatalf("revert --force removed %s: %v", tc.keep, err)
				}
				if string(got) != string(want) {
					t.Errorf("%s changed: %q", tc.keep, got)
				}
			})
		}
	}
}

// An output that is itself a link into the source tree is kept, so a
// restore never writes through it into the note.
func TestRevertCmd_KeepsAnOutputLinkedIntoTheSourceTree(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\ntargets: [codex]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(".agnostic-ai", "AGNOSTIC_AI.md")
	if err := os.WriteFile(note, []byte("Run tests.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(note, "AGENTS.md"); err != nil {
		t.Skipf("symlink: %v", err)
	}
	if err := os.WriteFile("AGENTS.md.bak", []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := NewRootCmd("test")
	root.SetArgs([]string{"revert", "--force"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(note); err != nil || string(got) != "Run tests.\n" {
		t.Errorf("note = %q, err=%v", got, err)
	}
}

// A spec folder at the project root would hold every output; it does not
// make revert keep them all.
func TestRevertCmd_ARootSpecFolderDoesNotKeepEveryOutput(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\nsources:\n  hooks: .\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root = NewRootCmd("test")
	root.SetArgs([]string{"revert", "-t", "claude", "--force"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("CLAUDE.md"); !os.IsNotExist(err) {
		t.Errorf("CLAUDE.md kept, err=%v", err)
	}
}

func TestRevertCmd_ReportsRestoredAndRemovedCounts(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	if err := os.WriteFile(filepath.Join(dir, "CLAUDE.md"), []byte("MY NOTES\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--backup"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--dry-run"}, "← revert claude\n    1 file(s) would be removed\n← revert entry-points\n    1 file(s) would be restored from .bak\n"},
		{nil, "← revert claude\n    1 file(s) removed\n← revert entry-points\n    1 file(s) restored from .bak\n"},
		// The restored CLAUDE.md has no .bak left, so --force removes it.
		{nil, "← revert claude\n← revert entry-points\n    1 file(s) removed\n"},
		{[]string{"--dry-run"}, "← revert claude\n← revert entry-points\n"},
	} {
		buf := captureLog(t)
		root = NewRootCmd("test")
		root.SetArgs(append([]string{"revert", "-t", "claude", "--force"}, tc.args...))
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if got := buf.String(); got != tc.want {
			t.Errorf("revert --force %v printed:\n%s\nwant:\n%s", tc.args, got, tc.want)
		}
	}
}
