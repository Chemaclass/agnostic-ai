package continueai

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != "continue" {
		t.Errorf("Name() = %q, want %q", got, "continue")
	}
}

func TestEmit_WritesRulesAndAgents(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule"},
		{Kind: spec.KindAgent, Name: "ag1", Body: "agent"},
		{Kind: spec.KindSkill, Name: "sk1", Body: "skill"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".continue/rules/r1.md", ".continue/rules/agent-ag1.md", ".continue/rules/skill-sk1.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing %s", p)
		}
	}
}

func TestEmit_MCP_StdioWritesPerServerYAML(t *testing.T) {
	dir := testutil.TempCwd(t)

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
	got := readFile(t, filepath.Join(dir, ".continue/mcpServers/fs.yaml"))
	for _, want := range []string{
		"name: fs",
		"version:",
		"schema: v1",
		"mcpServers:",
		"command: npx",
		"@modelcontextprotocol/server-filesystem",
		"ALLOWED_PATHS",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

// Continue's URL branch takes `type: "sse" | "streamable-http"` and
// nothing else, so the canonical agnostic spelling `http` has to be
// translated on the way out or `blockSchema.parse` throws and the
// whole file fails to load. Headers belong under `requestOptions`,
// one level down.
func TestEmit_MCP_HTTPWritesStreamableHTTPAndNestedHeaders(t *testing.T) {
	dir := testutil.TempCwd(t)

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
	path := filepath.Join(dir, ".continue/mcpServers/linear.yaml")
	got := readFile(t, path)
	for _, want := range []string{"name: linear", "schema: v1", "mcpServers:", "https://mcp.linear.app"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}

	server := continueServer(t, path)
	if server["type"] != "streamable-http" {
		t.Errorf("type = %v, want %q (Continue's schema has no \"http\" literal)", server["type"], "streamable-http")
	}
	if _, ok := server["headers"]; ok {
		t.Errorf("headers written at the top level, where Continue's schema strips them: %s", got)
	}
	opts, ok := server["requestOptions"].(map[string]any)
	if !ok {
		t.Fatalf("requestOptions = %v, want a map carrying headers", server["requestOptions"])
	}
	headers, ok := opts["headers"].(map[string]any)
	if !ok {
		t.Fatalf("requestOptions.headers = %v, want a map", opts["headers"])
	}
	if headers["Authorization"] != "Bearer x" {
		t.Errorf("requestOptions.headers.Authorization = %v, want %q", headers["Authorization"], "Bearer x")
	}
}

// `sse` and `streamable-http` are literals Continue accepts verbatim,
// so neither is rewritten on the way out.
func TestEmit_MCP_VendorTransportSpellingsPassThrough(t *testing.T) {
	for _, transport := range []string{"sse", "streamable-http"} {
		t.Run(transport, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			entries := []spec.Entry{
				{
					Kind: spec.KindMCP,
					Name: "remote",
					Meta: map[string]any{"type": transport, "url": "https://example.test/mcp"},
				},
			}
			if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			server := continueServer(t, filepath.Join(dir, ".continue/mcpServers/remote.yaml"))
			if server["type"] != transport {
				t.Errorf("type = %v, want %q unchanged", server["type"], transport)
			}
		})
	}
}

// Continue documents no websocket transport, so a `ws` entry has no
// valid shape here. Writing it anyway produced a name-only server that
// matched neither branch of `mcpServerSchema` and threw on load.
func TestEmit_MCP_WebsocketSkipsFileAndNotesCoverage(t *testing.T) {
	dir := testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "socket", Meta: map[string]any{"type": "ws", "url": "wss://example.test/ws"}},
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".continue/mcpServers/socket.yaml")); !os.IsNotExist(err) {
		t.Errorf("expected no file for a ws server, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".continue/mcpServers/fs.yaml")); err != nil {
		t.Errorf("stdio server alongside a ws one must still emit: %v", err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "1 mcp") || !strings.Contains(buf.String(), "continue") {
		t.Errorf("expected a coverage note naming the skipped ws entry, got: %s", buf.String())
	}
}

// continueServer unmarshals an emitted block file and returns its single
// `mcpServers` element, so a test can assert nesting rather than the
// substring order yaml.Marshal happens to pick.
func continueServer(t *testing.T, path string) map[string]any {
	t.Helper()
	var doc struct {
		MCPServers []map[string]any `yaml:"mcpServers"`
	}
	if err := yaml.Unmarshal([]byte(readFile(t, path)), &doc); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
	if len(doc.MCPServers) != 1 {
		t.Fatalf("mcpServers has %d entries in %s, want 1", len(doc.MCPServers), path)
	}
	return doc.MCPServers[0]
}

func TestEmit_MCP_DirOverride(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"continue": {MCPDir: "vendor/mcp"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/mcp/fs.yaml")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_AssistantsDirEmitsAgentsAsAssistants(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindAgent,
			Name: "ship-it",
			Meta: map[string]any{"description": "open and merge a PR"},
			Body: "Run the release.",
		},
	}
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"continue": {AssistantsDir: ".continue/assistants"},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".continue/assistants/ship-it.yaml"))
	for _, want := range []string{
		"name: ship-it",
		"version: 0.0.1",
		"schema: v1",
		"description: open and merge a PR",
		"prompts:",
		"Run the release.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}

func TestEmit_NoAssistantsDirNoEmit(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "ag1", Body: "x"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".continue/assistants")); !os.IsNotExist(err) {
		t.Errorf("expected no assistants dir; err=%v", err)
	}
}

func TestEmit_OutputsCarryProvenanceHeader(t *testing.T) {
	dir := testutil.TempCwd(t)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"continue": {AssistantsDir: ".continue/assistants"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "rule body"},
		{Kind: spec.KindAgent, Name: "ag1", Meta: map[string]any{"description": "d"}, Body: "agent body"},
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "x"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		".continue/rules/r1.md",
		".continue/rules/agent-ag1.md",
		".continue/assistants/ag1.yaml",
		".continue/mcpServers/fs.yaml",
	} {
		got, err := os.ReadFile(filepath.Join(dir, p))
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if !strings.Contains(string(got), "Generated by agnostic-ai") {
			t.Errorf("%s missing provenance header:\n%s", p, got)
		}
	}
}

func TestEmit_MCP_NoFilesWhenNoEntries(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindRule, Name: "r1", Body: "x"},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".continue/mcpServers")); !os.IsNotExist(err) {
		t.Errorf("expected no mcpServers dir when no MCP entries, err=%v", err)
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
