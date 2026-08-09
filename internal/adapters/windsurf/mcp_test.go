package windsurf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MCP_StdioWritesDevinMCPConfigJSON(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y", "@modelcontextprotocol/server-filesystem", "."},
				"env":     map[string]any{"ALLOWED_PATHS": "."},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".devin/mcp_config.json"))
	if err != nil {
		t.Fatalf("missing .devin/mcp_config.json: %v", err)
	}
	body := string(got)
	for _, want := range []string{`"mcpServers"`, `"fs"`, `"command": "npx"`, `"ALLOWED_PATHS"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, `"transport"`) {
		t.Errorf("stdio entries carry no transport key; got:\n%s", body)
	}
}

// TestEmit_MCP_HTTPWritesTransportNotType locks the schema difference
// that makes this adapter its own builder rather than a reuse of
// emit.MCPSchemaServersMap: docs.devin.ai/cli/extensibility/mcp/
// configuration spells the remote discriminant `transport`, and the
// shared helper always writes `type` instead.
func TestEmit_MCP_HTTPWritesTransportNotType(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "linear",
			Meta: map[string]any{
				"type":    "http",
				"url":     "https://mcp.linear.app",
				"headers": map[string]any{"Authorization": "Bearer x"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".devin/mcp_config.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(got)
	for _, want := range []string{`"url": "https://mcp.linear.app"`, `"transport": "http"`, `"Authorization"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, `"type"`) {
		t.Errorf("the internal spec's own \"type\" meta selects the branch but must never itself reach the file; got:\n%s", body)
	}
}

func TestEmit_MCP_SSETransportPassesThrough(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "legacy",
			Meta: map[string]any{"type": "sse", "url": "https://mcp.example.test/sse"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".devin/mcp_config.json"))
	if !strings.Contains(got, `"transport": "sse"`) {
		t.Errorf("expected transport: sse to pass through, got:\n%s", got)
	}
}

// TestEmit_MCP_OAuthFieldsPassThrough confirms the three OAuth fields
// docs.devin.ai/cli/extensibility/mcp/configuration documents for a
// remote server (oauthClientId, oauthClientSecret, oauthResource) pass
// through verbatim: none has a counterpart in the shared
// emit.MCPSchemaServersMap builder, which is one more reason this
// adapter does not reuse it.
func TestEmit_MCP_OAuthFieldsPassThrough(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "github",
			Meta: map[string]any{
				"type":              "http",
				"url":               "https://mcp.example.com/mcp",
				"oauthClientId":     "Iv1.abc123",
				"oauthClientSecret": "${env:MY_MCP_CLIENT_SECRET}",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".devin/mcp_config.json"))
	for _, want := range []string{`"oauthClientId": "Iv1.abc123"`, `"oauthClientSecret": "${env:MY_MCP_CLIENT_SECRET}"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// TestEmit_MCP_OAuthResourceEmptyStringPassesThrough locks the vendor's
// three-state oauthResource behavior: unset (omitted here), a value,
// or an explicit empty string that omits the OAuth resource parameter
// entirely. An explicit "" must still reach the file rather than be
// treated the same as unset.
func TestEmit_MCP_OAuthResourceEmptyStringPassesThrough(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "custom",
			Meta: map[string]any{
				"type":          "http",
				"url":           "https://mcp.example.test/mcp",
				"oauthResource": "",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".devin/mcp_config.json"))
	if !strings.Contains(got, `"oauthResource": ""`) {
		t.Errorf("expected an explicit empty oauthResource to reach the file, got:\n%s", got)
	}
}

// TestEmit_MCP_DisabledIsARealKeyUnlikeTraeAndAntigravity confirms
// disabled: true reaches the file as a literal `disabled` key: unlike
// trae, Devin's own docs document a working per-server toggle
// ("This sets the "disabled": true flag on the server entry in the
// config file... A disabled server is skipped during tool discovery"),
// so this adapter never strips it.
func TestEmit_MCP_DisabledIsARealKeyUnlikeTraeAndAntigravity(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "disabled": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".devin/mcp_config.json"))
	if !strings.Contains(got, `"disabled": true`) {
		t.Errorf("expected disabled: true to reach the file, got:\n%s", got)
	}
}

func TestEmit_MCP_FileOverride(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"windsurf": {MCPFile: "vendor/devin-mcp.json"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/devin-mcp.json")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".devin/mcp_config.json")); !os.IsNotExist(err) {
		t.Errorf("override must not also write the default, err=%v", err)
	}
}

func TestEmit_NoMCPConfigJSONWhenNoMCPs(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "x"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".devin/mcp_config.json")); !os.IsNotExist(err) {
		t.Errorf("expected no .devin/mcp_config.json when no MCP entries, err=%v", err)
	}
}

// TestEmit_MCP_EntryMissingRequiredFieldSkipped confirms an entry with
// nothing to run or connect to (no command, no url) renders empty and
// is dropped rather than writing a dead {} block.
func TestEmit_MCP_EntryMissingRequiredFieldSkipped(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "empty-stdio", Meta: map[string]any{}},
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".devin/mcp_config.json"))
	if strings.Contains(got, "empty-stdio") {
		t.Errorf("expected the fieldless entry to be dropped:\n%s", got)
	}
	if !strings.Contains(got, `"fs"`) {
		t.Errorf("expected the valid entry to still be written:\n%s", got)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
