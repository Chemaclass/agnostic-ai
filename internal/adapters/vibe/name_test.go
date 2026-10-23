package vibe

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MCPNativeIdentityRejectsBeforeWrites(t *testing.T) {
	for _, names := range [][]string{{"docs.api", "docs_api"}, {"---"}, {strings.Repeat("a", 256) + "b", strings.Repeat("a", 256) + "c"}} {
		t.Run(strings.Join(names, "/"), func(t *testing.T) {
			testutil.TempCwd(t)
			b := kitSinkBundle()
			b.MCPs = nil
			for _, name := range names {
				b.MCPs = append(b.MCPs, spec.Entry{Name: name, Meta: map[string]any{"command": "node"}})
			}
			if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err == nil {
				t.Error("invalid native identity accepted")
			}
			if _, err := os.Stat(".vibe"); !os.IsNotExist(err) {
				t.Error("native identity failure wrote files")
			}
		})
	}
}
func TestEmit_MCPSettingsArrayIdentityCannotBypassValidation(t *testing.T) {
	testutil.TempCwd(t)
	b := kitSinkBundle()
	b.MCPs = nil
	b.Settings = []spec.Entry{{Meta: map[string]any{"x-vibe": map[string]any{"mcp_servers": []map[string]any{{"name": "docs.api", "command": "a"}, {"name": "docs_api", "command": "b"}}}}}}
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err == nil {
		t.Error("settings native identity collision accepted")
	}
	if _, err := os.Stat(".vibe"); !os.IsNotExist(err) {
		t.Error("invalid settings wrote files")
	}
}

func TestNativeMCPName_MatchesPinnedRuntime(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"docs.api", "docs_api"}, {"a...b", "a___b"}, {"aé.b", "a__b"}, {"_-é-docs.api-_", "docs_api"}, {" --- ", ""}, {strings.Repeat("a", 255) + "-x", strings.Repeat("a", 255) + "-"}, {"Docs", "Docs"}, {"docs", "docs"}, {"__" + strings.Repeat("a", 256) + "b__", strings.Repeat("a", 256)},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			if got := nativeMCPName(tc.raw); got != tc.want {
				t.Errorf("got %q want %q", got, tc.want)
			}
		})
	}
}
func TestEmit_MCPRawNameAndSourcePrecedence(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{{Name: "docs.api", Meta: map[string]any{"command": "node", "x-vibe": map[string]any{"name": "ignored"}}}, {Name: "Docs", Meta: map[string]any{"command": "node"}}, {Name: "docs", Meta: map[string]any{"command": "node"}}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{MCPs: entries}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(".vibe/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `name = "docs.api"`) || strings.Contains(string(body), "ignored") {
		t.Fatalf("raw source spelling changed: %s", body)
	}
	record := map[string]any{"name": "docs.api", "command": "node"}
	canonical := (Adapter{}).CanonicalUserArrayRecord("mcp_servers", record).(map[string]any)
	if canonical["name"] != "docs_api" || record["name"] != "docs.api" {
		t.Fatal("comparison mutated native record")
	}
}
func TestEmit_MCPSplitConfigValidatesBothArrays(t *testing.T) {
	testutil.TempCwd(t)
	bundle := spec.Bundle{Settings: []spec.Entry{{Meta: map[string]any{"x-vibe": map[string]any{"mcp_servers": []any{map[string]any{"name": "---"}}}}}}, MCPs: []spec.Entry{{Name: "valid", Meta: map[string]any{"command": "node"}}}}
	cfg := &config.Config{Outputs: map[string]config.Output{"vibe": {MCPFile: ".vibe/separate.toml"}}}
	if err := New().Emit(emit.NewSession(), bundle, cfg, false); err == nil {
		t.Error("invalid separate settings array accepted")
	}
	if _, err := os.Stat(".vibe"); !os.IsNotExist(err) {
		t.Error("split configuration wrote before validation")
	}
}

func TestEmit_MCPIdentityValidatesOnlyFinalEmittedRecords(t *testing.T) {
	testutil.TempCwd(t)
	entries := []spec.Entry{{Name: "docs.api", Meta: map[string]any{"type": "sse", "url": "https://example.test"}}, {Name: "docs_api", Meta: map[string]any{"command": "node", "x-vibe": map[string]any{"name": "---"}}}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{MCPs: entries}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".vibe/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `name = "docs_api"`) || strings.Contains(string(data), "docs.api") {
		t.Fatalf("wrong final records: %s", data)
	}
}
