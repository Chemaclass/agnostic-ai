package junie

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

func emitJunieMCPTimeout(t *testing.T, extra map[string]any) (server map[string]any, notes string) {
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
	data, err := os.ReadFile(".junie/mcp/mcp.json")
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

// Junie ignores a timeout above 3600000 ms and uses 60 seconds
// (junie.jetbrains.com/docs/junie-cli-mcp-configuration.html), so sync
// writes the value as given and says so.
func TestEmit_MCPTimeoutAboveOneHourIsNoted(t *testing.T) {
	cases := []struct {
		meta  map[string]any
		ms    float64
		noted bool
	}{
		{map[string]any{"timeout": 4000000}, 4000000, true},
		{map[string]any{"timeout": 60000, "x-junie": map[string]any{"timeout": 4000000}}, 4000000, true},
		{map[string]any{"timeout": 3600000}, 3600000, false},
		{map[string]any{"timeout": 4000000, "x-junie": map[string]any{"timeout": 120000}}, 120000, false},
	}
	for _, c := range cases {
		server, notes := emitJunieMCPTimeout(t, c.meta)
		if got, _ := server["timeout"].(float64); got != c.ms {
			t.Errorf("%v: .junie/mcp/mcp.json timeout = %v, want %v", c.meta, server["timeout"], c.ms)
		}
		noted := strings.Contains(notes, "junie: MCP slow timeout") && strings.Contains(notes, "60 seconds")
		if noted != c.noted {
			t.Errorf("%v: note = %v, want %v:\n%s", c.meta, noted, c.noted, notes)
		}
	}
}
