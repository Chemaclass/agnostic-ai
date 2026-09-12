package copilot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_VSCodeMCPPreservesSiblingSettings(t *testing.T) {
	testutil.TempCwd(t)
	path := "custom/mcp.json"
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const existing = `{
  // VS Code owns these sibling sections.
  "inputs": [{"id":"api-key","type":"promptString","password":true}],
  "sandbox": {"filesystem":{"denyRead":["private/"]}},
  "servers": {"old":{"command":"old-server"}},
}`
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindMCP, Name: "new", Meta: map[string]any{
		"command": "new-server", "env": map[string]any{"API_KEY": "${input:api-key}"}, "sandboxEnabled": true,
	}}})
	cfg := &config.Config{Outputs: map[string]config.Output{"copilot": {MCPFile: path}}}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Inputs  []map[string]any `json:"inputs"`
		Sandbox map[string]any   `json:"sandbox"`
		Servers map[string]any   `json:"servers"`
	}
	if err := json.Unmarshal([]byte(readFile(t, path)), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Inputs) != 1 || got.Inputs[0]["id"] != "api-key" || got.Sandbox["filesystem"] == nil {
		t.Errorf("VS Code sibling settings lost: %+v", got)
	}
	if len(got.Servers) != 1 || got.Servers["new"] == nil {
		t.Errorf("servers map did not replace managed contents: %+v", got.Servers)
	}
}

func TestEmit_VSCodeMCPRefusesInvalidJSON(t *testing.T) {
	testutil.TempCwd(t)
	const path, existing = "broken.json", "{ broken config"
	if err := os.WriteFile(path, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindMCP, Name: "test", Meta: map[string]any{"command": "server"}}})
	cfg := &config.Config{Outputs: map[string]config.Output{"copilot": {MCPFile: path}}}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err == nil || !strings.Contains(err.Error(), path) {
		t.Errorf("malformed file needs path error: %v", err)
	}
	if got := readFile(t, path); got != existing {
		t.Errorf("malformed file changed: %q", got)
	}
}

func TestEmit_RemoteMCPPreservesWatchAndReportsUnsupportedDebug(t *testing.T) {
	testutil.TempCwd(t)
	notes := swapNoteWarner(t)
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{
		"type": "http", "url": "https://example.test/mcp",
		"dev": map[string]any{"watch": "src/**/*.ts", "debug": map[string]any{"type": "node"}},
	}}})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, ".vscode/mcp.json")
	if !strings.Contains(got, `"watch": "src/**/*.ts"`) || strings.Contains(got, `"debug"`) {
		t.Errorf("remote dev settings = %s", got)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "`dev.debug` on 1 mcp has no effect on copilot") {
		t.Errorf("missing debug coverage note: %s", notes.String())
	}
	if got := readFile(t, ".github/mcp.json"); strings.Contains(got, `"dev"`) {
		t.Errorf("VS Code settings leaked to CLI: %s", got)
	}
}
