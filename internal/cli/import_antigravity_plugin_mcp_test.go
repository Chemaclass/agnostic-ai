package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportAntigravity_PluginMCPFileRoundTrips(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: .agents/plugins/team/mcp.json\n")
	mcpPath := filepath.FromSlash(".agents/plugins/team/mcp.json")
	manifestPath := filepath.FromSlash(".agents/plugins/team/plugin.json")
	mcp := "{\n  \"mcpServers\": {\n    \"docs\": {\n      \"serverUrl\": \"https://example.com/mcp\"\n    },\n    \"fs\": {\n      \"args\": [\n        \"--root\",\n        \".\"\n      ],\n      \"command\": \"fs-server\"\n    }\n  }\n}\n"
	manifest := "{\n  \"name\": \"team\",\n  \"version\": \"2.0.0\"\n}\n"
	mustWriteFile(t, mcpPath, mcp)
	mustWriteFile(t, manifestPath, manifest)

	if out, err := runRoot(t, "import", "antigravity"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	for _, name := range []string{"docs", "fs"} {
		if !exists(filepath.Join(".agnostic-ai", "mcps", name+".yaml")) {
			t.Fatalf("import skipped plugin MCP server %q", name)
		}
	}
	if exists(filepath.FromSlash(".agents/mcp_config.json")) {
		t.Fatal("import created the standalone MCP file")
	}

	runSyncOK(t)
	if got := readFile(t, mcpPath); got != mcp {
		t.Errorf("sync changed plugin mcp.json:\n%s", got)
	}
	if got := readFile(t, manifestPath); got != manifest {
		t.Errorf("sync changed plugin.json:\n%s", got)
	}
	if _, err := os.Stat(mcpPath + ".bak"); !os.IsNotExist(err) {
		t.Errorf("unexpected mcp.json backup: %v", err)
	}
	runSyncOK(t, "--check")
}
