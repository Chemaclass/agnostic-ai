package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestImportFromAmp_NoSources(t *testing.T) {
	dir := t.TempDir()
	if err := importFromAmp(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, agnosticMainFile)); !os.IsNotExist(err) {
		t.Errorf("expected no AGNOSTIC_AI.md when AGENTS.md absent: %v", err)
	}
}

func TestImportFromAmp_MirrorsAgentsMd(t *testing.T) {
	dir := t.TempDir()
	body := "# AGENTS.md\n\n## rule-a\n\nbody.\n"
	writeFile(t, filepath.Join(dir, ampMainFile), body)
	if err := importFromAmp(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, agnosticMainFile))
	if err != nil {
		t.Fatalf("missing %s: %v", agnosticMainFile, err)
	}
	if string(got) != body {
		t.Errorf("AGNOSTIC_AI.md not byte-identical to AGENTS.md. got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "rules", "rule-a.md")); err != nil {
		t.Errorf("missing sliced rule rule-a.md: %v", err)
	}
}

func TestImportFromAmp_PrefersCommandsDir(t *testing.T) {
	dir := t.TempDir()
	body := "---\nname: reviewer\n---\n\nReview diffs.\n"
	writeFile(t, filepath.Join(dir, ampCommandsDir, "reviewer.md"), body)
	if err := importFromAmp(dir, rootSources()); err != nil {
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

func TestImportFromAmp_ImportsMCPServers(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "amp.mcpServers": {
    "fs": {"command": "fs-server", "args": ["--root", "."]}
  }
}`
	writeFile(t, filepath.Join(dir, ampSettingsFile), settings)
	if err := importFromAmp(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "mcps", "fs.yaml"))
	if err != nil {
		t.Fatalf("missing mcps/fs.yaml: %v", err)
	}
	for _, want := range []string{"name: fs", "command: fs-server"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("expected %q in %s", want, data)
		}
	}
}
