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

// Commands land in the commands source; native agents and skill folders
// land in theirs. Pre-native projects that only had agents-as-commands
// import those files as command specs, matching what OpenCode runs today.
func TestImportFromOpencode_AgentsSkillsAndCommands(t *testing.T) {
	dir := t.TempDir()
	cmdBody := "---\ndescription: Ship it\n---\n\nDeploy steps.\n"
	writeFile(t, filepath.Join(dir, opencodeCommandsDir, "deploy.md"), cmdBody)
	agentBody := "---\ndescription: Review diffs.\nmode: subagent\n---\n\nReview diffs.\n"
	writeFile(t, filepath.Join(dir, opencodeAgentsDir, "reviewer.md"), agentBody)
	writeFile(t, filepath.Join(dir, opencodeSkillsDir, "greet", "SKILL.md"), "---\nname: greet\n---\nhi\n")
	writeFile(t, filepath.Join(dir, opencodeSkillsDir, "greet", "helper.sh"), "echo hi\n")

	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "commands", "deploy.md"))
	if err != nil {
		t.Fatalf("missing commands/deploy.md: %v", err)
	}
	if string(got) != cmdBody {
		t.Errorf("command not byte-identical. got %q", got)
	}
	got, err = os.ReadFile(filepath.Join(dir, "agents", "reviewer.md"))
	if err != nil {
		t.Fatalf("missing agents/reviewer.md: %v", err)
	}
	if string(got) != agentBody {
		t.Errorf("agent not byte-identical. got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "skills", "greet", "helper.sh")); err != nil {
		t.Errorf("skill folder should import with assets: %v", err)
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
	// OpenCode's `type: remote` is dropped during normalization, then
	// the shared writer synthesizes `type: http` from the url-bearing
	// entry shape so a subsequent emit knows which transport branch
	// to take. The opencode adapter still re-translates http→remote
	// on emit.
	if !strings.Contains(out, "type: http") {
		t.Errorf("expected synthesized type: http for url-bearing mcp:\n%s", out)
	}
	if strings.Contains(out, "type: remote") {
		t.Errorf("opencode-specific type: remote should be dropped:\n%s", out)
	}
}

// OpenCode's `enabled: false` round-trips to the spec's own `disabled:
// true` field, the inverse of what the opencode adapter emits (#555).
// Without this, re-syncing an imported project would drop the disabled
// state and OpenCode would start running the server.
func TestImportFromOpencode_DisabledMCP(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "mcp": {
    "fs": {"type": "local", "command": ["fs-server"], "enabled": false}
  }
}`
	writeFile(t, filepath.Join(dir, opencodeMCPFile), settings)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "mcps", "fs.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if !strings.Contains(out, "disabled: true") {
		t.Errorf("expected disabled: true in mcp file:\n%s", out)
	}
	if strings.Contains(out, "enabled") {
		t.Errorf("opencode-specific enabled key should not leak into the spec:\n%s", out)
	}
}

// An entry with no `enabled` key (the common case) gets no `disabled`
// key in the recovered spec either.
func TestImportFromOpencode_EnabledMCPHasNoDisabledKey(t *testing.T) {
	dir := t.TempDir()
	settings := `{
  "mcp": {
    "fs": {"type": "local", "command": ["fs-server"]}
  }
}`
	writeFile(t, filepath.Join(dir, opencodeMCPFile), settings)
	if err := importFromOpencode(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "mcps", "fs.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)
	if strings.Contains(out, "disabled") {
		t.Errorf("expected no disabled key for an enabled server, got:\n%s", out)
	}
}
