package cli

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// commitKindsFixture syncs a project with one spec of each kind the
// acceptance for #1332 names, under `gitignore.commit: [instructions, hooks]`.
func commitKindsFixture(t *testing.T, targets string) string {
	t.Helper()
	dir, _ := gitRepo(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+targets+"]\ngitignore:\n  enabled: true\n  commit: [instructions, hooks]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), "---\nname: style\ndescription: Style.\n---\nKeep it short.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "api.md"), "---\nname: api\ndescription: API rules.\nscope: services/api\n---\nVersion every route.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "fmt.yaml"), "name: fmt\nevent: PostToolUse\nmatcher: Edit\ncommand: echo hi\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review a diff.\n---\nReview it.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "fs.yaml"), "name: fs\ncommand: fs-server\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "settings", "defaults.yaml"), "model:\n  codex: gpt-5\n")
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--all"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	return dir
}

// gitIgnored reports whether git ignores the project-relative path rel.
func gitIgnored(t *testing.T, dir, rel string) bool {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "-q", "--no-index", rel)
	cmd.Dir = dir
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return true
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return false
	default:
		t.Fatalf("git check-ignore %s: %v", rel, err)
		return false
	}
}

// generatedFiles lists every file on disk outside git, the spec source,
// the config, and the .gitignore itself.
func generatedFiles(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".git" || rel == ".agnostic-ai" {
				return filepath.SkipDir
			}
			return nil
		}
		if rel != "agnostic-ai.yaml" && rel != ".gitignore" {
			out = append(out, rel)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSync_GitignoreCommitKindsKeepsInstructionsAndHooksVisible(t *testing.T) {
	dir := commitKindsFixture(t, "claude, codex, cursor")
	files := generatedFiles(t, dir)

	for _, rel := range []string{"AGENTS.md", "CLAUDE.md", ".claude/settings.json", "services/api/AGENTS.md", ".codex/hooks.json", ".cursor/hooks.json"} {
		if !slices.Contains(files, rel) {
			t.Fatalf("fixture did not generate %s; generated: %v", rel, files)
		}
	}
	var sawRule, sawSkill, sawToolSettings bool
	for _, rel := range files {
		visible := !gitIgnored(t, dir, rel)
		switch {
		case filepath.Base(rel) == "AGENTS.md", filepath.Base(rel) == "CLAUDE.md",
			rel == ".claude/settings.json", rel == ".codex/hooks.json", rel == ".cursor/hooks.json":
			if !visible {
				t.Errorf("%s is ignored; instructions and hooks must stay committed", rel)
			}
		case strings.HasPrefix(rel, ".claude/rules/"), strings.HasPrefix(rel, ".cursor/rules/"):
			sawRule = true
			if !visible {
				t.Errorf("rule output %s is ignored; instructions must stay committed", rel)
			}
		case strings.Contains(rel, "/skills/"):
			sawSkill = true
			if visible {
				t.Errorf("skill output %s is visible; skills are not in gitignore.commit", rel)
			}
		case rel == ".codex/config.toml", rel == ".cursor/mcp.json":
			sawToolSettings = true
			if visible {
				t.Errorf("tool settings %s is visible; settings and mcps are not in gitignore.commit", rel)
			}
		}
	}
	if !sawRule || !sawSkill || !sawToolSettings {
		t.Fatalf("fixture misses a kind (rule %v, skill %v, tool settings %v): %v", sawRule, sawSkill, sawToolSettings, files)
	}

	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "\n!") {
		t.Errorf("committed kinds must be left out of the block, not re-allowed:\n%s", data)
	}
}

// A committed file next to an ignored one keeps their directory expanded:
// a `/dir/` rule would hide the committed file, and no `!` line can
// re-include a file under an excluded directory.
func TestBuildManagedBlock_CommittedFileKeepsItsDirectoryExpanded(t *testing.T) {
	committed := map[string]struct{}{"/.cursor/rules/style.mdc": {}}

	block := buildManagedBlockCommitting(&config.Config{}, []string{".cursor/rules/style.mdc", ".cursor/rules/skill-review.mdc", ".cursor/skills/review/SKILL.md"}, nil, committed)

	for _, want := range []string{"/.cursor/rules/skill-review.mdc", "/.cursor/skills/"} {
		if !slices.Contains(block, want) {
			t.Errorf("block missing %q: %v", want, block)
		}
	}
	for _, unwanted := range []string{"/.cursor/rules/", "/.cursor/rules/style.mdc"} {
		if slices.Contains(block, unwanted) {
			t.Errorf("block hides the committed file with %q: %v", unwanted, block)
		}
	}
}

func TestSync_GitignoreCommitKindsFollowsANewTarget(t *testing.T) {
	dir := commitKindsFixture(t, "claude, codex, cursor, gemini")

	if !slices.Contains(generatedFiles(t, dir), "GEMINI.md") {
		t.Fatal("fixture did not generate GEMINI.md")
	}
	if gitIgnored(t, dir, "GEMINI.md") {
		t.Error("GEMINI.md is ignored; a new target's instructions must stay committed with no other config change")
	}
}
