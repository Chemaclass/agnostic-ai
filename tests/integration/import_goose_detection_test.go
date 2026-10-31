package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestImportAntigravityPlugin_DoesNotDetectGoose(t *testing.T) {
	packageDir, err := os.Getwd()
	must(t, err)
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}

	dir := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		path = filepath.Join(dir, filepath.FromSlash(path))
		must(t, os.MkdirAll(filepath.Dir(path), 0o755))
		must(t, os.WriteFile(path, []byte(content), 0o644))
	}
	write("agnostic-ai.yaml", "version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: .agents/plugins/team/mcp.json\n")
	write(".agents/plugins/team/plugin.json", "{\"name\":\"team\"}\n")
	write(".agents/plugins/team/mcp.json", "{\"mcpServers\":{\"docs\":{\"serverUrl\":\"https://example.com/mcp\"}}}\n")
	write(".agents/plugins/team/hooks.json", "{}\n")
	write(".agents/plugins/team/skills/plugin-only/SKILL.md", "---\nname: plugin-only\ndescription: Plugin skill\n---\nPlugin-only instructions.\n")

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	if out := run("import", "antigravity"); strings.Contains(out, "goose") {
		t.Errorf("Antigravity plugin import suggested Goose:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "mcps", "docs.yaml")); err != nil {
		t.Fatalf("Antigravity plugin MCP was not imported: %v", err)
	}

	write(".agents/rules/main.md", "---\ntrigger: always_on\n---\nProject instructions.\n")
	if out := run("import", "all"); strings.Contains(out, "goose") {
		t.Errorf("import all detected Goose from Antigravity files:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "skills", "plugin-only", "SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("import all imported Antigravity plugin skills through Goose: %v", err)
	}
}
