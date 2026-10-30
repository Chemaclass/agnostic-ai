package cli

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestServeEditMCP_PreviewOnlyNamesToolsThatWriteTheServer(t *testing.T) {
	source, _ := mcpEditGlobal(t, `{"mcpServers":{"github":{"command":"github-mcp","env":{"GITHUB_TOKEN":"ghp_synthetic123"}}}}`)
	mustWrite(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude, kimi]\n")
	preview := mcpEditPreview(t, newEditHandler(source, true), mcpEditIdentity("github", "shared", "user", "~/.claude.json"))
	if !slices.Equal(preview.Targets, []string{"claude"}) {
		t.Errorf("preview targets = %v; Kimi cannot interpolate the env reference", preview.Targets)
	}
}

func TestServeEditMCP_ProjectPreviewOnlyNamesToolsThatWriteTheServer(t *testing.T) {
	nativeMCPHome(t)
	project := scopeProject(t)
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, kimi]\n")
	mustWrite(t, ".mcp.json", `{"mcpServers":{"github":{"command":"github-mcp","env":{"GITHUB_TOKEN":"ghp_synthetic123"}}}}`)
	preview := mcpEditPreview(t, newEditHandler(project, false), mcpEditIdentity("github", "shared", "project", ".mcp.json"))
	if !slices.Equal(preview.Targets, []string{"claude"}) {
		t.Errorf("preview targets = %v; Kimi cannot interpolate the env reference", preview.Targets)
	}
}
