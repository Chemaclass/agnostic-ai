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
	meta := map[string]any{}
	if timeout != nil {
		meta["timeout"] = timeout
	}
	return emitMCPTimeoutMeta(t, meta)
}

func emitMCPTimeoutMeta(t *testing.T, extra map[string]any) (server map[string]any, notes string) {
	t.Helper()
	dir := testutil.TempCwd(t)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	meta := map[string]any{"type": "http", "url": "https://api.example.com/mcp/"}
	for k, v := range extra {
		meta[k] = v
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

// x-crush.timeout is already in Crush's seconds, so it is written as
// given and wins over the portable milliseconds.
func TestEmit_MCP_XCrushTimeoutIsSecondsAndWins(t *testing.T) {
	server, notes := emitMCPTimeoutMeta(t, map[string]any{
		"timeout": 600000, "x-crush": map[string]any{"timeout": 45},
	})
	if got, _ := server["timeout"].(float64); got != 45 {
		t.Errorf("crush.json timeout = %v, want 45", server["timeout"])
	}
	if strings.Contains(notes, "timeout") {
		t.Errorf("want no note:\n%s", notes)
	}
}

// A timeout Crush cannot read writes no key, and the note says why
// instead of dropping it silently.
func TestEmit_MCP_TextTimeoutIsNoted(t *testing.T) {
	for _, meta := range []map[string]any{
		{"timeout": "30"},
		{"x-crush": map[string]any{"timeout": "30s"}},
		{"x-crush": map[string]any{"timeout": 1.5}},
	} {
		server, notes := emitMCPTimeoutMeta(t, meta)
		if _, set := server["timeout"]; set {
			t.Errorf("%v: want no timeout key, got %v", meta, server["timeout"])
		}
		if !strings.Contains(notes, "crush: MCP slow") || !strings.Contains(notes, "is not a") {
			t.Errorf("%v: want a note naming the server:\n%s", meta, notes)
		}
	}
}
