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

func TestImportFlatSkill(t *testing.T) {
	for _, source := range []string{"claude", "cursor"} {
		t.Run(source, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+source+"]\n")
			const flat = ".agnostic-ai/skills/checklist.md"
			const skill = "---\nname: demo\ndescription: Demo skill. Use when testing.\n# keep this comment\n---\n\nOriginal body.\n"
			writeFile(t, flat, skill)
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			native := sharedSkillNative[source]

			preview := captureStdout(t, func() {
				if _, err := runCLI(t, "import", source, "--dry-run", "--diff"); err != nil {
					t.Errorf("preview: %v", err)
				}
			})
			if !strings.Contains(preview, "unchanged "+flat) {
				t.Errorf("preview did not report the flat skill unchanged:\n%s", preview)
			}
			if out, err := runCLI(t, "import", source); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			if got := readFile(t, flat); got != skill {
				t.Errorf("unedited import changed the flat skill:\n%s", got)
			}
			if _, err := os.Stat(".agnostic-ai/skills/demo"); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("import created a second skill folder: %v", err)
			}

			writeFile(t, native, strings.Replace(readFile(t, native), "Original body.", "Edited body.", 1))
			if out, err := runCLI(t, "import", source); err != nil {
				t.Fatalf("import edit: %v\n%s", err, out)
			}
			if got := readFile(t, flat); !strings.Contains(got, "Edited body.") || !strings.Contains(got, "# keep this comment") {
				t.Errorf("import did not bring back the edit and keep the comment:\n%s", got)
			}
			if _, err := os.Stat(".agnostic-ai/skills/demo"); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("import created a second skill folder: %v", err)
			}
			if out, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v\n%s", err, out)
			}
		})
	}
}

func TestImportFlatSkillWithAssetsExplainsHowToKeepThem(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	const flat = ".agnostic-ai/skills/demo.md"
	const folder = ".agnostic-ai/skills/demo/SKILL.md"
	const skill = "---\nname: demo\ndescription: Demo skill. Use when testing.\n---\n\nPortable body.\n"
	const asset = ".claude/skills/demo/references/extra.md"
	writeFile(t, flat, skill)
	writeFile(t, sharedSkillNative["claude"], skill)
	writeFile(t, asset, "Extra reference.\n")
	if _, err := runCLI(t, "import", "claude"); err == nil || !strings.Contains(filepath.ToSlash(err.Error()), "move "+flat) {
		t.Errorf("import did not explain how to keep the bundled reference: %v", err)
	}
	if got := readFile(t, flat); got != skill {
		t.Errorf("import overwrote the flat skill:\n%s", got)
	}
	if _, err := os.Stat(folder); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("import created a second skill: %v", err)
	}
	if err := os.Remove(flat); err != nil {
		t.Fatal(err)
	}
	writeFile(t, folder, skill)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import into a folder: %v\n%s", err, out)
	}
	if got := readFile(t, ".agnostic-ai/skills/demo/references/extra.md"); got != "Extra reference.\n" {
		t.Errorf("import changed the bundled reference: %q", got)
	}
}

func TestImportFlatSkillProtectsIndependentEdit(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	const flat = ".agnostic-ai/skills/demo.md"
	const skill = "---\nname: demo\ndescription: Demo skill. Use when testing.\n---\n\nPortable body.\n"
	writeFile(t, flat, skill)
	writeFile(t, sharedSkillNative["claude"], strings.Replace(skill, "Portable body.", "Independent native body.", 1))
	if out, err := runCLI(t, "import", "claude"); errs.CodeOf(err) != errs.CodeImportWouldReplace {
		t.Errorf("import: %v\n%s", err, out)
	}
	if got := readFile(t, flat); got != skill {
		t.Errorf("import overwrote the flat skill:\n%s", got)
	}
	if _, err := os.Stat(".agnostic-ai/skills/demo"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("import created a second skill folder: %v", err)
	}
}

func TestImportFlatSkillWithCodexGeneratedAsset(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	const flat = ".agnostic-ai/skills/demo.md"
	const skill = "---\nname: demo\ndescription: Demo skill. Use when testing.\ndisable-model-invocation: true\n---\n\nPortable body.\n"
	writeFile(t, flat, skill)
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if _, err := os.Stat(".agents/skills/demo/agents/openai.yaml"); err != nil {
		t.Fatal(err)
	}
	if out, err := runCLI(t, "import", "codex"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if got := readFile(t, flat); got != skill {
		t.Errorf("import changed the flat skill:\n%s", got)
	}
	if _, err := os.Stat(".agnostic-ai/skills/demo"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("import created a second skill folder: %v", err)
	}
}

func TestImportFlatSkillKeepsWorkspaceFromScopedLink(t *testing.T) {
	root := t.TempDir()
	testutil.Chdir(t, root)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	const skill = "---\nname: ops\ndescription: Operations.\n---\n\nBody.\n"
	const flat = ".agnostic-ai/skills/ops.md"
	native := filepath.Join("packages", "ops", ".cursor", "skills", "ops")
	writeFile(t, flat, strings.Replace(skill, "description: Operations.\n", "description: Operations.\nworkspaces: [packages/ops]\n", 1))
	writeFile(t, filepath.Join(native, "SKILL.md"), skill)
	link := filepath.Join(".cursor", "skills", "ops")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", native), link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if out, err := runCLI(t, "import", "cursor"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if got := readFile(t, flat); !strings.Contains(got, "workspaces: [packages/ops]") {
		t.Errorf("flat skill has no workspace association:\n%s", got)
	}
	for _, path := range []string{".agnostic-ai/skills/ops", ".agnostic-ai/skills/packages"} {
		if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("import added a second spec at %s: %v", path, err)
		}
	}
}

func TestFlatSkillPaths_SkipsMarkdownBundledWithAncestorSkill(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "ops", "SKILL.md"), "Operations.\n")
	scoped := filepath.Join(root, "ops", "references")
	writeFile(t, filepath.Join(scoped, "notes.md"), "Reference notes.\n")
	if got := flatSkillPaths(scoped, "ops/references"); len(got) != 0 {
		t.Errorf("bundled Markdown matched as a flat skill: %v", got)
	}
}

func TestImportFlatSkillSkipsUnchangedCursorWorkspaceCopies(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	const source = ".agnostic-ai/skills/ops.md"
	const skill = "---\nname: ops\ndescription: Operations.\nworkspaces: [packages/ops]\n---\n\nBody.\n"
	writeFile(t, source, skill)
	writeFile(t, "packages/ops/README.md", "Workspace.\n")
	execCLI(t, "sync")
	if _, err := os.Stat("packages/ops/.cursor/skills/ops/SKILL.md"); err != nil {
		t.Fatal(err)
	}
	execCLI(t, "import", "cursor")
	if got := readFile(t, source); got != skill {
		t.Errorf("flat skill changed:\n%s", got)
	}
	if _, err := os.Stat(".agnostic-ai/skills/packages"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("import added a duplicate from the workspace copy: %v", err)
	}
}
