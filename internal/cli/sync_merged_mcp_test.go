package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// mergedMCPMaps lists, per target, the shared settings file and the key
// of the MCP server map sync merges into it.
var mergedMCPMaps = []struct {
	target, file, key string
}{
	{"amp", ".amp/settings.json", "amp.mcpServers"},
	{"augment", ".augment/settings.json", "mcpServers"},
	{"copilot", ".vscode/mcp.json", "servers"},
	{"crush", "crush.json", "mcp"},
	{"gemini", ".gemini/settings.json", "mcpServers"},
	{"kilo", "kilo.jsonc", "mcp"},
	{"opencode", "opencode.json", "mcp"},
	{"qoder", ".qoder/settings.json", "mcpServers"},
	{"zed", ".zed/settings.json", "context_servers"},
}

func mcpServerNames(t *testing.T, file, key string) []string {
	t.Helper()
	servers, _ := readJSONMap(t, file)[key].(map[string]any)
	var names []string
	for name := range servers {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

// Sync merges MCP servers by name: the user's own servers survive every
// sync, a server whose spec goes leaves, and releasing the file takes
// out only sync's servers (#1552).
func TestSync_MergesMCPServersByName(t *testing.T) {
	for _, tc := range mergedMCPMaps {
		t.Run(tc.target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\n")
			mine := map[string]any{"command": "my-server", "args": []any{"--mine"}}
			seed, _ := json.Marshal(map[string]any{"userKey": "kept", tc.key: map[string]any{"mine": mine}})
			mustWriteFile(t, tc.file, string(seed)+"\n")
			mustWriteFile(t, ".agnostic-ai/mcps/gh.yaml", "name: gh\ncommand: npx\nargs: [gh-mcp]\n")
			mustWriteFile(t, ".agnostic-ai/mcps/gl.yaml", "name: gl\ncommand: npx\nargs: [gl-mcp]\n")
			runSyncOK(t)
			if got := mcpServerNames(t, tc.file, tc.key); !reflect.DeepEqual(got, []string{"gh", "gl", "mine"}) {
				t.Fatalf("servers after first sync = %v", got)
			}
			runSyncOK(t, "--check")

			removeSpecs(t, ".agnostic-ai/mcps/gl.yaml")
			runSyncOK(t)
			if got := mcpServerNames(t, tc.file, tc.key); !reflect.DeepEqual(got, []string{"gh", "mine"}) {
				t.Errorf("servers after removing gl = %v", got)
			}
			runSyncOK(t, "--check")

			removeSpecs(t, ".agnostic-ai/mcps/gh.yaml")
			runSyncOK(t)
			got := readJSONMap(t, tc.file)
			want := map[string]any{"userKey": "kept", tc.key: map[string]any{"mine": mine}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("%s after removing every MCP spec = %#v", tc.file, got)
			}
		})
	}
}

// A spec server that takes a user server's name replaces it, and sync
// says so.
func TestSync_SpecServerReplacingUserServerIsNoted(t *testing.T) {
	const settings = ".gemini/settings.json"
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [gemini]\n")
	mustWriteFile(t, settings, "{\"mcpServers\": {\"gh\": {\"command\": \"mine\"}}}\n")
	mustWriteFile(t, ".agnostic-ai/mcps/gh.yaml", "name: gh\ncommand: npx\n")
	var warned bytes.Buffer
	adapters.SetWarner(&warned)
	defer adapters.SetWarner(os.Stderr)
	runSyncOK(t)
	if got := readJSONMap(t, settings)["mcpServers"].(map[string]any)["gh"]; !reflect.DeepEqual(got, map[string]any{"command": "npx"}) {
		t.Errorf("gh = %#v, want the spec's", got)
	}
	if !strings.Contains(warned.String(), `"gh"`) || !strings.Contains(warned.String(), settings) {
		t.Errorf("no note about the replaced server:\n%s", warned.String())
	}
}
