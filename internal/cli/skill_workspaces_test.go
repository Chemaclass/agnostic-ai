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

// syncedWorkspaceSkill is a project whose skill lists workspaces and that
// sync wrote with shared skills, so the root `.cursor/skills/web-skill`
// and `apps/web/.cursor/skills/web-skill` both link to one folder.
func syncedWorkspaceSkill(t *testing.T, skill string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	silence(t)
	captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, cursor]\nsync:\n  shared-skills: true\n")
	writeFile(t, filepath.Join("apps", "web", "index.txt"), "x\n")
	writeFile(t, filepath.Join("apps", "old", "index.txt"), "x\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "web-skill", "SKILL.md"), skill)
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(skill, "apps/web") {
		return
	}
	if info, err := os.Lstat(filepath.Join("apps", "web", ".cursor", "skills", "web-skill")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Skipf("sync wrote no linked workspace copy: %v", err)
	}
}

const workspaceSkillSpec = ".agnostic-ai/skills/web-skill/SKILL.md"

// An import that finds the workspace the spec already lists leaves the
// spec as written, however often it runs, and lint still passes (#2202).
func TestImportCursor_LeavesAMatchingWorkspacesKeyAlone(t *testing.T) {
	for name, skill := range map[string]string{
		"flow list":        "---\nname: web-skill\nworkspaces: [apps/web]\ndescription: Web skill.\n---\n\nBody.\n",
		"comment above":    "---\nname: web-skill\n# Cursor needs a copy in the web app.\nworkspaces: [apps/web]\ndescription: Web skill.\n---\n\nBody.\n",
		"block list":       "---\nname: web-skill\nworkspaces:\n  - apps/web\ndescription: Web skill.\n---\n\nBody.\n",
		"trailing slash":   "---\nname: web-skill\nworkspaces: [apps/web/]\ndescription: Web skill.\n---\n\nBody.\n",
		"last key in file": "---\nname: web-skill\ndescription: Web skill.\nworkspaces: [apps/web]\n---\n\nBody.\n",
	} {
		t.Run(name, func(t *testing.T) {
			syncedWorkspaceSkill(t, skill)
			for run := 1; run <= 2; run++ {
				if out, err := runCLI(t, "import", "cursor"); err != nil {
					t.Fatalf("import run %d: %v\n%s", run, err, out)
				}
				if got := readFile(t, workspaceSkillSpec); got != skill {
					t.Fatalf("import run %d rewrote the spec:\n%s", run, got)
				}
			}
			if out, err := runCLI(t, "lint"); err != nil {
				t.Errorf("lint: %v\n%s", err, out)
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v\n%s", err, out)
			}
		})
	}
}

// A workspace list that differs from the native tree is replaced where it
// stands, so the key is written once and the rest of the spec is kept.
func TestImportCursor_ReplacesAWorkspacesKeyThatDiffers(t *testing.T) {
	for name, tc := range map[string]struct{ listed, want string }{
		"flow list":    {"workspaces: [apps/web, apps/old]\n", "workspaces: [apps/web]\n"},
		"block list":   {"workspaces:\n  - apps/web\n  - apps/old\n", "workspaces: [apps/web]\n"},
		"comment kept": {"# Cursor copies.\nworkspaces: [apps/web, apps/old]\n", "# Cursor copies.\nworkspaces: [apps/web]\n"},
		"line comment": {"workspaces: [apps/web, apps/old] # both\n", "workspaces: [apps/web] # both\n"},
		"reordered":    {"workspaces: [apps/old, apps/web]\n", "workspaces: [apps/web]\n"},
	} {
		t.Run(name, func(t *testing.T) {
			syncedWorkspaceSkill(t, "---\nname: web-skill\n"+tc.listed+"description: Web skill.\n---\n\nBody.\n")
			if err := os.RemoveAll(filepath.Join("apps", "old", ".cursor")); err != nil {
				t.Fatal(err)
			}
			if out, err := runCLI(t, "import", "cursor"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			got := readFile(t, workspaceSkillSpec)
			want := "---\nname: web-skill\n" + tc.want + "description: Web skill.\n---\n\nBody.\n"
			if got != want {
				t.Errorf("spec after import:\n%s\nwant:\n%s", got, want)
			}
			if out, err := runCLI(t, "import", "cursor"); err != nil {
				t.Fatalf("second import: %v\n%s", err, out)
			}
			if after := readFile(t, workspaceSkillSpec); after != want {
				t.Errorf("a second import rewrote the spec:\n%s", after)
			}
		})
	}
}

// A spec that lists no workspace gets the key once, however often import
// runs.
func TestImportCursor_AddsAWorkspacesKeyTheSpecLacks(t *testing.T) {
	syncedWorkspaceSkill(t, "---\nname: web-skill\ndescription: Web skill.\n---\n\nBody.\n")
	link := filepath.Join("apps", "web", ".cursor", "skills", "web-skill")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "..", "..", ".agents", "skills", "web-skill"), link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	want := "---\nname: web-skill\ndescription: Web skill.\nworkspaces: [apps/web]\n---\n\nBody.\n"
	for run := 1; run <= 2; run++ {
		if out, err := runCLI(t, "import", "cursor"); err != nil {
			t.Fatalf("import run %d: %v\n%s", run, err, out)
		}
		if got := readFile(t, workspaceSkillSpec); got != want {
			t.Fatalf("spec after import run %d:\n%s\nwant:\n%s", run, got, want)
		}
	}
}

func TestLint_WarnsOnScopeInSkillFrontmatter(t *testing.T) {
	got := lintSkillScopeKey([]spec.Entry{{Path: "skills/p/SKILL.md", Meta: map[string]any{"scope": "apps/p"}}, {Path: "skills/q/SKILL.md", Meta: map[string]any{}}})
	if len(got) != 1 || got[0].Code != "LINT018" {
		t.Errorf("findings = %v", got)
	}
}

// A hook spec's fields are its content, so one with a command and no
// description is not empty.
func TestLint_HookWithACommandIsNotEmpty(t *testing.T) {
	got := lintEmptySpecs([]spec.Entry{
		{Kind: spec.KindHook, Path: "hooks/fmt.yaml", Meta: map[string]any{"event": "PostToolUse", "command": "pnpm fmt"}},
		{Kind: spec.KindRule, Path: "rules/empty.md", Meta: map[string]any{}},
	})
	if len(got) != 1 || got[0].Path != "rules/empty.md" {
		t.Errorf("findings = %v", got)
	}
}
