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

// Round-trip for #538: workflow fields beyond name/command/description/
// tags must survive an import under x-warp so a sync -> import -> sync
// cycle does not silently drop them.
func TestImportFromWarp_WorkflowExtraFieldsRoundTripUnderXWarp(t *testing.T) {
	dir := t.TempDir()
	wf := "name: deploy\ndescription: ship\ncommand: ./deploy.sh\nshells:\n  - zsh\nauthor: release-eng\n"
	writeFile(t, filepath.Join(dir, warpWorkflowsDir, "deploy.yaml"), wf)
	if err := importFromWarp(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "agents", "deploy.md"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(data)
	for _, want := range []string{"x-warp:", "shells:", "- zsh", "author: release-eng"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in agent file:\n%s", want, out)
		}
	}
}

// A workflow with only the known fields must not gain an empty x-warp
// block.
func TestImportFromWarp_NoXWarpBlockForPlainWorkflow(t *testing.T) {
	dir := t.TempDir()
	wf := "name: deploy\ncommand: ./deploy.sh\n"
	writeFile(t, filepath.Join(dir, warpWorkflowsDir, "deploy.yaml"), wf)
	if err := importFromWarp(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "agents", "deploy.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "x-warp") {
		t.Errorf("unexpected x-warp block for a plain workflow:\n%s", data)
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

// Warp's own field name for a stdio server's working directory is
// `working_directory`, not the spec's cross-tool `cwd`
// (docs.warp.dev/agents/capabilities/mcp). Import must rename it back to
// `cwd` so a re-emit through buildMCPServer (which reads e.Meta["cwd"])
// reaches the same JSON again instead of producing a spec with a
// `working_directory` key no other target's adapter reads. See #606.
func TestImportFromWarp_MCPStdioRenamesWorkingDirectoryToCwd(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "mcpServers": {
    "fs": {"command": "fs-server", "args": ["--root", "."], "working_directory": "./backend"}
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
	got := string(data)
	if !strings.Contains(got, "cwd: ./backend") {
		t.Errorf("expected working_directory renamed to cwd, got: %s", got)
	}
	if strings.Contains(got, "working_directory") {
		t.Errorf("working_directory must not survive the import, got: %s", got)
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
