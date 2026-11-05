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
// seconds"); the spec holds milliseconds.
func TestEmit_MCPTimeoutIsWrittenInWholeSeconds(t *testing.T) {
	cases := []struct {
		meta     map[string]any
		seconds  float64
		rounding bool
	}{
		{map[string]any{"timeout": 30000}, 30, false},
		{map[string]any{"timeout": 1500}, 2, true},
		{map[string]any{"timeout": 30000, "x-pi": map[string]any{"timeout": 45}}, 45, false},
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
	if !strings.Contains(notes, "pi: MCP slow timeout 30 is not a whole number") {
		t.Errorf("want a note:\n%s", notes)
	}
}
