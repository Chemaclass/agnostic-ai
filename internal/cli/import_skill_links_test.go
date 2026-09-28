package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// skillLinkProject lays out a project whose tool folders link some skill
// folders: `linked` to a package inside the project, `far` to a folder
// outside it.
func skillLinkProject(t *testing.T) string {
	t.Helper()
	outside := t.TempDir()
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cursor, codex]\n")
	skill := func(path, name string) {
		writeFile(t, filepath.Join(path, "SKILL.md"), "---\nname: "+name+"\ndescription: Test.\n---\n\nBody of "+name+".\n")
	}
	skill(filepath.Join(dir, ".claude/skills/real"), "real")
	skill(filepath.Join(dir, "packages/ui/skills/linked"), "linked")
	writeFile(t, filepath.Join(dir, "packages/ui/skills/linked/scripts/check.sh"), "#!/bin/sh\n")
	skill(filepath.Join(outside, "far"), "far")
	for _, tool := range []string{".claude/skills", ".cursor/skills", ".agents/skills"} {
		if err := os.MkdirAll(filepath.Join(dir, tool), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..", "..", "packages", "ui", "skills", "linked"), filepath.Join(dir, tool, "linked")); err != nil {
			t.Skipf("symlinks unsupported: %v", err)
		}
		if err := os.Symlink(filepath.Join(outside, "far"), filepath.Join(dir, tool, "far")); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestImport_ReadsSymlinkedSkillFolderInsideTheProject(t *testing.T) {
	for _, source := range []string{"claude", "cursor", "codex"} {
		t.Run(source, func(t *testing.T) {
			dir := skillLinkProject(t)
			out := captureSummary(t)
			if _, err := runCLI(t, "import", source); err != nil {
				t.Fatalf("import %s: %v", source, err)
			}
			for _, want := range []string{"linked/SKILL.md", "linked/scripts/check.sh"} {
				if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai/skills", want)); err != nil {
					t.Errorf("symlinked skill folder not imported: %v", err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai/skills/far")); !os.IsNotExist(err) {
				t.Errorf("a skill folder linked from outside the project was imported: %v", err)
			}
			if !strings.Contains(out.String(), "skipped "+filepath.Join(nativeSkillsDirFor(source), "far")) {
				t.Errorf("no note names the skipped outside link:\n%s", out.String())
			}
		})
	}
}

func TestImport_DryRunListsSymlinkedSkillFoldersLikeTheRealImport(t *testing.T) {
	skillLinkProject(t)
	out := captureSummary(t)
	stdout := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "--dry-run"); err != nil {
			t.Fatalf("import --dry-run: %v", err)
		}
	})
	if !strings.Contains(stdout, filepath.FromSlash(".agnostic-ai/skills/linked/SKILL.md")) {
		t.Errorf("dry-run lost the symlinked skill:\n%s", stdout)
	}
	if strings.Contains(stdout, filepath.FromSlash("skills/far/")) {
		t.Errorf("dry-run lists a skill folder linked from outside the project:\n%s", stdout)
	}
	if !strings.Contains(out.String(), "skipped "+filepath.Join(".claude", "skills", "far")) {
		t.Errorf("dry-run gives no note for the skipped outside link:\n%s", out.String())
	}
}

// A whole skills directory linked from elsewhere, such as dotfiles, is
// the tool folder the user chose: its skills still import.
func TestImport_ReadsSkillsDirectoryLinkedFromOutside(t *testing.T) {
	outside := t.TempDir()
	writeFile(t, filepath.Join(outside, "shared/SKILL.md"), "---\nname: shared\ndescription: Test.\n---\n\nBody.\n")
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, ".claude/skills")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai/skills/shared/SKILL.md")); err != nil {
		t.Errorf("skill under a linked skills directory not imported: %v", err)
	}
}

// A link to a folder that holds the skill sources would copy the folder
// into itself; a link into them is already a spec. Both import nothing.
func TestImport_SkipsSkillLinksThatHoldOrPointIntoTheSources(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: root\ndescription: Test.\n---\n\nBody.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/skills/kept/SKILL.md"), "---\nname: kept\ndescription: Test.\n---\n\nKept.\n")
	if err := os.MkdirAll(filepath.Join(dir, ".claude/skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, to := range map[string]string{"selfskill": "../..", "kept": "../../.agnostic-ai/skills/kept"} {
		if err := os.Symlink(filepath.FromSlash(to), filepath.Join(dir, ".claude/skills", name)); err != nil {
			t.Skipf("symlinks unsupported: %v", err)
		}
	}
	if _, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai/skills/selfskill")); !os.IsNotExist(err) {
		t.Errorf("a link to the project root was imported as a skill: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai/skills/kept/SKILL.md"))
	if err != nil || !strings.Contains(string(data), "Kept.") {
		t.Errorf("a link into the sources changed the spec (%v):\n%s", err, data)
	}
}

// The same holds for a scoped link, whose destination does not exist yet,
// under a root reached through a symlink (a macOS temp dir).
func TestImportScopedSkillFolders_SkipsAScopedLinkToTheProjectRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "SKILL.md"), "---\nname: root\ndescription: Test.\n---\n\nBody.\n")
	if err := os.MkdirAll(filepath.Join(root, "pkg/.cursor/skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.FromSlash("../../.."), filepath.Join(root, "pkg/.cursor/skills/rootskill")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	count, err := importScopedSkillFolders(root, filepath.Join(".cursor", "skills"), filepath.Join(root, ".agnostic-ai/skills"))
	if err != nil || count != 0 {
		t.Errorf("a scoped link to the project root imported %d skills (%v)", count, err)
	}
}

// A skill linked into a directory the preview copy leaves out, such as
// node_modules, still previews like the real import.
func TestImport_DryRunFollowsSkillLinksIntoLeftOutDirectories(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(dir, "node_modules/pkg/skills/nm/SKILL.md"), "---\nname: nm\ndescription: Test.\n---\n\nBody.\n")
	if err := os.MkdirAll(filepath.Join(dir, ".claude/skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.FromSlash("../../node_modules/pkg/skills/nm"), filepath.Join(dir, ".claude/skills/nm")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	stdout := captureStdout(t, func() {
		if _, err := runCLI(t, "import", "claude", "--dry-run"); err != nil {
			t.Fatalf("import --dry-run: %v", err)
		}
	})
	if !strings.Contains(stdout, filepath.FromSlash(".agnostic-ai/skills/nm/SKILL.md")) {
		t.Errorf("dry-run lost a skill linked into node_modules:\n%s", stdout)
	}
}

func nativeSkillsDirFor(source string) string {
	return map[string]string{
		"claude": filepath.Join(".claude", "skills"),
		"cursor": filepath.Join(".cursor", "skills"),
		"codex":  filepath.Join(".agents", "skills"),
	}[source]
}
