package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// writePreviewFixture lays out the acceptance scenario of #1035 under
// dir: distinct CLAUDE.md and AGENTS.md instructions, an existing
// canonical entry point, and a canonical rule the claude import changes.
func writePreviewFixture(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "# Claude\n\nClaude body.\n")
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Agents\n\nAgents body.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Old shared body.\n")
	writeFile(t, filepath.Join(dir, ".claude", "rules", "style.md"), "---\ndescription: style\n---\n\nUse tabs.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\ndescription: style\n---\n\nUse spaces.\n")
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), `{"model":"opus"}`)
}

// writeSharedSkill adds one skill both tools define. Codex merges its
// copy into the one claude imported first, so the preview must let the
// later source read the earlier planned write.
func writeSharedSkill(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, ".claude", "skills", "review", "SKILL.md"),
		"---\nname: review\ndescription: Review code\n---\n\nClaude review steps.\n")
	writeFile(t, filepath.Join(dir, ".agents", "skills", "review", "SKILL.md"),
		"---\nname: review\ndescription: Review code\n---\n\nCodex review steps.\n")
}

func TestImport_DiffRequiresDryRun(t *testing.T) {
	dir := testutil.TempCwd(t)
	writePreviewFixture(t, dir)

	_, err := runCLI(t, "import", "claude", "--diff")

	if err == nil {
		t.Fatal("expected --diff without --dry-run to fail")
	}
	if !strings.Contains(err.Error(), "--dry-run") {
		t.Errorf("error should name --dry-run, got %v", err)
	}
	if got := errs.CodeOf(err); got != errs.CodeFlagConflict {
		t.Errorf("code=%q, want %q", got, errs.CodeFlagConflict)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "overlays")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("a rejected flag combination must not import: %v", err)
	}
}

func TestImport_DryRunDiffShowsCreatedChangedAndUnchanged(t *testing.T) {
	dir := testutil.TempCwd(t)
	writePreviewFixture(t, dir)
	writeFile(t, filepath.Join(dir, ".claude", "rules", "same.md"), "---\ndescription: same\n---\n\nKeep me.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "same.md"), "---\ndescription: same\n---\n\nKeep me.\n")

	stdout := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "--dry-run", "--diff"); err != nil {
			t.Fatalf("import: %v", err)
		}
	})

	for _, want := range []string{
		"create    .agnostic-ai/overlays/claude.settings.json",
		"change    .agnostic-ai/rules/style.md",
		"unchanged .agnostic-ai/rules/same.md",
		"--- .agnostic-ai/rules/style.md (current)",
		"+++ .agnostic-ai/rules/style.md (after import)",
		"-Use spaces.",
		"+Use tabs.",
		"--- /dev/null",
		"+++ .agnostic-ai/overlays/claude.settings.json (after import)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("preview missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "-Keep me.") || strings.Contains(stdout, "+Keep me.") {
		t.Errorf("an unchanged destination must not print a diff:\n%s", stdout)
	}
}

// The acceptance scenario: two sources write different entry points to
// the one canonical AGNOSTIC_AI.md. The preview names both, the import
// order, and the source whose bytes a real import keeps.
func TestImport_DryRunDiffReportsCompetingEntryPointAndWinner(t *testing.T) {
	dir := testutil.TempCwd(t)
	writePreviewFixture(t, dir)
	writeSharedSkill(t, dir)
	before := snapshotProject(t, dir)

	stdout := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "codex", "--dry-run", "--diff"); err != nil {
			t.Fatalf("import: %v", err)
		}
	})

	for _, want := range []string{
		"import order: 1. claude, 2. codex",
		"change    .agnostic-ai/AGNOSTIC_AI.md  (claude, codex)",
		"! conflict .agnostic-ai/AGNOSTIC_AI.md: claude, codex propose different content; codex (last) is kept",
		"-Old shared body.",
		"+Agents body.",
		"! conflict .agnostic-ai/skills/review/SKILL.md: claude, codex propose different content; codex (last) is kept",
		"+Codex review steps.",
		"2 conflict(s)",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("preview missing %q:\n%s", want, stdout)
		}
	}
	assertProjectUnchanged(t, before, snapshotProject(t, dir))
}

// Two sources proposing the same bytes for one destination agree, so
// the preview must not flag them.
func TestImport_DryRunDiffIgnoresIdenticalProposals(t *testing.T) {
	dir := testutil.TempCwd(t)
	writePreviewFixture(t, dir)
	writeFile(t, filepath.Join(dir, "AGENTS.md"), "# Claude\n\nClaude body.\n")

	stdout := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "codex", "--dry-run", "--diff"); err != nil {
			t.Fatalf("import: %v", err)
		}
	})

	if !strings.Contains(stdout, "change    .agnostic-ai/AGNOSTIC_AI.md  (claude, codex)") {
		t.Errorf("both sources should be listed as writers:\n%s", stdout)
	}
	if strings.Contains(stdout, "! conflict .agnostic-ai/AGNOSTIC_AI.md") {
		t.Errorf("identical proposals are not a conflict:\n%s", stdout)
	}
}

// The preview runs the real importers, so its planned final bytes must
// equal what an ordinary sequential import leaves on disk, and it must
// plan every file that import changes.
func TestImport_PreviewFinalBytesMatchRealImport(t *testing.T) {
	// Same base name: codex names a shredded rule after the project dir.
	previewDir := filepath.Join(t.TempDir(), "project")
	realDir := filepath.Join(t.TempDir(), "project")
	for _, dir := range []string{previewDir, realDir} {
		writePreviewFixture(t, dir)
		writeSharedSkill(t, dir)
	}
	silence(t)
	args := []string{"claude", "codex"}

	testutil.Chdir(t, previewDir)
	preview, err := planImportPreview(args)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(preview.entries) == 0 {
		t.Fatal("preview planned no writes")
	}

	before := snapshotProject(t, realDir)
	testutil.Chdir(t, realDir)
	if _, err := runCLI(t, append([]string{"import"}, args...)...); err != nil {
		t.Fatalf("real import: %v", err)
	}
	after := snapshotProject(t, realDir)

	planned := map[string]bool{}
	for _, e := range preview.entries {
		planned[filepath.FromSlash(e.path)] = true
		got, err := os.ReadFile(filepath.Join(realDir, filepath.FromSlash(e.path)))
		if err != nil {
			t.Errorf("real import did not write %s: %v", e.path, err)
			continue
		}
		if string(got) != string(e.after) {
			t.Errorf("%s: preview planned\n%q\nreal import wrote\n%q", e.path, e.after, got)
		}
	}
	for p, v := range after {
		if before[p] == v || strings.HasPrefix(v, "<dir") {
			continue
		}
		if !planned[p] {
			t.Errorf("real import changed %s but the preview did not plan it", p)
		}
	}
}

// Plain --dry-run keeps its path-only shape when --diff is absent, even
// for a multi-source run.
func TestImport_DryRunWithoutDiffStillOmitsBodies(t *testing.T) {
	dir := testutil.TempCwd(t)
	writePreviewFixture(t, dir)

	stdout := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "codex", "--dry-run"); err != nil {
			t.Fatalf("import: %v", err)
		}
	})

	if !strings.Contains(stdout, "would write "+filepath.Join(".agnostic-ai", "AGNOSTIC_AI.md")) {
		t.Errorf("expected path lines:\n%s", stdout)
	}
	for _, leak := range []string{"Agents body.", "Use tabs.", "(after import)", "conflict"} {
		if strings.Contains(stdout, leak) {
			t.Errorf("plain dry-run leaked %q:\n%s", leak, stdout)
		}
	}
}

// A canonical directory symlinked outside the project would let a real
// import write through it. The preview copies such a link by content,
// so the outside file stays as it was while the diff still shows it.
func TestImport_DryRunDiffNeverWritesThroughOutsideSymlink(t *testing.T) {
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "style.md"), "---\ndescription: style\n---\n\nUse spaces.\n")
	dir := testutil.TempCwd(t)
	writePreviewFixture(t, dir)
	rules := filepath.Join(dir, ".agnostic-ai", "rules")
	if err := os.Remove(filepath.Join(rules, "style.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(rules); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, rules); err != nil {
		t.Fatal(err)
	}
	before := snapshotProject(t, outside)

	stdout := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "--dry-run", "--diff"); err != nil {
			t.Fatalf("import: %v", err)
		}
	})

	if !strings.Contains(stdout, "change    .agnostic-ai/rules/style.md") {
		t.Errorf("the linked rule should show as changed:\n%s", stdout)
	}
	assertProjectUnchanged(t, before, snapshotProject(t, outside))
}

// A symlink inside the project is recreated as a link in the copy, so
// both paths keep reading the same file. .git is never copied.
func TestCopyImportPreviewTree_KeepsInProjectSymlinksAndSkipsGit(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, filepath.Join(src, "AGENTS.md"), "shared\n")
	writeFile(t, filepath.Join(src, ".git", "HEAD"), "ref\n")
	if err := os.Symlink("AGENTS.md", filepath.Join(src, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}

	if _, err := copyImportPreviewTree(src, dst); err != nil {
		t.Fatalf("copy: %v", err)
	}

	link, err := os.Readlink(filepath.Join(dst, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md should stay a symlink: %v", err)
	}
	if link != "AGENTS.md" {
		t.Errorf("link target=%q, want AGENTS.md", link)
	}
	if _, err := os.Stat(filepath.Join(dst, ".git")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf(".git must not be copied: %v", err)
	}
}

// The copy turns a link leaving the project into a regular file, yet an
// `import all` preview must still skip it, as the real run does.
func TestPlanImportPreview_ImportAllNotesEntryFileLinkedOutsideTheProject(t *testing.T) {
	outside := filepath.Join(t.TempDir(), "CLAUDE.md")
	writeFile(t, outside, "# elsewhere\n")
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\n")
	writeFile(t, filepath.Join(dir, ".claude", "settings.json"), "{}\n")
	if err := os.Symlink(outside, filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	testutil.Chdir(t, dir)
	out := captureSummary(t)

	if _, err := planImportPreview([]string{"all"}); err != nil {
		t.Fatalf("preview: %v", err)
	}

	if got := strings.Count(out.String(), "skipped CLAUDE.md"); got != 1 {
		t.Errorf("noted the skipped entry file %d times, want once:\n%s", got, out.String())
	}
}

func TestRemoveImportPreviewDir_RefusesPathsOutsideTempDir(t *testing.T) {
	victim := filepath.Join(t.TempDir(), importPreviewDirPrefix+"x")
	writeFile(t, filepath.Join(victim, "keep.md"), "keep\n")

	removeImportPreviewDir(victim)
	removeImportPreviewDir("")

	if _, err := os.Stat(filepath.Join(victim, "keep.md")); err != nil {
		t.Errorf("a path outside the temp dir root was deleted: %v", err)
	}
}
