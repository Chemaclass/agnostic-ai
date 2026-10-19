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

func TestSyncGlobal_RefusesANonBooleanAgentOnly(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "github.yaml"), "name: github\nagent-only: \"yes\"\ncommand: github-mcp\n")
	_, _, err := runGlobalAgentTest("--only", "claude")
	if err == nil || !strings.Contains(err.Error(), "agent-only must be true or false") {
		t.Fatalf("sync --global must refuse it: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(home, ".claude.json")); err == nil && strings.Contains(string(data), "github") {
		t.Errorf("~/.claude.json lists the server:\n%s", data)
	}
}

// An agent file is committed with every copy of the agent, so a server
// sync writes into one gets the stricter check: credentials in url,
// args, x-<target> blocks, and reference defaults are refused too.
func TestStopOnSpecTypos_InlinedServerRefusesCredentialsOutsideEnvAndHeaders(t *testing.T) {
	cases := map[string]string{
		"url userinfo":      "type: http\nurl: https://user:hunter2secretpw@mcp.example.com/mcp\n",
		"url query":         "type: http\nurl: https://mcp.example.com/mcp?api_key=sk-live-abcdef123456\n",
		"args":              "command: srv\nargs: [--token, ghp_abcdefghijklmnopqrstuvwxyz0123456789]\n",
		"x-target env":      "command: srv\nx-kiro:\n  env:\n    GITHUB_TOKEN: ghp_abcdefghijklmnopqrstuvwxyz0123456789\n",
		"reference default": "command: srv\nenv:\n  GITHUB_TOKEN: ${GITHUB_TOKEN:-ghp_abcdefghijklmnopqrstuvwxyz0123456789}\n",
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			e := mcpEntry(t, "name: github\nagent-only: true\n"+body)
			err := stopOnSpecTypos(spec.Bundle{MCPs: []spec.Entry{e}}, []string{"claude"})
			if err == nil || !strings.Contains(err.Error(), `MCP server "github"`) {
				t.Fatalf("an agent file would carry the credential: %v", err)
			}
			for _, secret := range []string{"hunter2secretpw", "sk-live-abcdef123456", "ghp_abcdefghijklmnopqrstuvwxyz0123456789"} {
				if strings.Contains(err.Error(), secret) {
					t.Errorf("the error quotes the value: %v", err)
				}
			}
		})
	}
}

// The stricter check covers only servers an agent file carries. A shared
// server Claude keeps as a name stays under the project-level check.
func TestStopOnSpecTypos_SharedServerKeepsTheProjectCheck(t *testing.T) {
	e := mcpEntry(t, "name: docs\ncommand: srv\nargs: [--token, ghp_abcdefghijklmnopqrstuvwxyz0123456789]\n")
	agent := spec.Entry{Kind: spec.KindAgent, Name: "triager", Path: "agents/triager.md", Meta: map[string]any{"mcpServers": []any{"docs"}}}
	if err := stopOnSpecTypos(spec.Bundle{MCPs: []spec.Entry{e}, Agents: []spec.Entry{agent}}, []string{"claude"}); err != nil {
		t.Errorf("claude writes no shared server into an agent file: %v", err)
	}
	if err := stopOnSpecTypos(spec.Bundle{MCPs: []spec.Entry{e}, Agents: []spec.Entry{agent}}, []string{"kiro"}); err == nil {
		t.Errorf("kiro writes the named server into the agent file, so the credential must be refused")
	}
}

func TestValidateAndLint_ReportAnAgentFileCredential(t *testing.T) {
	dir := newProject(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "mcps", "github.yaml"),
		"name: github\nagent-only: true\ntype: http\nurl: https://mcp.example.com/mcp?api_key=sk-live-abcdef1234567890\n")
	out, err := runCLI(t, "validate")
	if err == nil || !strings.Contains(out+err.Error(), `MCP server "github": url holds a credential`) {
		t.Errorf("validate must report it: %v\n%s", err, out)
	}
	out, err = runCLI(t, "lint")
	if err == nil || !strings.Contains(out, `LINT035 [error]`) || !strings.Contains(out, `"github": url holds a credential`) {
		t.Errorf("lint must report it under LINT035: %v\n%s", err, out)
	}
	if strings.Contains(out, "sk-live-abcdef1234567890") {
		t.Errorf("lint quotes the value:\n%s", out)
	}
}

// A placeholder default is no credential, the same as in an x-<target>
// block.
func TestStopOnSpecTypos_PlaceholderDefaultPasses(t *testing.T) {
	e := mcpEntry(t, "name: github\nagent-only: true\ncommand: srv\nenv:\n  API_KEY: ${API_KEY:-none}\nx-claude:\n  env:\n    API_KEY: none\n")
	if err := stopOnSpecTypos(spec.Bundle{MCPs: []spec.Entry{e}}, []string{"claude"}); err != nil {
		t.Errorf("a placeholder default must pass: %v", err)
	}
}

func TestStopOnSpecTypos_PlaceholderUnderATokenNameIsRefused(t *testing.T) {
	e := mcpEntry(t, "name: github\nagent-only: true\ncommand: srv\nenv:\n  GITHUB_TOKEN: ${GITHUB_TOKEN:-abc}\n")
	if err := stopOnSpecTypos(spec.Bundle{MCPs: []spec.Entry{e}}, []string{"claude"}); err == nil {
		t.Errorf("a default under a token name is a credential")
	}
}
