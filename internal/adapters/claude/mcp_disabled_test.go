package claude

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MCPReenablePreservesManualRejections(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".claude", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".claude/settings.json", []byte(`{"disabledMcpjsonServers":["manual","already"],"model":"keep"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "generated", Meta: map[string]any{"command": "echo", "disabled": true}},
		{Kind: spec.KindMCP, Name: "already", Meta: map[string]any{"command": "echo", "disabled": true}},
	}
	for _, disable := range []bool{true, true, false} {
		for i := range entries {
			entries[i].Meta["disabled"] = disable
		}
		if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(".claude/settings.json")
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		want := []any{"manual", "already"}
		if disable {
			want = append(want, "generated")
		}
		if !reflect.DeepEqual(doc[rejectionKey], want) {
			t.Errorf("rejections=%v, want %v", doc[rejectionKey], want)
		}
		if doc["model"] != "keep" {
			t.Errorf("lost manual model: %s", raw)
		}
	}
}

func TestEmit_MCPDisableRefusesUnmanagedPolicyAndCustomMCPFile(t *testing.T) {
	for _, scenario := range []string{"settings", "state", "custom", "corrupt"} {
		t.Run(scenario, func(t *testing.T) {
			testutil.TempCwd(t)
			sess := emit.NewSession()
			cfg := &config.Config{}
			switch scenario {
			case "settings":
				sess.SetUnmanaged([]string{".claude/settings.json"})
			case "state":
				sess.SetUnmanaged([]string{".claude/.agnostic-ai-mcp-disabled.json"})
			case "custom":
				cfg.Outputs = map[string]config.Output{"claude": {MCPFile: "custom.json"}}
			case "corrupt":
				if err := os.MkdirAll(".claude", 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(".claude/.agnostic-ai-mcp-disabled.json", []byte(`{"broken":true}`), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			entry := spec.Entry{Kind: spec.KindMCP, Name: "server", Meta: map[string]any{"command": "echo", "disabled": true}}
			if err := New().Emit(sess, spec.NewBundle([]spec.Entry{entry}), cfg, false); err == nil {
				t.Error("expected actionable error")
			}
			if _, err := os.Stat(".mcp.json"); !os.IsNotExist(err) {
				t.Errorf("must not emit connectable MCP config: %v", err)
			}
		})
	}
}
