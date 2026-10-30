package antigravity

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Plugins bundle MCP servers in `<plugin-root>/mcp.json`, while
// standalone configs keep `mcp_config.json`
// (antigravity.google/docs/mcp?tab=ide, "Standalone configs vs. plugin
// bundles").
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/antigravity/ -run 'MCP_GoldenSnapshot'
func TestPluginMCP_GoldenSnapshot(t *testing.T) {
	assertMCPGolden(t, "plugin-mcp", &config.Config{Outputs: map[string]config.Output{target: {
		MCPFile: ".agents/plugins/team/mcp.json",
	}}})
}

func TestStandaloneMCP_GoldenSnapshot(t *testing.T) {
	assertMCPGolden(t, "standalone-mcp", &config.Config{})
}

func assertMCPGolden(t *testing.T, name string, cfg *config.Config) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(mcpGoldenEntries()), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}
	testutil.AssertGoldenTree(t, dir, filepath.Join(cwd, "testdata", name))
}

func mcpGoldenEntries() []spec.Entry {
	return []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Path: "mcps/fs.yaml", Meta: map[string]any{
			"command": "npx",
			"args":    []any{"-y", "@modelcontextprotocol/server-filesystem", "."},
		}},
		{Kind: spec.KindMCP, Name: "docs", Path: "mcps/docs.yaml", Meta: map[string]any{
			"type":    "http",
			"url":     "https://example.com/mcp",
			"headers": map[string]any{"Authorization": "Bearer ${DOCS_TOKEN}"},
		}},
	}
}
