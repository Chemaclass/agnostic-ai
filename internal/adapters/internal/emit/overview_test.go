package emit

import (
	"strings"
	"testing"
)

func TestRenderTargetOverview_SingleTargetFlatBullets(t *testing.T) {
	out := RenderTargetOverview([]TargetArtifacts{{
		Target: "claude",
		Artifacts: []NativeArtifact{
			{Label: "Rules", Location: ".claude/rules/", Note: "one file per rule"},
			{Label: "MCP servers", Location: ".mcp.json"},
		},
	}})
	if !strings.Contains(out, OverviewStartMarker) || !strings.Contains(out, OverviewEndMarker) {
		t.Fatalf("missing sentinel markers:\n%s", out)
	}
	if strings.Contains(out, "### claude") {
		t.Errorf("single target must not render a sub-heading:\n%s", out)
	}
	if !strings.Contains(out, "- **Rules**: `.claude/rules/` (one file per rule)") {
		t.Errorf("missing rules bullet:\n%s", out)
	}
	if !strings.Contains(out, "- **MCP servers**: `.mcp.json`\n") {
		t.Errorf("missing mcp bullet without note:\n%s", out)
	}
}

func TestRenderTargetOverview_SharedPathRendersPerTargetHeadings(t *testing.T) {
	out := RenderTargetOverview([]TargetArtifacts{
		{Target: "codex", Artifacts: []NativeArtifact{{Label: "Agents", Location: ".codex/agents/"}}},
		{Target: "amp", Artifacts: []NativeArtifact{{Label: "Skills", Location: ".agents/skills/"}}},
	})
	if !strings.Contains(out, "### codex") || !strings.Contains(out, "### amp") {
		t.Errorf("expected per-target headings on shared path:\n%s", out)
	}
}

func TestRenderTargetOverview_EmptyWhenNoArtifacts(t *testing.T) {
	if out := RenderTargetOverview(nil); out != "" {
		t.Errorf("nil sections: got %q, want empty", out)
	}
	out := RenderTargetOverview([]TargetArtifacts{{Target: "aider"}})
	if out != "" {
		t.Errorf("artifact-less target: got %q, want empty", out)
	}
}

func TestStripTargetOverview_RoundTrip(t *testing.T) {
	body := "# Conventions\n\nEdit sources.\n"
	overview := RenderTargetOverview([]TargetArtifacts{{
		Target:    "claude",
		Artifacts: []NativeArtifact{{Label: "Rules", Location: ".claude/rules/"}},
	}})
	appended := AppendTargetOverview(body, overview)
	if got := StripTargetOverview(appended); got != body {
		t.Errorf("round-trip mismatch:\ngot  %q\nwant %q", got, body)
	}
}

func TestStripTargetOverview_NoBlockUnchanged(t *testing.T) {
	body := "# Conventions\n\nNo block here.\n"
	if got := StripTargetOverview(body); got != body {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestStripTargetOverview_TruncatedBlockHeals(t *testing.T) {
	body := "# Conventions\n\n" + OverviewStartMarker + "\ntruncated, no end marker"
	got := StripTargetOverview(body)
	if got != "# Conventions\n" {
		t.Errorf("got %q, want body without truncated block", got)
	}
}

func TestAppendTargetOverview_DoesNotStack(t *testing.T) {
	body := "# Conventions\n"
	overview := RenderTargetOverview([]TargetArtifacts{{
		Target:    "claude",
		Artifacts: []NativeArtifact{{Label: "Rules", Location: ".claude/rules/"}},
	}})
	once := AppendTargetOverview(body, overview)
	twice := AppendTargetOverview(once, overview)
	if once != twice {
		t.Errorf("appendix stacked on repeated append:\nonce  %q\ntwice %q", once, twice)
	}
	if n := strings.Count(twice, OverviewStartMarker); n != 1 {
		t.Errorf("start marker count = %d, want 1", n)
	}
}
