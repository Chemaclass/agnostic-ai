package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImport_AfterSyncUpdatesSingleFileSkillInPlace(t *testing.T) {
	const skill = "---\nname: yaml-check\ndescription: Validates YAML files.\n---\n\nRun a YAML parser on each changed file.\n"
	for _, target := range []string{"claude", "codex", "cursor", "gemini", "copilot", "opencode", "goose"} {
		t.Run(target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, cursor, gemini, copilot, opencode, goose]\n")
			writeFile(t, ".agnostic-ai/skills/yaml-check.md", skill)
			if out, err := runAbsoluteImportCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if out, err := runAbsoluteImportCLI(t, "import", target); err != nil {
				t.Fatalf("import %s: %v\n%s", target, err, out)
			}
			if _, err := os.Stat(filepath.Join(".agnostic-ai", "skills", "yaml-check")); !os.IsNotExist(err) {
				t.Errorf("import added a skill folder beside the single-file spec, stat err = %v", err)
			}
			if got := readFile(t, ".agnostic-ai/skills/yaml-check.md"); got != skill {
				t.Errorf("single-file skill changed:\n%s", got)
			}
		})
	}
}

func TestImport_NativeEditLandsInSingleFileSkill(t *testing.T) {
	const skill = "---\nname: yaml-check\ndescription: Validates YAML files.\n---\n\nRun a YAML parser on each changed file.\n"
	for target, native := range map[string]string{
		"claude": ".claude/skills/yaml-check/SKILL.md",
		"codex":  ".agents/skills/yaml-check/SKILL.md",
		"cursor": ".cursor/skills/yaml-check/SKILL.md",
	} {
		t.Run(target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\n")
			writeFile(t, ".agnostic-ai/skills/yaml-check.md", skill)
			if out, err := runAbsoluteImportCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			writeFile(t, native, strings.ReplaceAll(readFile(t, native), "each changed file", "every staged file"))
			if out, err := runAbsoluteImportCLI(t, "import", target); err != nil {
				t.Fatalf("import %s: %v\n%s", target, err, out)
			}
			if _, err := os.Stat(filepath.Join(".agnostic-ai", "skills", "yaml-check")); !os.IsNotExist(err) {
				t.Errorf("import added a skill folder beside the single-file spec, stat err = %v", err)
			}
			if got := readFile(t, ".agnostic-ai/skills/yaml-check.md"); !strings.Contains(got, "every staged file") {
				t.Errorf("native edit did not reach the single-file skill:\n%s", got)
			}
		})
	}
}

func TestImportCursor_ScopedLinkToASingleFileSkillImports(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	writeFile(t, ".agnostic-ai/skills/platforma.md", "---\nname: platforma\ndescription: Triage.\n---\n\nTriage it.\n")
	if out, err := runAbsoluteImportCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	link := filepath.Join("apps", "platforma", ".cursor", "skills", "platforma")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "..", "..", "..", ".cursor", "skills", "platforma"), link); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	if out, err := runAbsoluteImportCLI(t, "import", "cursor"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".agnostic-ai", "skills", "platforma")); !os.IsNotExist(err) {
		t.Errorf("import added a skill folder beside the single-file spec, stat err = %v", err)
	}
}

func TestImportClaude_IgnoresFinderMetadataBesideASingleFileSkill(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".agnostic-ai/skills/yaml-check.md", "---\nname: yaml-check\ndescription: Validates YAML files.\n---\n\nCheck it.\n")
	if out, err := runAbsoluteImportCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	writeFile(t, ".claude/skills/yaml-check/.DS_Store", "finder")
	if out, err := runAbsoluteImportCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(".agnostic-ai", "skills", "yaml-check")); !os.IsNotExist(err) {
		t.Errorf("import added a skill folder beside the single-file spec, stat err = %v", err)
	}
}
