package crush

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func emitMCPTimeout(t *testing.T, timeout any) (server map[string]any, notes string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	meta := map[string]any{"type": "http", "url": "https://api.example.com/mcp/"}
	if timeout != nil {
		meta["timeout"] = timeout
	}
	entries := []spec.Entry{{Kind: spec.KindMCP, Name: "slow", Meta: meta}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	var doc struct {
		MCP map[string]map[string]any `json:"mcp"`
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, "crush.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.MCP["slow"], buf.String()
}

// Crush reads mcp.<name>.timeout in whole seconds (config.go: "Timeout in
// seconds for MCP server connections"); the spec holds milliseconds.
func TestEmit_MCP_TimeoutIsWrittenInWholeSeconds(t *testing.T) {
	cases := []struct {
		ms       any
		seconds  float64
		rounding bool
	}{
		{600000, 600, false},
		{3000, 3, false},
		{1500, 2, true},
		{400, 1, true},
		{float64(30000), 30, false},
	}
	for _, c := range cases {
		server, notes := emitMCPTimeout(t, c.ms)
		if got, _ := server["timeout"].(float64); got != c.seconds {
			t.Errorf("timeout %v ms: crush.json timeout = %v, want %v", c.ms, server["timeout"], c.seconds)
		}
		noted := strings.Contains(notes, "crush: MCP slow timeout")
		if noted != c.rounding {
			t.Errorf("timeout %v ms: rounding note = %v, want %v:\n%s", c.ms, noted, c.rounding, notes)
		}
	}
}

// Crush treats 0 as "use the default", so a zero or negative spec value
// writes no key rather than one Crush would read differently.
func TestEmit_MCP_NoTimeoutKeyWithoutAPositiveValue(t *testing.T) {
	for _, ms := range []any{nil, 0, -5} {
		server, notes := emitMCPTimeout(t, ms)
		if _, set := server["timeout"]; set {
			t.Errorf("timeout %v: want no timeout key, got %v", ms, server["timeout"])
		}
		if strings.Contains(notes, "timeout") {
			t.Errorf("timeout %v: want no note:\n%s", ms, notes)
		}
	}
}
