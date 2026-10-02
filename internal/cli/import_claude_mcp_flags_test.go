package cli

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportFromClaude_MCPBooleanFlagsRoundTrip(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	const native = `{
  "mcpServers": {
    "deferred": {"command": "example-server", "description": "Deferred tools.", "alwaysLoad": false, "bareElicitationCapability": true},
    "loaded": {"type": "http", "url": "https://example.invalid/mcp", "description": "Loaded tools.", "alwaysLoad": true, "bareElicitationCapability": false},
    "default": {"command": "default-server", "description": "Default loading."}
  }
}`
	mustWriteFile(t, ".mcp.json", native)
	if out, err := runCLI(t, "import", "claude"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	runSyncOK(t)
	var want map[string]any
	if err := json.Unmarshal([]byte(native), &want); err != nil {
		t.Fatal(err)
	}
	if got := readJSONMap(t, ".mcp.json"); !reflect.DeepEqual(got, want) {
		t.Errorf("MCP round trip = %#v, want %#v", got, want)
	}
	data, err := os.ReadFile(".codex/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"alwaysLoad", "bareElicitationCapability"} {
		if strings.Contains(string(data), key) {
			t.Errorf("Codex inherited Claude field %s: %s", key, data)
		}
	}
	runSyncOK(t, "--check")
	if out, err := runCLI(t, "lint", "--strict"); err != nil {
		t.Fatalf("lint: %v\n%s", err, out)
	}
}

func TestSync_ClaudeMCPBooleanFlagsUseNamespacedOverrides(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/mcps/deferred.yaml", "name: deferred\ndescription: Deferred tools.\ncommand: example-server\nalwaysLoad: true\nbareElicitationCapability: false\nx-claude:\n  alwaysLoad: false\n  bareElicitationCapability: true\n")
	mustWriteFile(t, ".agnostic-ai/mcps/loaded.yaml", "name: loaded\ndescription: Loaded tools.\ntype: http\nurl: https://example.invalid/mcp\nalwaysLoad: false\nbareElicitationCapability: true\nx-claude:\n  alwaysLoad: true\n  bareElicitationCapability: false\n")
	runSyncOK(t)
	servers, ok := readJSONMap(t, ".mcp.json")["mcpServers"].(map[string]any)
	if !ok {
		t.Fatal("missing native MCP server map")
	}
	want := map[string]map[string]bool{
		"deferred": {"alwaysLoad": false, "bareElicitationCapability": true},
		"loaded":   {"alwaysLoad": true, "bareElicitationCapability": false},
	}
	for name, fields := range want {
		server, ok := servers[name].(map[string]any)
		if !ok {
			t.Errorf("missing server %s: %#v", name, servers)
			continue
		}
		for key, value := range fields {
			got, ok := server[key].(bool)
			if !ok || got != value {
				t.Errorf("%s.%s = %#v, want %v", name, key, server[key], value)
			}
		}
	}
	runSyncOK(t, "--check")
	if out, err := runCLI(t, "lint", "--strict"); err != nil {
		t.Fatalf("lint: %v\n%s", err, out)
	}
}

func TestLintNearMissKeys_FlagsClaudeMCPCompatibilityTypo(t *testing.T) {
	entries := []spec.Entry{{
		Kind: spec.KindMCP, Name: "compat", Path: "mcps/compat.yaml",
		Meta: map[string]any{"bareElicitationCapabilty": true},
	}}
	findings := lintNearMissKeys(entries, []string{"claude"})
	if len(findings) != 1 {
		t.Fatalf("compatibility typo findings = %#v, want one", findings)
	}
	if !strings.Contains(findings[0].Message, "bareElicitationCapability") {
		t.Errorf("compatibility typo advice = %q", findings[0].Message)
	}
}
