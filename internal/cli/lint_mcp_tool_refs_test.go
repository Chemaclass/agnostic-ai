package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestLintMCPToolRefs_FlagsToolFormsInURLAndArgs(t *testing.T) {
	mcps := []spec.Entry{
		{Kind: spec.KindMCP, Name: "remote", Path: "mcps/remote.yaml", Meta: map[string]any{"type": "http", "url": "https://mcp.example.com/mcp?key=${env:API_KEY}"}},
		{Kind: spec.KindMCP, Name: "local", Path: "mcps/local.yaml", Meta: map[string]any{"command": "npx", "args": []any{"-y", "server", "--token", "{env:GH_TOKEN}"}}},
		{Kind: spec.KindMCP, Name: "secret", Path: "mcps/secret.yaml", Meta: map[string]any{"command": "npx", "args": []any{"${{ secrets.TOKEN }}"}}},
	}
	findings := lintMCPToolRefs([]string{"claude", "cursor"}, targetsSupportingKind, mcps)
	if len(findings) != 3 {
		t.Fatalf("findings = %+v, want 3", findings)
	}
	wants := []struct{ path, token, fix string }{
		{"mcps/remote.yaml", "${env:API_KEY}", "${API_KEY}"},
		{"mcps/local.yaml", "{env:GH_TOKEN}", "${GH_TOKEN}"},
		{"mcps/secret.yaml", "${{ secrets.TOKEN }}", "${TOKEN}"},
	}
	for i, want := range wants {
		f := findings[i]
		if f.Code != "LINT028" || f.Severity != lintWarn || f.Path != want.path {
			t.Errorf("finding %d = %+v, want LINT028 warn on %s", i, f, want.path)
		}
		if !strings.Contains(f.Message, want.token) || !strings.Contains(f.Message, want.fix) {
			t.Errorf("message %q should name %s and suggest %s", f.Message, want.token, want.fix)
		}
	}
	if strings.Contains(findings[0].Message, "mcp.example.com") {
		t.Errorf("message %q should quote the reference, not the value around it, which may hold a secret", findings[0].Message)
	}
}

func TestLintMCPToolRefs_ReportsEveryArgumentInOneFinding(t *testing.T) {
	mcps := []spec.Entry{{Kind: spec.KindMCP, Name: "local", Path: "mcps/local.yaml", Meta: map[string]any{"command": "srv", "args": []any{"--a", "${env:A}", "--b", "${env:B}"}}}}
	findings := lintMCPToolRefs([]string{"claude", "cursor"}, targetsSupportingKind, mcps)
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "${env:A}, ${env:B}") || !strings.Contains(findings[0].Message, "${A}, ${B}") {
		t.Errorf("findings = %+v, want one naming both references", findings)
	}
}

func TestLintMCPToolRefs_LeavesSpecRefsOverridesAndToolVariablesAlone(t *testing.T) {
	mcps := []spec.Entry{
		{Kind: spec.KindMCP, Name: "ok", Path: "mcps/ok.yaml", Meta: map[string]any{
			"type": "http", "url": "https://${API_HOST}/mcp",
			"x-cursor": map[string]any{"url": "https://${env:API_HOST}/mcp"},
		}},
		{Kind: spec.KindMCP, Name: "editor", Path: "mcps/editor.yaml", Meta: map[string]any{"command": "srv", "args": []any{"${workspaceFolder}", "${env:workspaceFolder}", "${input:token}"}}},
		{Kind: spec.KindMCP, Name: "env", Path: "mcps/env.yaml", Meta: map[string]any{"command": "srv", "env": map[string]any{"TOKEN": "${env:TOKEN}"}}},
	}
	if findings := lintMCPToolRefs([]string{"claude", "cursor"}, targetsSupportingKind, mcps); len(findings) != 0 {
		t.Errorf("findings = %+v, want none", findings)
	}
}

func TestLintMCPToolRefs_SkipsFormEveryEnabledTargetReadsAndFieldSyncDoesNotWrite(t *testing.T) {
	mcps := []spec.Entry{
		{Kind: spec.KindMCP, Name: "remote", Path: "mcps/remote.yaml", Meta: map[string]any{"type": "http", "url": "https://x/${env:API_KEY}"}},
		{Kind: spec.KindMCP, Name: "stray", Path: "mcps/stray.yaml", Meta: map[string]any{"command": "srv", "url": "https://x/${env:API_KEY}"}},
	}
	if findings := lintMCPToolRefs([]string{"cursor", "windsurf"}, targetsSupportingKind, mcps); len(findings) != 0 {
		t.Errorf("every enabled target reads ${env:NAME}, and a stdio server's url is never written: %+v", findings)
	}
	findings := lintMCPToolRefs([]string{"cursor", "claude", "aider"}, targetsSupportingKind, mcps[:1])
	if len(findings) != 1 || !strings.Contains(findings[0].Message, "as text to claude.") || strings.Contains(findings[0].Message, "cursor") {
		t.Errorf("findings = %+v, want one naming claude only", findings)
	}
}

func TestCollectLintFindings_IncludesMCPToolRefs(t *testing.T) {
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindMCP, Name: "remote", Path: "mcps/remote.yaml", Meta: map[string]any{"type": "http", "url": "https://x/${env:API_KEY}"}}})
	for _, f := range collectLintFindings([]string{"claude"}, targetsSupportingKind, b) {
		if f.Code == "LINT028" {
			return
		}
	}
	t.Error("collectLintFindings must run the MCP tool-reference check")
}
