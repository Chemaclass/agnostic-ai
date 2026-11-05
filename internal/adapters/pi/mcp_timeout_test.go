package pi

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func emitPiMCPTimeout(t *testing.T, extra map[string]any) (server map[string]any, notes string) {
	t.Helper()
	testutil.TempCwd(t)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	meta := map[string]any{"type": "http", "url": "https://example.com/mcp"}
	for k, v := range extra {
		meta[k] = v
	}
	bundle := spec.NewBundle([]spec.Entry{{Kind: spec.KindMCP, Name: "slow", Meta: meta}})
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	data, err := os.ReadFile(".pi/mcp.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Servers["slow"], buf.String()
}

// Pi reads timeout in seconds (docs/mcp.md: "per-request timeout in
// seconds") and accepts fractions, so milliseconds divide exactly.
func TestEmit_MCPTimeoutIsWrittenInSeconds(t *testing.T) {
	cases := []struct {
		meta     map[string]any
		seconds  float64
		rounding bool
	}{
		{map[string]any{"timeout": 30000}, 30, false},
		{map[string]any{"timeout": 1500}, 1.5, false},
		{map[string]any{"timeout": 30}, 0.03, true},
		{map[string]any{"timeout": 30000, "x-pi": map[string]any{"timeout": 45}}, 45, false},
		{map[string]any{"x-pi": map[string]any{"timeout": 1.5}}, 1.5, false},
	}
	for _, c := range cases {
		server, notes := emitPiMCPTimeout(t, c.meta)
		if got, _ := server["timeout"].(float64); got != c.seconds {
			t.Errorf("%v: .pi/mcp.json timeout = %v, want %v", c.meta, server["timeout"], c.seconds)
		}
		if noted := strings.Contains(notes, "pi: MCP slow timeout"); noted != c.rounding {
			t.Errorf("%v: rounding note = %v, want %v:\n%s", c.meta, noted, c.rounding, notes)
		}
	}
}

func TestEmit_MCPTextTimeoutIsNoted(t *testing.T) {
	server, notes := emitPiMCPTimeout(t, map[string]any{"timeout": "30"})
	if _, set := server["timeout"]; set {
		t.Errorf("want no timeout key, got %v", server["timeout"])
	}
	if !strings.Contains(notes, "pi: MCP slow timeout \"30\" is text") {
		t.Errorf("want a note:\n%s", notes)
	}
}

// A value under one second most likely holds seconds from an older spec;
// Crush gets a rounding note for it, so Pi needs its own.
func TestEmit_MCPTimeoutUnderASecondIsNoted(t *testing.T) {
	server, notes := emitPiMCPTimeout(t, map[string]any{"timeout": 40})
	if got, _ := server["timeout"].(float64); got != 0.04 {
		t.Errorf("timeout = %v, want 0.04", server["timeout"])
	}
	if !strings.Contains(notes, "pi: MCP slow timeout 40 ms") || !strings.Contains(notes, "multiply it by 1000") {
		t.Errorf("want a note that the value looks like seconds:\n%s", notes)
	}
}

// Pi rejects a timeout that is not positive and skips the server
// (mcp-servers.ts: "timeout must be a positive number of seconds").
func TestEmit_MCPOverrideTimeoutMustBePositive(t *testing.T) {
	for _, value := range []any{0, -5, 0.0} {
		server, notes := emitPiMCPTimeout(t, map[string]any{"x-pi": map[string]any{"timeout": value}})
		if _, set := server["timeout"]; set {
			t.Errorf("x-pi.timeout %v: want no key, got %v", value, server["timeout"])
		}
		if !strings.Contains(notes, "x-pi.timeout") {
			t.Errorf("x-pi.timeout %v: want a note:\n%s", value, notes)
		}
	}
}
