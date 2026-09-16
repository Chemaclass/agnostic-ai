package cli

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestImportFromAmp_ImportsProjectSkillsWithAssets(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, ".agents", "skills", "review")
	writeFile(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: review\n---\n\nReview the diff.\n")
	script := filepath.Join(skillDir, "scripts", "check.sh")
	writeFile(t, script, "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := importFromAmp(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, "skills", "review", "SKILL.md"))
	if !strings.Contains(got, "Review the diff.") {
		t.Errorf("skill body not imported:\n%s", got)
	}
	info, err := os.Stat(filepath.Join(dir, "skills", "review", "scripts", "check.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("script lost executable mode: %o", info.Mode().Perm())
	}
}
