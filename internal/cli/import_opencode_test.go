package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportFromOpencode_NoSources(t *testing.T) {
	dir := t.TempDir()
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, agnosticMainFile)); !os.IsNotExist(err) {
		t.Errorf("expected no AGNOSTIC_AI.md when .opencode/AGENTS.md absent: %v", err)
	}
}

func TestImportFromOpencode_MirrorsAgentsMd(t *testing.T) {
	dir := t.TempDir()
	body := "# AGENTS.md\n\n## rule-a\n\nbody.\n"
	writeFile(t, filepath.Join(dir, opencodeMainFile), body)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("missing %s: %v", agnosticMainFile, err)
	}
	if string(got) != body {
		t.Errorf("AGNOSTIC_AI.md not byte-identical. got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "rule-a.md")); err != nil {
		t.Errorf("missing sliced rule rule-a.md: %v", err)
	}
}

func TestImportFromOpencode_CopiesCommands(t *testing.T) {
	dir := t.TempDir()
	body := "---\nname: reviewer\n---\n\nReview diffs.\n"
	writeFile(t, filepath.Join(dir, opencodeCommandsDir, "reviewer.md"), body)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "agents", "reviewer.md"))
	if err != nil {
		t.Fatalf("missing agents/reviewer.md: %v", err)
	}
	if string(got) != body {
		t.Errorf("agent not byte-identical. got %q", got)
	}
}

func TestImportFromOpencode_ImportsMCP(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "$schema": "https://opencode.ai/config.json",
  "mcp": {
    "fs": {
      "type": "local",
      "command": ["fs-server", "--root", "."],
      "environment": {"TOKEN": "abc"}
    }
  }
}`
	writeFile(t, filepath.Join(dir, opencodeMCPFile), settings)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "mcps", "fs.yaml"))
	if err != nil {
		t.Fatalf("missing mcps/fs.yaml: %v", err)
	}
	out := string(got)
	for _, want := range []string{
		"name: fs",
		"command: fs-server",
		"args:",
		"- --root",
		"env:",
		"TOKEN: abc",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in mcp file:\n%s", want, out)
		}
	}
	for _, unwanted := range []string{"type: local", "environment:", "command:\n  - fs-server"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("unexpected %q in mcp file:\n%s", unwanted, out)
		}
	}
}

func TestImportFromOpencode_RemoteMCP(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "mcp": {
    "remote-fs": {"type": "remote", "url": "https://example.test/mcp"}
  }
}`
	writeFile(t, filepath.Join(dir, opencodeMCPFile), settings)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "mcps", "remote-fs.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if !strings.Contains(out, "url: https://example.test/mcp") {
		t.Errorf("expected url in mcp file:\n%s", out)
	}
	if strings.Contains(out, "type:") {
		t.Errorf("type should be dropped:\n%s", out)
	}
}
