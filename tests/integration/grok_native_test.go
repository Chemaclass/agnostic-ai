package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrokNativeProject_ImportSyncCheckAndLint(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	if os.PathSeparator == '\\' {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	project := t.TempDir()
	files := map[string]string{
		"agnostic-ai.yaml":                "version: 1\ntargets: [grok]\ngitignore:\n  enabled: true\n  commit: [grok:instructions]\n",
		".grok/rules/style.md":            "Use Conventional Commits.\n",
		"backend/.grok/rules/database.md": "Use transactions.\n",
		".grok/agents/reviewer.md":        "---\nname: reviewer\ndescription: Review changes\ntools: [Read]\neffort: high\n---\n\nFind defects.\n",
		".grok/skills/review/SKILL.md":    "---\nname: review\ndescription: Review work\n---\n\nRead [guide](guide.txt).\n",
		".grok/skills/review/guide.txt":   "Check the tests.\n",
		".grok/commands/check.md":         "---\ndescription: Check work\n---\n\nRun the project checks.\n",
		".grok/hooks/guard.json":          `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo safe","timeout":7}]}]}}`,
		".grok/config.toml":               "[mcp_servers.docs]\ncommand = \"server\"\nenabled = false\n[permission]\ndeny = [\"Bash(git push *)\"]\n",
	}
	for path, body := range files {
		path = filepath.Join(project, path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git := exec.Command("git", "init", "--quiet", project)
	if output, err := git.CombinedOutput(); err != nil {
		t.Fatalf("git: %v %s", err, output)
	}
	run := func(args ...string) {
		t.Helper()
		command := exec.Command(binary, args...)
		command.Dir = project
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, output)
		}
	}
	run("import", "grok")
	run("sync", "-t", "grok")
	run("sync", "--check")
	run("lint")
	for path, want := range map[string]string{".grok/rules/style.md": "Conventional Commits", "backend/.grok/rules/database.md": "transactions", ".grok/agents/reviewer.md": "Read", ".grok/skills/review/guide.txt": "Check the tests.", ".grok/commands/check.md": "Run the project checks.", ".grok/hooks/guard.json": "PreToolUse", ".grok/config.toml": "enabled = false"} {
		data, err := os.ReadFile(filepath.Join(project, path))
		if err != nil || !strings.Contains(string(data), want) {
			t.Errorf("%s missing %q: %v", path, want, err)
		}
	}
	visibility := exec.Command("git", "-C", project, "check-ignore", "--no-index", ".grok/rules/style.md", "backend/.grok/rules/database.md")
	if output, err := visibility.CombinedOutput(); err == nil || len(output) > 0 {
		t.Fatalf("instructions ignored: %v %s", err, output)
	}
	run("import", "grok")
	run("sync", "-t", "grok")
	run("sync", "--check")
	run("lint")
}
