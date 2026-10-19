package adapters

import (
	"bytes"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func agentMCPBundle(agentMeta map[string]any) spec.Bundle {
	meta := map[string]any{"description": "Reviews pull requests.", "mcpServers": []any{"github", "docs"}}
	for k, v := range agentMeta {
		meta[k] = v
	}
	return spec.NewBundle([]spec.Entry{
		{Kind: spec.KindMCP, Name: "github", Path: "mcps/github.yaml", Meta: map[string]any{
			"command":    "github-mcp",
			"env":        map[string]any{"GITHUB_TOKEN": "${GITHUB_TOKEN}"},
			"agent-only": true,
		}},
		{Kind: spec.KindMCP, Name: "docs", Path: "mcps/docs.yaml", Meta: map[string]any{
			"type": "http",
			"url":  "https://docs.example.com/mcp",
		}},
		{Kind: spec.KindAgent, Name: "reviewer", Path: "agents/reviewer.md", Meta: meta, Body: "Review the diff."},
	})
}

// emitAgentMCP emits b to target and returns each written file by path,
// plus the coverage notes.
func emitAgentMCP(t *testing.T, target string, b spec.Bundle, targets ...string) (map[string]string, string) {
	t.Helper()
	testutil.TempCwd(t)
	var notes bytes.Buffer
	old := emit.Warner
	emit.Warner = &notes
	ResetCoverageNotes()
	t.Cleanup(func() { emit.Warner = old; ResetCoverageNotes() })
	a, err := Resolve(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) == 0 {
		targets = []string{target}
	}
	sess := NewSession()
	sess.StartCapture()
	if err := EmitWithProvenance(sess, a, b, &config.Config{Targets: targets}, false); err != nil {
		sess.StopCapture()
		t.Fatalf("%s: %v", target, err)
	}
	files := map[string]string{}
	for _, f := range sess.StopCapture() {
		files[f.Path] = f.Content
	}
	FlushCoverageNotes()
	return files, notes.String()
}

func fileContaining(files map[string]string, suffix string) string {
	for path, content := range files {
		if strings.HasSuffix(path, suffix) {
			return content
		}
	}
	return ""
}

func TestAgentMCP_ClaudeInlinesAgentOnlyServerAndKeepsItOutOfMCPJSON(t *testing.T) {
	files, _ := emitAgentMCP(t, "claude", agentMCPBundle(nil))
	agent := fileContaining(files, ".claude/agents/reviewer.md")
	for _, want := range []string{"mcpServers:", "- github:", "command: github-mcp", "GITHUB_TOKEN: ${GITHUB_TOKEN}", "- docs"} {
		if !strings.Contains(agent, want) {
			t.Errorf("agent file misses %q:\n%s", want, agent)
		}
	}
	if strings.Contains(agent, "agent-only") {
		t.Errorf("agent file leaks the agent-only flag:\n%s", agent)
	}
	mcp := fileContaining(files, ".mcp.json")
	if strings.Contains(mcp, "github") {
		t.Errorf(".mcp.json lists the agent-only server:\n%s", mcp)
	}
	if !strings.Contains(mcp, `"docs"`) {
		t.Errorf(".mcp.json lost the shared server:\n%s", mcp)
	}
}

func TestAgentMCP_QoderInlinesAgentOnlyServer(t *testing.T) {
	files, _ := emitAgentMCP(t, "qoder", agentMCPBundle(nil))
	agent := fileContaining(files, ".qoder/agents/reviewer.md")
	for _, want := range []string{"- github:", "command: github-mcp", "- docs"} {
		if !strings.Contains(agent, want) {
			t.Errorf("agent file misses %q:\n%s", want, agent)
		}
	}
	if settings := fileContaining(files, ".qoder/settings.json"); strings.Contains(settings, "github") {
		t.Errorf("settings.json lists the agent-only server:\n%s", settings)
	}
}

func TestAgentMCP_KiroInlinesEveryNamedServer(t *testing.T) {
	files, notes := emitAgentMCP(t, "kiro", agentMCPBundle(nil))
	agent := fileContaining(files, ".kiro/agents/reviewer.md")
	for _, want := range []string{"mcpServers:", "github:", "command: github-mcp", "GITHUB_TOKEN: ${GITHUB_TOKEN}", "docs:", "url: https://docs.example.com/mcp"} {
		if !strings.Contains(agent, want) {
			t.Errorf("agent file misses %q:\n%s", want, agent)
		}
	}
	if mcp := fileContaining(files, ".kiro/settings/mcp.json"); strings.Contains(mcp, "github") {
		t.Errorf("mcp.json lists the agent-only server:\n%s", mcp)
	}
	if strings.Contains(notes, "x-kiro.mcpServers") {
		t.Errorf("an inlined list still asks for x-kiro.mcpServers:\n%s", notes)
	}
}

func TestAgentMCP_KiroKeepsAnExplicitOverride(t *testing.T) {
	b := agentMCPBundle(map[string]any{"x-kiro": map[string]any{"mcpServers": map[string]any{"own": map[string]any{"command": "own-mcp"}}}})
	files, _ := emitAgentMCP(t, "kiro", b)
	agent := fileContaining(files, ".kiro/agents/reviewer.md")
	if !strings.Contains(agent, "own-mcp") || strings.Contains(agent, "github-mcp") {
		t.Errorf("x-kiro.mcpServers should win:\n%s", agent)
	}
}

func TestAgentMCP_OpenHandsInlinesEveryNamedServer(t *testing.T) {
	files, _ := emitAgentMCP(t, "openhands", agentMCPBundle(nil))
	agent := fileContaining(files, ".agents/agents/reviewer.md")
	for _, want := range []string{"mcp_servers:", "github:", "command: github-mcp", "docs:"} {
		if !strings.Contains(agent, want) {
			t.Errorf("agent file misses %q:\n%s", want, agent)
		}
	}
}

func TestAgentMCP_OpenHandsSharingTheFolderWithGooseDoesNotInline(t *testing.T) {
	files, notes := emitAgentMCP(t, "openhands", agentMCPBundle(nil), "openhands", "goose")
	if agent := fileContaining(files, ".agents/agents/reviewer.md"); strings.Contains(agent, "mcp_servers") {
		t.Errorf("a file shared with Goose must stay byte-identical:\n%s", agent)
	}
	if !strings.Contains(notes, "Goose") {
		t.Errorf("expected a note naming Goose, got:\n%s", notes)
	}
}

func TestAgentMCP_CursorNotesTheAgentOnlyServer(t *testing.T) {
	files, notes := emitAgentMCP(t, "cursor", agentMCPBundle(nil))
	if mcp := fileContaining(files, ".cursor/mcp.json"); strings.Contains(mcp, "github") {
		t.Errorf("cursor mcp.json lists the agent-only server:\n%s", mcp)
	}
	if !strings.Contains(notes, "agent-only") {
		t.Errorf("expected an agent-only coverage note, got:\n%s", notes)
	}
}

func TestAgentMCP_UnnamedAgentOnlyServerIsNoted(t *testing.T) {
	files, notes := emitAgentMCP(t, "claude", agentMCPBundle(map[string]any{"mcpServers": []any{"docs"}}))
	if agent := fileContaining(files, ".claude/agents/reviewer.md"); strings.Contains(agent, "github") {
		t.Errorf("agent file has a server it does not name:\n%s", agent)
	}
	if !strings.Contains(notes, "agent-only") {
		t.Errorf("expected a note for the agent-only server no agent names, got:\n%s", notes)
	}
}

func TestAgentMCP_UserMCPServersLeaveAgentOnlyOut(t *testing.T) {
	servers, ok := UserMCPServers("claude", agentMCPBundle(nil).MCPs)
	if !ok {
		t.Fatal("claude has a user MCP renderer")
	}
	if _, leaked := servers["github"]; leaked {
		t.Errorf("~/.claude.json would list the agent-only server: %v", servers)
	}
	if _, kept := servers["docs"]; !kept {
		t.Errorf("shared server missing: %v", servers)
	}
}

func TestAgentMCP_DisabledAgentOnlyServerLeavesNoDanglingName(t *testing.T) {
	b := agentMCPBundle(nil)
	b.MCPs[0].Meta["disabled"] = true
	files, _ := emitAgentMCP(t, "claude", b)
	agent := fileContaining(files, ".claude/agents/reviewer.md")
	if strings.Contains(agent, "github") {
		t.Errorf("a disabled agent-only server must leave no name behind:\n%s", agent)
	}
	if !strings.Contains(agent, "- docs") {
		t.Errorf("the shared name was lost:\n%s", agent)
	}
}

func TestAgentMCP_OverrideStillCountsAsNamingTheServer(t *testing.T) {
	b := agentMCPBundle(map[string]any{"x-claude": map[string]any{"mcpServers": []any{"docs"}}})
	_, notes := emitAgentMCP(t, "claude", b)
	if strings.Contains(notes, "no agent names it") {
		t.Errorf("the agent names the server; only its override replaces the list:\n%s", notes)
	}
}

type pluginStub struct{ got spec.Bundle }

func (p *pluginStub) Name() string              { return "myplugin" }
func (p *pluginStub) Capabilities() []spec.Kind { return nil }
func (p *pluginStub) Emit(_ *emit.Session, b spec.Bundle, _ *config.Config, _ bool) error {
	p.got = b
	return nil
}

func TestAgentMCP_PluginTargetGetsNoAgentOnlyServer(t *testing.T) {
	testutil.TempCwd(t)
	plugin := &pluginStub{}
	sess := NewSession()
	sess.StartCapture()
	defer sess.StopCapture()
	if err := EmitWithProvenance(sess, plugin, agentMCPBundle(nil), &config.Config{Targets: []string{"myplugin"}}, false); err != nil {
		t.Fatal(err)
	}
	for _, e := range plugin.got.MCPs {
		if e.Name == "github" {
			t.Errorf("a plugin target must not receive the agent-only server: %v", e.Meta)
		}
	}
	if len(plugin.got.MCPs) != 1 {
		t.Errorf("the shared server is missing: %v", plugin.got.MCPs)
	}
}

func TestAgentMCP_NamesOnlyTargetsDropTheAgentOnlyName(t *testing.T) {
	for target, path := range map[string]string{"factory": ".factory/droids/reviewer.md", "junie": ".junie/agents/reviewer.md"} {
		t.Run(target, func(t *testing.T) {
			files, _ := emitAgentMCP(t, target, agentMCPBundle(nil))
			agent := fileContaining(files, path)
			if strings.Contains(agent, "github") || !strings.Contains(agent, "docs") {
				t.Errorf("want only the shared name left:\n%s", agent)
			}
		})
	}
}

// On Junie an empty list keeps every server, so a list that names only
// agent-only servers stays as written: its names match no server.
func TestAgentMCP_JunieNeverEmptiesTheList(t *testing.T) {
	files, _ := emitAgentMCP(t, "junie", agentMCPBundle(map[string]any{"mcpServers": []any{"github"}}))
	if agent := fileContaining(files, ".junie/agents/reviewer.md"); !strings.Contains(agent, "github") {
		t.Errorf("an emptied Junie list would grant every server:\n%s", agent)
	}
}

func TestAgentMCP_KiroAndQoderLeaveADisabledServerOut(t *testing.T) {
	for target, path := range map[string]string{"kiro": ".kiro/agents/reviewer.md", "qoder": ".qoder/agents/reviewer.md"} {
		t.Run(target, func(t *testing.T) {
			b := agentMCPBundle(nil)
			b.MCPs[0].Meta["disabled"] = true
			files, _ := emitAgentMCP(t, target, b)
			if agent := fileContaining(files, path); strings.Contains(agent, "github") {
				t.Errorf("a disabled server reached the agent file:\n%s", agent)
			}
		})
	}
}

func TestAgentMCP_GeminiAndCopilotNotesNameTheAgentKey(t *testing.T) {
	for target, key := range map[string]string{"gemini": "x-gemini.mcp_servers", "copilot": "x-copilot.mcp-servers"} {
		t.Run(target, func(t *testing.T) {
			_, notes := emitAgentMCP(t, target, agentMCPBundle(nil))
			if !strings.Contains(notes, key) {
				t.Errorf("note should name %s:\n%s", key, notes)
			}
		})
	}
}

func TestAgentMCP_UnknownNamesCountEachAgentOnce(t *testing.T) {
	_, notes := emitAgentMCP(t, "kiro", agentMCPBundle(map[string]any{"mcpServers": []any{"nope", "nada"}}))
	if !strings.Contains(notes, "`mcpServers` on 1 agent") {
		t.Errorf("one agent with two unknown names is one agent:\n%s", notes)
	}
}

func TestAgentMCP_AntigravityGivesOneReason(t *testing.T) {
	_, notes := emitAgentMCP(t, "antigravity", agentMCPBundle(nil))
	if strings.Contains(notes, "inline server objects only") {
		t.Errorf("the older reason contradicts the agent-only note:\n%s", notes)
	}
}
