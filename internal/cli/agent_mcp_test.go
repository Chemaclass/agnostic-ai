package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func writeAgentOnlyProject(t *testing.T, env string) string {
	t.Helper()
	dir := newProject(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, kiro, cursor]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "github.yaml"),
		"name: github\nagent-only: true\ncommand: github-mcp\nenv:\n  GITHUB_TOKEN: "+env+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "triager.md"),
		"---\nname: triager\ndescription: Triage issues.\nmcpServers: [github]\n---\nLabel new issues.\n")
	return dir
}

func TestSync_AgentOnlyServerReachesOnlyTheAgentFiles(t *testing.T) {
	dir := writeAgentOnlyProject(t, "${GITHUB_TOKEN}")
	out, err := runCLI(t, "sync")
	if err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	for _, path := range []string{".claude/agents/triager.md", ".kiro/agents/triager.md"} {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || !strings.Contains(string(data), "command: github-mcp") {
			t.Errorf("%s misses the inline server: %v\n%s", path, err, data)
		}
	}
	for _, path := range []string{".mcp.json", ".kiro/settings/mcp.json", ".cursor/mcp.json"} {
		if data, err := os.ReadFile(filepath.Join(dir, path)); err == nil && strings.Contains(string(data), "github") {
			t.Errorf("%s lists the agent-only server:\n%s", path, data)
		}
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Errorf("sync --check after sync: %v\n%s", err, out)
	}
}

func TestSync_AgentOnlyServerWithALiteralTokenIsRefused(t *testing.T) {
	dir := writeAgentOnlyProject(t, "ghp_literalvalue123")
	out, err := runCLI(t, "sync")
	if err == nil || !strings.Contains(err.Error(), `MCP server "github": env GITHUB_TOKEN is a literal value`) {
		t.Fatalf("sync must refuse the literal: %v\n%s", err, out)
	}
	if strings.Contains(out+err.Error(), "ghp_literalvalue123") {
		t.Errorf("output quotes the value:\n%s\n%v", out, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "agents", "triager.md")); !os.IsNotExist(err) {
		t.Errorf("sync must write no agent file: %v", err)
	}
}

// A pack's literal reaches a shared MCP file today, but an agent file
// would carry it into every copy of the agent, so sync refuses it.
func TestStopOnSpecTypos_RefusesAPackLiteralWrittenIntoAnAgent(t *testing.T) {
	literal := mcpEntry(t, "name: github\nagent-only: true\ncommand: srv\nenv:\n  GITHUB_TOKEN: ghp_literalvalue123\n")
	literal.Layer = "pack:tools"
	named := mcpEntry(t, "name: docs\ncommand: srv\nenv:\n  API_KEY: sk-live-abc\n")
	named.Layer = "pack:tools"
	unnamed := mcpEntry(t, "name: other\ncommand: srv\nenv:\n  API_KEY: sk-live-abc\n")
	unnamed.Layer = "pack:tools"
	agent := spec.Entry{Kind: spec.KindAgent, Name: "triager", Path: "agents/triager.md", Meta: map[string]any{"mcpServers": []any{"docs"}}}
	b := spec.Bundle{MCPs: []spec.Entry{literal, named, unnamed}, Agents: []spec.Entry{agent}}
	err := stopOnSpecTypos(b, []string{"kiro"})
	if err == nil {
		t.Fatal("sync must refuse a pack literal that lands in an agent file")
	}
	for _, want := range []string{`"github": env GITHUB_TOKEN`, `"docs": env API_KEY`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error misses %s: %v", want, err)
		}
	}
	if strings.Contains(err.Error(), `"other"`) {
		t.Errorf("a pack server no agent writes keeps today's behavior: %v", err)
	}
}

// Claude keeps a shared server a name, so a pack literal there stays as
// before; only the agent-only server reaches the agent file.
func TestStopOnSpecTypos_PackLiteralOfANamedServerStaysAllowedWhereItStaysAName(t *testing.T) {
	named := mcpEntry(t, "name: docs\ncommand: srv\nenv:\n  API_KEY: sk-live-abc\n")
	named.Layer = "pack:tools"
	agent := spec.Entry{Kind: spec.KindAgent, Name: "triager", Path: "agents/triager.md", Meta: map[string]any{"mcpServers": []any{"docs"}}}
	if err := stopOnSpecTypos(spec.Bundle{MCPs: []spec.Entry{named}, Agents: []spec.Entry{agent}}, []string{"claude", "cursor"}); err != nil {
		t.Errorf("no target writes the server into an agent file: %v", err)
	}
}

func TestStopOnSpecTypos_RefusesANonBooleanAgentOnly(t *testing.T) {
	e := mcpEntry(t, "name: github\nagent-only: yes\ncommand: srv\n")
	err := stopOnSpecTypos(spec.Bundle{MCPs: []spec.Entry{e}}, []string{"claude"})
	if err == nil || !strings.Contains(err.Error(), "agent-only must be true or false") {
		t.Errorf("a string agent-only would write the server to every MCP file: %v", err)
	}
}
