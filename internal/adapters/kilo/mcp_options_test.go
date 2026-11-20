package kilo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MCPUsesOnlyPositiveWholeTimeouts(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	var notes strings.Builder
	previousWarner := emit.Warner
	emit.Warner = &notes
	t.Cleanup(func() { emit.Warner = previousWarner })
	b := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindMCP, Name: "local", Meta: map[string]any{"command": "server", "timeout": 0, "oauth": false}},
		{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{"type": "http", "url": "https://example.test/mcp", "timeout": 45000, "oauth": false}},
		{Kind: spec.KindMCP, Name: "custom", Meta: map[string]any{"type": "sse", "url": "https://example.test/sse", "timeout": 99, "x-kilo": map[string]any{"timeout": 0, "oauth": false}}},
		{Kind: spec.KindMCP, Name: "negative", Meta: map[string]any{"command": "server", "timeout": -5}},
		{Kind: spec.KindMCP, Name: "fractional", Meta: map[string]any{"command": "server", "timeout": 1.5}},
		{Kind: spec.KindMCP, Name: "largest", Meta: map[string]any{"command": "server", "timeout": int64(9007199254740991)}},
		{Kind: spec.KindMCP, Name: "too-large", Meta: map[string]any{"command": "server", "timeout": int64(9007199254740992)}},
		{Kind: spec.KindMCP, Name: "default", Meta: map[string]any{"command": "server"}},
	})
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Servers map[string]map[string]any `json:"mcp"`
	}
	if err := json.Unmarshal([]byte(readFile(t, "kilo.jsonc")), &got); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"local", "custom", "negative", "fractional", "too-large", "default"} {
		server, exists := got.Servers[name]
		if !exists {
			t.Errorf("%s server missing: %#v", name, got.Servers)
		}
		if timeout, exists := server["timeout"]; exists {
			t.Errorf("%s timeout = %#v, want omitted", name, timeout)
		}
	}
	if got.Servers["remote"]["timeout"] != float64(45000) {
		t.Errorf("remote timeout = %#v, want 45000", got.Servers["remote"]["timeout"])
	}
	if got.Servers["largest"]["timeout"] != float64(9007199254740991) {
		t.Errorf("largest timeout = %#v, want 9007199254740991", got.Servers["largest"]["timeout"])
	}
	if _, exists := got.Servers["local"]["oauth"]; exists {
		t.Error("OAuth must not emit on stdio")
	}
	for _, name := range []string{"remote", "custom"} {
		if got.Servers[name]["oauth"] != false {
			t.Errorf("%s OAuth disablement lost: %#v", name, got.Servers[name])
		}
	}
	emit.FlushCoverageNotes()
	for _, name := range []string{"local", "custom", "negative", "fractional", "too-large"} {
		if !strings.Contains(notes.String(), "MCP "+name+" timeout") {
			t.Errorf("note did not identify the invalid timeout on %s: %s", name, &notes)
		}
	}
	if strings.Contains(notes.String(), "MCP default timeout") {
		t.Errorf("note flagged an omitted timeout: %s", &notes)
	}
}
