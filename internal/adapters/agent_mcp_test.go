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
