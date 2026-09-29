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

// syncCommitProject syncs a project whose specs and overlays are given as
// project-relative path to content, under the given gitignore.commit list.
func syncCommitProject(t *testing.T, targets, commit string, files map[string]string) string {
	t.Helper()
	dir, _ := gitRepo(t)
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+targets+"]\ngitignore:\n  enabled: true\n  commit: ["+commit+"]\n")
	for rel, content := range files {
		mustWriteFile(t, filepath.Join(dir, filepath.FromSlash(rel)), content)
	}
	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "--all"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	return dir
}

const claudeSettingsOverlay = `{"statusLine": {"type": "command", "command": "echo hi"}}`

// A file the config alone writes belongs to no kind: listing instructions
// must not commit the settings file an overlay produces.
func TestSync_GitignoreCommitIgnoresConfigOnlyOutputs(t *testing.T) {
	dir := syncCommitProject(t, "claude", "instructions", map[string]string{
		".agnostic-ai/overlays/claude.settings.json": claudeSettingsOverlay,
		".agnostic-ai/rules/style.md":                "---\nname: style\ndescription: Style.\n---\nKeep it short.\n",
	})

	if !slices.Contains(generatedFiles(t, dir), ".claude/settings.json") {
		t.Fatal("fixture did not generate .claude/settings.json")
	}
	if !gitIgnored(t, dir, ".claude/settings.json") {
		t.Error(".claude/settings.json is visible; instructions did not write it")
	}
	if gitIgnored(t, dir, "CLAUDE.md") || gitIgnored(t, dir, ".claude/rules/style.md") {
		t.Error("instructions outputs are ignored")
	}
}

// A kind that changes a config-driven file's content contributed to it.
func TestSync_GitignoreCommitKeepsAFileAListedKindChanges(t *testing.T) {
	dir := syncCommitProject(t, "claude", "hooks", map[string]string{
		".agnostic-ai/overlays/claude.settings.json": claudeSettingsOverlay,
		".agnostic-ai/hooks/fmt.yaml":                "name: fmt\nevent: PostToolUse\nmatcher: Edit\ncommand: echo hi\n",
	})

	if gitIgnored(t, dir, ".claude/settings.json") {
		t.Error(".claude/settings.json is ignored; its hook registrations are committed")
	}
}

// Junie writes .junie/AGENTS.md from the shared instructions with no spec,
// so only instructions decides it.
func TestSync_GitignoreCommitDecidesJunieMirrorByInstructions(t *testing.T) {
	skills := map[string]string{".agnostic-ai/skills/review/SKILL.md": "---\nname: review\ndescription: Review a diff.\n---\nReview it.\n"}

	dir := syncCommitProject(t, "junie", "skills", skills)
	if !slices.Contains(generatedFiles(t, dir), ".junie/AGENTS.md") {
		t.Fatal("fixture did not generate .junie/AGENTS.md")
	}
	if !gitIgnored(t, dir, ".junie/AGENTS.md") {
		t.Error(".junie/AGENTS.md is visible under commit: [skills]")
	}

	dir = syncCommitProject(t, "junie", "instructions", skills)
	if gitIgnored(t, dir, ".junie/AGENTS.md") {
		t.Error(".junie/AGENTS.md is ignored under commit: [instructions]")
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

// `<target>:<kind>` commits a kind for one target only: the Cursor
// environment files a cloud agent reads from Git stay visible, while the
// Claude Code and Codex files the same spec writes stay ignored.
func TestSync_GitignoreCommitScopesAKindToOneTarget(t *testing.T) {
	dir := syncCommitProject(t, "claude, codex, cursor", "cursor:environments", map[string]string{
		".agnostic-ai/environments/dev.yaml": "name: dev\nsetup: npm ci\ninstall: npm ci\ndev-commands:\n  - name: Docs\n    command: npm run docs\n",
	})

	for _, p := range []string{".cursor/environment.json", ".cursor/worktrees.json", ".claude/launch.json", ".codex/environments/environment.toml"} {
		if !slices.Contains(generatedFiles(t, dir), p) {
			t.Fatalf("fixture did not generate %s", p)
		}
	}
	for _, p := range []string{".cursor/environment.json", ".cursor/worktrees.json"} {
		if gitIgnored(t, dir, p) {
			t.Errorf("%s is ignored under commit: [cursor:environments]", p)
		}
	}
	for _, p := range []string{".claude/launch.json", ".codex/environments/environment.toml"} {
		if !gitIgnored(t, dir, p) {
			t.Errorf("%s is visible; only cursor's environments are committed", p)
		}
	}
}

func TestLint_WarnsOnGitignoreCommitForAnUnconfiguredTarget(t *testing.T) {
	cfg := &config.Config{Targets: []string{"claude", "cursor"}, Gitignore: config.Gitignore{Commit: []string{"cursor:reviews", "cursr:environments", "hooks"}}}
	got := lintGitignoreCommitTargets(cfg)
	if len(got) != 1 || got[0].Code != "LINT017" || !strings.Contains(got[0].Message, "cursr") {
		t.Errorf("findings = %v, want one LINT017 for cursr", got)
	}
}
