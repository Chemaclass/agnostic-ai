package continueai

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MCP_PreservesStdioConnectionOptions(t *testing.T) {
	dir := testutil.TempCwd(t)
	entry := spec.Entry{
		Kind: spec.KindMCP, Name: "workspace",
		Meta: map[string]any{
			"command":           "./server",
			"cwd":               "./tools",
			"connectionTimeout": 15000,
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	server := continueServer(t, filepath.Join(dir, ".continue/mcpServers/workspace.yaml"))
	if server["cwd"] != "./tools" {
		t.Errorf("cwd = %v, want ./tools", server["cwd"])
	}
	if server["connectionTimeout"] != 15000 {
		t.Errorf("connectionTimeout = %v, want 15000", server["connectionTimeout"])
	}
}

func TestEmit_MCP_MergesRemoteRequestOptions(t *testing.T) {
	dir := testutil.TempCwd(t)
	options := map[string]any{
		"caBundlePath": []any{"/etc/company-ca.pem"},
		"proxy":        "http://proxy.test:8080",
		"timeout":      30000,
		"headers":      map[string]any{"Authorization": "Bearer native", "X-Native": "native"},
	}
	entry := spec.Entry{
		Kind: spec.KindMCP, Name: "remote",
		Meta: map[string]any{
			"type":              "http",
			"url":               "https://example.test/mcp",
			"connectionTimeout": 12000,
			"headers":           map[string]any{"Authorization": "Bearer portable", "X-Portable": "portable"},
			"requestOptions":    options,
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	server := continueServer(t, filepath.Join(dir, ".continue/mcpServers/remote.yaml"))
	wantOptions := map[string]any{
		"caBundlePath": []any{"/etc/company-ca.pem"},
		"proxy":        "http://proxy.test:8080",
		"timeout":      30000,
		"headers": map[string]any{
			"Authorization": "Bearer native",
			"X-Native":      "native",
			"X-Portable":    "portable",
		},
	}
	if !reflect.DeepEqual(server["requestOptions"], wantOptions) {
		t.Errorf("requestOptions = %#v, want %#v", server["requestOptions"], wantOptions)
	}
	if server["connectionTimeout"] != 12000 {
		t.Errorf("connectionTimeout = %v, want 12000", server["connectionTimeout"])
	}
	if _, ok := options["headers"].(map[string]any)["X-Portable"]; ok {
		t.Error("emission mutated the input requestOptions headers")
	}
}

func TestEmit_MCP_ContinueOverridesConnectionOptions(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{
			Kind: spec.KindMCP, Name: "workspace",
			Meta: map[string]any{
				"command":           "./server",
				"cwd":               "./shared",
				"connectionTimeout": 1000,
				"x-continue": map[string]any{
					"cwd":               "./continue",
					"connectionTimeout": 2000,
				},
			},
		},
		{
			Kind: spec.KindMCP, Name: "remote",
			Meta: map[string]any{
				"type":           "http",
				"url":            "https://example.test/mcp",
				"headers":        map[string]any{"Authorization": "Bearer portable", "X-Portable": "portable"},
				"requestOptions": map[string]any{"proxy": "http://shared.test:8080", "timeout": 1000},
				"x-continue": map[string]any{
					"requestOptions": map[string]any{
						"proxy":   "http://continue.test:8080",
						"timeout": 2000,
						"headers": map[string]any{"Authorization": "Bearer continue"},
					},
				},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	stdio := continueServer(t, filepath.Join(dir, ".continue/mcpServers/workspace.yaml"))
	if stdio["cwd"] != "./continue" || stdio["connectionTimeout"] != 2000 {
		t.Errorf("Continue stdio overrides lost: %#v", stdio)
	}
	remote := continueServer(t, filepath.Join(dir, ".continue/mcpServers/remote.yaml"))
	want := map[string]any{
		"proxy":   "http://continue.test:8080",
		"timeout": 2000,
		"headers": map[string]any{"Authorization": "Bearer continue", "X-Portable": "portable"},
	}
	if !reflect.DeepEqual(remote["requestOptions"], want) {
		t.Errorf("requestOptions = %#v, want %#v", remote["requestOptions"], want)
	}
}

func TestEmit_MCP_ValidatesContinueOverrides(t *testing.T) {
	dir := testutil.TempCwd(t)
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "unsupported", Meta: map[string]any{
			"command": "server", "x-continue": map[string]any{"type": "ws"},
		}},
		{Kind: spec.KindMCP, Name: "no-command", Meta: map[string]any{
			"command": "server", "x-continue": map[string]any{"command": nil},
		}},
		{Kind: spec.KindMCP, Name: "no-url", Meta: map[string]any{
			"type": "http", "url": "https://example.test/mcp", "x-continue": map[string]any{"url": nil},
		}},
	}
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if paths := testutil.WalkRel(t, dir); len(paths) != 0 {
		t.Errorf("invalid Continue overrides emitted files: %v", paths)
	}
}
