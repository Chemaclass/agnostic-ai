package config_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func TestLoad_RefusesPathsOutsideTheProject(t *testing.T) {
	abs := filepath.ToSlash(filepath.Join(t.TempDir(), "victim"))
	cases := []struct {
		name, yaml, key string
	}{
		{"gitignore path above the root", "gitignore:\n  enabled: true\n  path: ../victim/bashrc\n", "gitignore.path"},
		{"absolute gitignore path", "gitignore:\n  path: \"" + abs + "/.gitignore\"\n", "gitignore.path"},
		{"gitignore allow with a newline", "gitignore:\n  allow: [\"x\\ntouch PWNED\"]\n", "gitignore.allow"},
		{"gitignore allow with a carriage return", "gitignore:\n  allow: [\"x\\rtouch PWNED\"]\n", "gitignore.allow"},
		{"output dir above the root", "outputs:\n  codex:\n    dir: ../elsewhere\n", "outputs.codex.dir"},
		{"rules file above the root", "outputs:\n  codex:\n    rules-file: ../outside.md\n", "outputs.codex.rules-file"},
		{"rules file climbing out after a clean", "outputs:\n  codex:\n    rules-file: ./a/../../outside.md\n", "outputs.codex.rules-file"},
		{"mcp file above the root", "outputs:\n  claude:\n    mcp-file: tools/../../m.json\n", "outputs.claude.mcp-file"},
		{"setup file in the git hooks", "outputs:\n  amp:\n    setup-file: .git/hooks/post-checkout\n", "outputs.amp.setup-file"},
		{"rules dir reaching the git folder in another case", "outputs:\n  claude:\n    rules-dir: x/../.Git/hooks\n", "outputs.claude.rules-dir"},
		{"gitignore path in the git hooks", "gitignore:\n  path: .git/hooks/pre-commit\n", "gitignore.path"},
		{"unmanaged entry above the root", "sync:\n  unmanaged: [\"../victim/skills/foo/\"]\n", "sync.unmanaged"},
		{"unmanaged entry with a newline", "sync:\n  unmanaged: [\"a\\nb\"]\n", "sync.unmanaged"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeConfig(t, dir, "version: 1\ntargets: [claude, codex]\n"+tc.yaml)

			_, err := config.Load(dir)

			if err == nil || !strings.Contains(err.Error(), tc.key) {
				t.Fatalf("Load() error = %v, want one naming %s", err, tc.key)
			}
		})
	}
}

func TestLoad_AcceptsProjectRelativeOutputAndGitignorePaths(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "version: 1\ntargets: [claude, codex]\n"+
		"outputs:\n  claude:\n    rules-dir: tools/r\n    mcp-file: x/../tools/m.json\n  codex:\n    file: tools/A.md\n"+
		"gitignore:\n  path: sub/.gitignore\n  allow: [\"testdata/AGENTS.md\"]\n"+
		"sync:\n  unmanaged: [\"/AGENTS.md\", \".claude/agents/\"]\n")

	cfg, err := config.Load(dir)

	if err != nil {
		t.Fatal(err)
	}
	if cfg.Outputs["claude"].RulesDir != "tools/r" || cfg.Gitignore.Path != "sub/.gitignore" {
		t.Errorf("paths changed: %+v %+v", cfg.Outputs["claude"], cfg.Gitignore)
	}
}

func TestLoad_CleansRelativeOutputPaths(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.ToSlash(filepath.Join(dir, "tools", "rules"))
	writeConfig(t, dir, "version: 1\ntargets: [claude, codex]\n"+
		"outputs:\n  codex:\n    rules-file: ./AGENTS.md\n    dir: .codex/\n    skills-dir: a//b/./skills/\n    rules-dir: x/../rules\n"+
		"  claude:\n    rules-dir: \""+inside+"\"\n    file: CLAUDE.md\n")

	cfg, err := config.Load(dir)

	if err != nil {
		t.Fatal(err)
	}
	codex := cfg.Outputs["codex"]
	if codex.RulesFile != "AGENTS.md" || codex.Dir != ".codex" || codex.SkillsDir != "a/b/skills" {
		t.Errorf("codex paths not cleaned: rules-file %q, dir %q, skills-dir %q", codex.RulesFile, codex.Dir, codex.SkillsDir)
	}
	if codex.RulesDir != "x/../rules" {
		t.Errorf("a path with .. must keep its spelling, got rules-dir %q", codex.RulesDir)
	}
	if claude := cfg.Outputs["claude"]; claude.RulesDir != inside || claude.File != "CLAUDE.md" {
		t.Errorf("claude paths changed: rules-dir %q, file %q", claude.RulesDir, claude.File)
	}
}

// Writers join paths with backslashes on Windows and the ledger keeps
// them, so cleaning must not turn one into a slash there.
func TestCleanOutputPathOn_KeepsSpellingsThatCouldNameAnotherFile(t *testing.T) {
	for _, tc := range []struct{ in, goos, want string }{
		{"./AGENTS.md", "windows", "AGENTS.md"},
		{"AGENTS.md/", "linux", "AGENTS.md"},
		{`.\docs\AGENTS.md`, "windows", `.\docs\AGENTS.md`},
		{`.claude\settings.json`, "windows", `.claude\settings.json`},
		{`./a\b`, "linux", `a\b`},
		{"alias/../settings.json", "linux", "alias/../settings.json"},
	} {
		if got := config.CleanOutputPathOn(tc.in, tc.goos); got != tc.want {
			t.Errorf("CleanOutputPathOn(%q, %s) = %q, want %q", tc.in, tc.goos, got, tc.want)
		}
	}
}

func TestLoad_AcceptsTheLocalGitExcludeFileAsGitignorePath(t *testing.T) {
	dir := t.TempDir()
	writeConfig(t, dir, "version: 1\ntargets: [claude]\ngitignore:\n  path: .git/info/exclude\n")

	if _, err := config.Load(dir); err != nil {
		t.Fatal(err)
	}
}

func TestLoad_AcceptsAnAbsoluteOutputInsideTheProjectOnly(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.ToSlash(filepath.Join(dir, "tools", "rules"))
	writeConfig(t, dir, "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    rules-dir: \""+inside+"\"\n")
	if _, err := config.Load(dir); err != nil {
		t.Fatalf("absolute rules dir inside the project: %v", err)
	}

	hooks := filepath.ToSlash(filepath.Join(dir, ".git", "hooks"))
	writeConfig(t, dir, "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    rules-dir: \""+hooks+"\"\n")
	if _, err := config.Load(dir); err == nil || !strings.Contains(err.Error(), "outputs.claude.rules-dir") {
		t.Fatalf("absolute rules dir in .git: error = %v, want one naming outputs.claude.rules-dir", err)
	}
}
