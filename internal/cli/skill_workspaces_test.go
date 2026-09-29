package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// A root skill with `workspaces` stays at the root for every tool and gets
// a copy under each workspace for Cursor, which loads skills only from the
// workspace it opens. The key never reaches a SKILL.md.
func TestSync_SkillWorkspacesCopyTheSkillForCursor(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "platforma", "SKILL.md"), "---\nname: platforma\ndescription: Triage an incident.\nworkspaces: [apps/platforma]\n---\nTriage it.\n")
	if _, err := runCLI(t, "sync"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".cursor/skills/platforma/SKILL.md", "apps/platforma/.cursor/skills/platforma/SKILL.md", ".claude/skills/platforma/SKILL.md"} {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			t.Errorf("%s not written: %v", p, err)
			continue
		}
		if strings.Contains(string(data), "workspaces") {
			t.Errorf("%s carries the workspaces key:\n%s", p, data)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "apps", "platforma", ".claude")); !os.IsNotExist(err) {
		t.Errorf("Claude Code got a workspace copy: %v", err)
	}
	if _, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after sync: %v", err)
	}
}

// A root link to a skill folder that lives in a workspace imports once at
// the root and records the workspace, so the next sync writes both places.
func TestImportCursor_RecordsTheWorkspaceOfALinkedSkill(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	writeFile(t, filepath.Join("apps", "platforma", ".cursor", "skills", "platforma", "SKILL.md"), "---\nname: platforma\ndescription: Triage.\n---\nTriage it.\n")
	if err := os.MkdirAll(filepath.Join(".cursor", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "apps", "platforma", ".cursor", "skills", "platforma"), filepath.Join(".cursor", "skills", "platforma")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if _, err := runCLI(t, "import", "cursor"); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(".agnostic-ai", "skills", "platforma", "SKILL.md"))
	if !strings.Contains(got, "workspaces: [apps/platforma]") {
		t.Errorf("imported spec lacks the workspace:\n%s", got)
	}
}

func TestLint_WarnsOnScopeInSkillFrontmatter(t *testing.T) {
	got := lintSkillScopeKey([]spec.Entry{{Path: "skills/p/SKILL.md", Meta: map[string]any{"scope": "apps/p"}}, {Path: "skills/q/SKILL.md", Meta: map[string]any{}}})
	if len(got) != 1 || got[0].Code != "LINT018" {
		t.Errorf("findings = %v", got)
	}
}
