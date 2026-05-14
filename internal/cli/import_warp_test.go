package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportFromWarp_NoSources(t *testing.T) {
	dir := t.TempDir()
	if err := importFromWarp(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, agnosticMainFile)); !os.IsNotExist(err) {
		t.Errorf("expected no AGNOSTIC_AI.md when AGENTS.md absent: %v", err)
	}
}

func TestImportFromWarp_MirrorsAgentsMd(t *testing.T) {
	dir := t.TempDir()
	body := "# AGENTS.md\n\n## rule-a\n\nbody.\n"
	writeFile(t, filepath.Join(dir, warpMainFile), body)
	if err := importFromWarp(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("missing %s: %v", agnosticMainFile, err)
	}
	if string(got) != body {
		t.Errorf("AGNOSTIC_AI.md not byte-identical to AGENTS.md. got %q", got)
	}
}

func TestImportFromWarp_ImportsWorkflowsAsAgents(t *testing.T) {
	dir := t.TempDir()
	wf := "name: deploy\ndescription: ship to prod\ncommand: |\n  ./scripts/deploy.sh\n"
	writeFile(t, filepath.Join(dir, warpWorkflowsDir, "deploy.yaml"), wf)
	if err := importFromWarp(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "agents", "deploy.md"))
	if err != nil {
		t.Fatalf("missing agents/deploy.md: %v", err)
	}
	out := string(got)
	for _, want := range []string{"name: deploy", "description: ship to prod", "./scripts/deploy.sh"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in agent file:\n%s", want, out)
		}
	}
}

func TestImportFromWarp_ImportsMCPServers(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "mcpServers": {
    "fs": {"command": "fs-server", "args": ["--root", "."]}
  }
}`
	writeFile(t, filepath.Join(dir, warpMCPFile), settings)
	if err := importFromWarp(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "mcps", "fs.yaml"))
	if err != nil {
		t.Fatalf("missing mcps/fs.yaml: %v", err)
	}
	for _, want := range []string{"name: fs", "command: fs-server"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("expected %q in mcp file: %s", want, data)
		}
	}
}

func TestImportFromWarp_WorkflowTagsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	wf := "name: deploy\ndescription: ship\ntags:\n  - release\n  - ops\ncommand: ./deploy.sh\n"
	writeFile(t, filepath.Join(dir, warpWorkflowsDir, "deploy.yaml"), wf)
	if err := importFromWarp(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "agents", "deploy.md"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	if !strings.Contains(out, "tags: [release, ops]") {
		t.Errorf("expected tags frontmatter in agent file:\n%s", out)
	}
}
