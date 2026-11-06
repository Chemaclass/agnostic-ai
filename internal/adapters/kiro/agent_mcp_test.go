package kiro

import (
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestAgentMCPServers_WritesKiroTimeouts(t *testing.T) {
	notes := swapNoteWarner(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	servers := New().AgentMCPServers([]spec.Entry{{
		Kind: spec.KindMCP, Name: "db",
		Meta: map[string]any{
			"command": "db-mcp",
			"x-kiro":  map[string]any{"timeout": 90000, "requestTimeout": 300000},
		},
	}})

	got := servers["db"].(map[string]any)
	if got["timeout"] != 90000 || got["requestTimeout"] != 300000 {
		t.Fatalf("server = %v, want timeout 90000 and requestTimeout 300000", got)
	}
	emit.FlushCoverageNotes()
	if notes.Len() != 0 {
		t.Errorf("unexpected note: %s", notes.String())
	}
}

func TestAgentMCPServers_NotesPortableTimeout(t *testing.T) {
	notes := swapNoteWarner(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	servers := New().AgentMCPServers([]spec.Entry{{
		Kind: spec.KindMCP, Name: "db",
		Meta: map[string]any{"command": "db-mcp", "timeout": 45000},
	}})

	want := map[string]any{"db": map[string]any{"command": "db-mcp"}}
	if !reflect.DeepEqual(servers, want) {
		t.Fatalf("servers = %v, want %v", servers, want)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "x-kiro.timeout") || !strings.Contains(notes.String(), "x-kiro.requestTimeout") {
		t.Errorf("note must name both Kiro keys: %s", notes.String())
	}
}

func TestAgentMCPServers_SkipsInvalidKiroTimeout(t *testing.T) {
	notes := swapNoteWarner(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)

	servers := New().AgentMCPServers([]spec.Entry{{
		Kind: spec.KindMCP, Name: "db",
		Meta: map[string]any{
			"command": "db-mcp",
			"x-kiro":  map[string]any{"timeout": "slow", "requestTimeout": -1},
		},
	}})

	got := servers["db"].(map[string]any)
	if _, set := got["timeout"]; set {
		t.Errorf("invalid timeout written: %v", got)
	}
	if _, set := got["requestTimeout"]; set {
		t.Errorf("invalid requestTimeout written: %v", got)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(notes.String(), "x-kiro.timeout") || !strings.Contains(notes.String(), "x-kiro.requestTimeout") {
		t.Errorf("note must name each invalid key: %s", notes.String())
	}
}
