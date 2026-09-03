package emit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestMCPDocument_ServersMap(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y", "@modelcontextprotocol/server-filesystem"},
				"env":     map[string]any{"ROOT": "/tmp"},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	servers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers key: %s", got)
	}
	fs, ok := servers["fs"].(map[string]any)
	if !ok {
		t.Fatalf("expected fs server: %s", got)
	}
	if fs["command"] != "npx" {
		t.Errorf("command mismatch: %v", fs["command"])
	}
	args, _ := fs["args"].([]any)
	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d", len(args))
	}
	if _, ok := fs["type"]; ok {
		t.Errorf("ServersMap schema should not include type field")
	}
}

func TestMCPDocument_VSCodeServers(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y"},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaVSCodeServers)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"servers"`) {
		t.Errorf("expected servers key: %s", got)
	}
	if !strings.Contains(got, `"type": "stdio"`) {
		t.Errorf("expected type stdio: %s", got)
	}
}

func TestMCPDocument_HTTPTransport(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "remote",
			Meta: map[string]any{
				"type": "http",
				"url":  "https://example.com/mcp",
				"headers": map[string]any{
					"Authorization": "Bearer x",
				},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "https://example.com/mcp") {
		t.Errorf("missing url: %s", got)
	}
	if !strings.Contains(got, "Bearer x") {
		t.Errorf("missing header: %s", got)
	}
}

func TestMCPDocument_ServersMapNoTypeOmitsTransportDiscriminant(t *testing.T) {
	t.Parallel()
	// Warp's remote-server table documents only `url` and `headers`, and a
	// single tab covers "Streamable HTTP or SSE Server (URL)", so there is
	// no transport discriminant to emit (#592). Warp itself moved to its
	// own builder once its entry shape also diverged (#606); this variant
	// stays covered here as reusable infrastructure for the next vendor
	// in the same shape.
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "remote",
			Meta: map[string]any{
				"type":    "sse",
				"url":     "https://example.com/mcp",
				"headers": map[string]any{"Authorization": "Bearer x"},
			},
		},
		{
			Kind: spec.KindMCP,
			Name: "local",
			Meta: map[string]any{"command": "npx", "args": []any{"-y", "srv"}},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMapNoType)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, `"type"`) {
		t.Errorf("no-type schema must not emit a transport discriminant:\n%s", got)
	}
	for _, want := range []string{"https://example.com/mcp", "Bearer x", `"command"`, "npx"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q:\n%s", want, got)
		}
	}
}

func TestMCPDocument_ServersMapKeepsTypeForOtherTargets(t *testing.T) {
	t.Parallel()
	// Guard the default: removing the discriminant for warp must not
	// change what claude, cursor, and the rest emit.
	mcps := []spec.Entry{{
		Kind: spec.KindMCP,
		Name: "remote",
		Meta: map[string]any{"type": "sse", "url": "https://example.com/mcp"},
	}}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `"type": "sse"`) {
		t.Errorf("default schema must keep the sse discriminant:\n%s", got)
	}
}

// TestMCPDocument_CwdStaysDroppedForSharedBuilderTargets guards #606:
// warp's `cwd` -> `working_directory` mapping lives entirely in warp's
// own bespoke builder (internal/adapters/warp/mcp.go), not here. Only
// warp's vendor doc was confirmed to name a destination key for `cwd`;
// claude, cursor, copilot, factory, junie, kiro, and qoder all still
// share this builder (MCPSchemaServersMap covers the first six of those,
// MCPSchemaVSCodeServers covers copilot), so a `cwd` in their spec must
// keep silently dropping exactly as before this fix, not gain a guessed
// key of its own.
func TestMCPDocument_CwdStaysDroppedForSharedBuilderTargets(t *testing.T) {
	t.Parallel()
	for _, schema := range []MCPSchema{MCPSchemaServersMap, MCPSchemaVSCodeServers} {
		mcps := []spec.Entry{{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{"command": "npx", "cwd": "./backend"},
		}}
		got, err := MCPDocument(mcps, schema)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(got, "working_directory") {
			t.Errorf("schema %d: shared builder must not invent working_directory:\n%s", schema, got)
		}
		if strings.Contains(got, `"cwd"`) {
			t.Errorf("schema %d: shared builder must not pass cwd through verbatim either:\n%s", schema, got)
		}
	}
}

func TestMCPDocument_DescriptionAndDisabled(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "myserver",
			Meta: map[string]any{
				"command":     "npx",
				"description": "My server",
				"disabled":    true,
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	srv := parsed["mcpServers"].(map[string]any)["myserver"].(map[string]any)
	if srv["description"] != "My server" {
		t.Errorf("description not emitted: %v", got)
	}
	if srv["disabled"] != true {
		t.Errorf("disabled not emitted: %v", got)
	}
}

func TestMCPDocument_Roots(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"roots": []any{
					map[string]any{"uri": "file:///workspace", "name": "workspace"},
				},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "file:///workspace") {
		t.Errorf("roots uri not emitted: %s", got)
	}
}

func TestMCPDocument_SSETransport(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "events",
			Meta: map[string]any{
				"type": "sse",
				"url":  "https://example.com/sse",
				"headers": map[string]any{
					"Authorization": "Bearer abc",
				},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "https://example.com/sse") {
		t.Errorf("missing sse url: %s", got)
	}
	if !strings.Contains(got, "Bearer abc") {
		t.Errorf("missing sse header: %s", got)
	}
	// VSCode schema preserves the type discriminator.
	gotVS, err := MCPDocument(mcps, MCPSchemaVSCodeServers)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotVS, `"type": "sse"`) {
		t.Errorf("vscode schema should preserve type sse: %s", gotVS)
	}
}

func TestMCPDocument_StdioTypeOnlyWhenVSCodeSchema(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{"command": "npx", "args": []any{"-y"}},
		},
	}
	gotMap, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	// ServersMap schema omits the type discriminator for stdio (default).
	if strings.Contains(gotMap, `"type"`) {
		t.Errorf("servers-map schema should not emit type for stdio default: %s", gotMap)
	}
	gotVS, err := MCPDocument(mcps, MCPSchemaVSCodeServers)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotVS, `"type": "stdio"`) {
		t.Errorf("vscode schema must emit type stdio: %s", gotVS)
	}
}

func TestMCPDocument_EmptySkips(t *testing.T) {
	t.Parallel()
	got, err := MCPDocument(nil, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// StripMCPDisabled backs the claude/cursor/copilot fix for the
// target-audit finding that `disabled: true` is a no-op on those
// targets: none reads a per-server disable key from its project MCP
// file, so writing a dead key would let a user believe the server
// stopped connecting when it did not.
func TestStripMCPDisabled_RemovesKeyAndNotes(t *testing.T) {
	buf := swapWarnerForNotes(t)
	mcps := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "disabled": true}},
		{Kind: spec.KindMCP, Name: "db", Meta: map[string]any{"command": "pg", "description": "postgres"}},
	}
	out := StripMCPDisabled("claude", mcps, "no file-based way to pre-disable a project-scoped MCP server")

	got, err := MCPDocument(out, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "disabled") {
		t.Errorf("disabled key must not reach the emitted document: %s", got)
	}
	if !strings.Contains(got, "postgres") {
		t.Errorf("untouched entries must survive unchanged: %s", got)
	}

	FlushCoverageNotes()
	want := "  note: `disabled` on 1 mcp has no effect on claude (no file-based way to pre-disable a project-scoped MCP server)\n"
	if got := buf.String(); got != want {
		t.Errorf("expected field no-op note, got %q", got)
	}
}

// No entry sets `disabled`: no key to strip, no note to buffer, and the
// original entries pass through untouched.
func TestStripMCPDisabled_NoOpWhenNoneDisabled(t *testing.T) {
	buf := swapWarnerForNotes(t)
	mcps := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	out := StripMCPDisabled("cursor", mcps, "reason")
	if len(out) != 1 || out[0].Name != "fs" {
		t.Errorf("expected the single entry to pass through unchanged, got %+v", out)
	}
	FlushCoverageNotes()
	if buf.Len() != 0 {
		t.Errorf("expected no note when nothing was disabled, got: %s", buf)
	}
}

// The caller's slice and its Meta maps must not be mutated in place —
// other adapters (or a second call for a different target) may still
// hold a reference to the original bundle.
func TestStripMCPDisabled_DoesNotMutateInput(t *testing.T) {
	swapWarnerForNotes(t)
	original := map[string]any{"command": "npx", "disabled": true}
	mcps := []spec.Entry{{Kind: spec.KindMCP, Name: "fs", Meta: original}}
	StripMCPDisabled("claude", mcps, "reason")
	if _, ok := original["disabled"]; !ok {
		t.Errorf("caller's Meta map must not be mutated, disabled key was removed from it")
	}
}

// TestMCPDocument_WSTransport guards audit finding B8. Before `ws`
// shared the remote branch, a WebSocket entry matched neither the
// "stdio" nor the "http", "sse" case, so it was written with no
// command, no url, and no type: a malformed server object emitted with
// no warning, on both claude's .mcp.json and cursor's .cursor/mcp.json.
// Claude documents `type: "ws"` as taking the same url/headers fields
// as http, and Qoder documents the same transport.
func TestMCPDocument_WSTransport(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "socket",
			Meta: map[string]any{
				"type": "ws",
				"url":  "wss://example.com/ws",
				"headers": map[string]any{
					"Authorization": "Bearer xyz",
				},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	servers, ok := parsed["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("missing mcpServers: %s", got)
	}
	entry, ok := servers["socket"].(map[string]any)
	if !ok {
		t.Fatalf("missing socket server: %s", got)
	}
	if entry["url"] != "wss://example.com/ws" {
		t.Errorf("url = %v, want the ws url (dropped entirely before B8)", entry["url"])
	}
	if entry["type"] != "ws" {
		t.Errorf("type = %v, want %q so the transport is not ambiguous", entry["type"], "ws")
	}
	if _, ok := entry["headers"]; !ok {
		t.Errorf("headers dropped: %s", got)
	}
}

// cursor.com/docs/mcp.md documents `envFile` (stdio only) and an `auth`
// object (CLIENT_ID / CLIENT_SECRET / scopes, remote servers using
// `url`). Both are cursor-specific, so they must stay off by default
// and only appear when the caller opts in via WithCursorMCPExtras. See
// #661.
func TestMCPDocument_CursorExtrasOffByDefault(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{"command": "npx", "envFile": ".env"},
		},
		{
			Kind: spec.KindMCP,
			Name: "oauth-server",
			Meta: map[string]any{
				"type": "http",
				"url":  "https://example.test/mcp",
				"auth": map[string]any{"CLIENT_ID": "abc", "CLIENT_SECRET": "xyz", "scopes": []any{"read", "write"}},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "envFile") {
		t.Errorf("envFile must not appear without WithCursorMCPExtras: %s", got)
	}
	if strings.Contains(got, "CLIENT_ID") {
		t.Errorf("auth must not appear without WithCursorMCPExtras: %s", got)
	}
}

func TestMCPDocument_CursorExtras_EnvFileStdioOnly(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{"command": "npx", "envFile": "${workspaceFolder}/.env"},
		},
		{
			Kind: spec.KindMCP,
			Name: "remote",
			Meta: map[string]any{"type": "http", "url": "https://example.test/mcp", "envFile": ".env"},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap, WithCursorMCPExtras())
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	servers := parsed["mcpServers"].(map[string]any)
	fs := servers["fs"].(map[string]any)
	if fs["envFile"] != "${workspaceFolder}/.env" {
		t.Errorf("stdio envFile missing: %s", got)
	}
	remote := servers["remote"].(map[string]any)
	if _, ok := remote["envFile"]; ok {
		t.Errorf("remote server must not carry envFile (stdio only per vendor doc): %s", got)
	}
}

func TestMCPDocument_CursorExtras_AuthOnRemoteServers(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "oauth-server",
			Meta: map[string]any{
				"type": "http",
				"url":  "https://example.test/mcp",
				"auth": map[string]any{
					"CLIENT_ID":     "your-oauth-client-id",
					"CLIENT_SECRET": "your-client-secret",
					"scopes":        []any{"read", "write"},
				},
			},
		},
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"auth":    map[string]any{"CLIENT_ID": "irrelevant"},
			},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaServersMap, WithCursorMCPExtras())
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("invalid json: %v", err)
	}
	servers := parsed["mcpServers"].(map[string]any)
	oauth := servers["oauth-server"].(map[string]any)
	auth, ok := oauth["auth"].(map[string]any)
	if !ok {
		t.Fatalf("expected auth object: %s", got)
	}
	if auth["CLIENT_ID"] != "your-oauth-client-id" || auth["CLIENT_SECRET"] != "your-client-secret" {
		t.Errorf("auth CLIENT_ID/CLIENT_SECRET mismatch: %v", auth)
	}
	scopes, _ := auth["scopes"].([]any)
	if len(scopes) != 2 {
		t.Errorf("expected 2 scopes, got %v", auth["scopes"])
	}
	fs := servers["fs"].(map[string]any)
	if _, ok := fs["auth"]; ok {
		t.Errorf("stdio server must not carry auth (remote-only per vendor doc): %s", got)
	}
}

// The shared builder also serves claude, kiro, junie, qoder, factory,
// and copilot's root-mcp-file mirror. None documents envFile or auth,
// so WithCursorMCPExtras must stay opt-in rather than a schema-wide
// default; this test only guards that the option itself has no
// hard-coded schema check that would silently no-op it elsewhere.
func TestMCPDocument_CursorExtras_AppliesRegardlessOfSchema(t *testing.T) {
	t.Parallel()
	mcps := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{"command": "npx", "envFile": ".env"},
		},
	}
	got, err := MCPDocument(mcps, MCPSchemaVSCodeServers, WithCursorMCPExtras())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "envFile") {
		t.Errorf("expected envFile: the option itself is schema-independent; callers gate it by only passing it for cursor.go: %s", got)
	}
}
