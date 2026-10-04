package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestExplainCapabilities_AgentListsNativeNamesAndWidening(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindAgent, Meta: map[string]any{"can": []any{"read", "edit", "mcp:github/get_issue"}}}
	got := explainCapabilities(entry, &config.Config{Targets: []string{"kiro", "gemini", "codex"}})
	var out bytes.Buffer
	printExplainCapabilities(&out, got)
	for _, want := range []string{"[kiro] can: read → read", "[kiro] can: edit → write", "delete_file", "[kiro] can: mcp:github/get_issue → @github/get_issue", "[gemini] can: read → read_file", "[codex] can: read → unsupported"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
}

func TestExplainCapabilities_SettingsPreservesRuleAndList(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindSettings, Meta: map[string]any{"permissions": map[string]any{"deny": []any{"edit(.env)"}, "allow": []any{"read(src/**)"}}}}
	got := explainCapabilities(entry, &config.Config{Targets: []string{"claude"}})
	var out bytes.Buffer
	printExplainCapabilities(&out, got)
	for _, want := range []string{"[claude] permissions.allow: read(src/**) → Read(src/**)", "[claude] permissions.deny: edit(.env) → Edit(.env)"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
}

func TestExplainCapabilities_NativeAgentOverrideWins(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindAgent, Meta: map[string]any{"can": []any{"edit"}, "x-kiro": map[string]any{"tools": []any{"read"}}}}
	got := explainCapabilities(entry, &config.Config{Targets: []string{"kiro"}})
	var out bytes.Buffer
	printExplainCapabilities(&out, got)
	if !strings.Contains(out.String(), "[kiro] x-kiro.tools: read → read") || strings.Contains(out.String(), "delete_file") {
		t.Errorf("override must bypass capability mapping:\n%s", out.String())
	}
}

func TestExplain_AgentCapabilitiesJSONWorksInErrorMode(t *testing.T) {
	dir := setupExplainFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	if err := os.WriteFile("agnostic-ai.yaml", []byte("version: 1\nsources:\n  agents: agents\n  rules: rules\ntargets: [claude, kiro]\non-unsupported: error\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("agents", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("agents/writer.md", []byte("---\nname: writer\ndescription: Edit source files.\ncan: [edit]\n---\nEdit the requested file.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"explain", "agents/writer.md", "--json"})
	if err := root.Execute(); err != nil {
		t.Fatalf("explain in error mode: %v", err)
	}
	var got explainOutput
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatalf("decode explain: %v", err)
	}
	if len(got.Capabilities) != 2 {
		t.Fatalf("capabilities = %+v, want two targets", got.Capabilities)
	}
	if got.Capabilities[1].Target != "kiro" || got.Capabilities[1].Capability != "edit" || len(got.Capabilities[1].Widening) == 0 {
		t.Errorf("Kiro capability = %+v, want edit with widening", got.Capabilities[1])
	}
}

func TestExplainCapabilities_SettingsNativeOverridesMatchEmission(t *testing.T) {
	cases := []struct{ target, key, nativeKey string }{
		{"kilo", "permission", "read"},
		{"opencode", "permission", "read"},
		{"windsurf", "permissions", "deny"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			var override map[string]any
			if tc.target == "windsurf" {
				override = map[string]any{"deny": []any{"Read"}}
			} else {
				override = map[string]any{"read": "deny"}
			}
			entry := spec.Entry{Kind: spec.KindSettings, Name: "policy", Path: "settings/policy.yaml", Meta: map[string]any{"permissions": map[string]any{"allow": []any{"read"}}, "x-" + tc.target: map[string]any{tc.key: override}}}
			cfg := &config.Config{Targets: []string{tc.target}, OnUnsupported: "silent"}
			got := explainCapabilities(entry, cfg)
			expectedNative := "deny"
			if tc.target == "windsurf" {
				expectedNative = "allow: null"
			}
			if len(got) != 1 || got[0].Override == "" || len(got[0].Widening) != 0 || !strings.Contains(strings.Join(got[0].Native, " "), expectedNative) {
				t.Errorf("native override report=%+v,want %s with override", got, expectedNative)
			}
			adapter, err := adapters.Resolve(tc.target)
			if err != nil {
				t.Fatal(err)
			}
			files, err := captureEmit(adapter, spec.Bundle{Settings: []spec.Entry{entry}}, cfg)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, file := range files {
				if strings.Contains(file.Content, `"`+tc.nativeKey+`"`) && strings.Contains(file.Content, `"deny"`) {
					found = true
				}
			}
			if !found {
				t.Errorf("native deny missing from captured output: %+v", files)
			}
		})
	}
}

func TestExplainCapabilities_KiloAgentNativePermissionWins(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindAgent, Name: "reader", Path: "agents/reader.md", Meta: map[string]any{"can": []any{"read"}, "x-kilo": map[string]any{"permission": map[string]any{"read": "deny"}}}}
	got := explainCapabilities(entry, &config.Config{Targets: []string{"kilo"}})
	if len(got) != 1 || got[0].Override != "x-kilo.permission" || !strings.Contains(strings.Join(got[0].Native, " "), "deny") {
		t.Errorf("native agent permission = %+v, want read deny override", got)
	}
	adapter, err := adapters.Resolve("kilo")
	if err != nil {
		t.Fatal(err)
	}
	files, err := captureEmit(adapter, spec.Bundle{Agents: []spec.Entry{entry}}, &config.Config{Targets: []string{"kilo"}, OnUnsupported: "silent"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range files {
		if strings.Contains(file.Content, "read: deny") {
			found = true
		}
	}
	if !found {
		t.Errorf("native agent read deny missing: %+v", files)
	}
}

func TestExplainCapabilities_CodexUsesWholePermissionPolicy(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindSettings, Name: "policy", Path: "settings/policy.yaml", Meta: map[string]any{"permissions": map[string]any{"allow": []any{"shell(git push)"}}}}
	exclusion := spec.Entry{Kind: spec.KindSettings, Name: "deny", Path: "settings/deny.yaml", Meta: map[string]any{"permissions": map[string]any{"deny": []any{"shell(git *)"}}}}
	cfg := &config.Config{Targets: []string{"codex"}, Outputs: map[string]config.Output{"codex": {ExecPoliciesFromPermissions: true}}}
	got := explainCapabilities(entry, cfg, entry, exclusion)
	if len(got) != 1 || !got[0].Supported || len(got[0].Widening) != 0 {
		t.Errorf("excluded Codex allow = %+v, want supported without widening", got)
	}
	cfg.Outputs["codex"] = config.Output{ExecPoliciesFromPermissions: true, ExecPolicies: []config.CodexExecPolicy{}}
	got = explainCapabilities(entry, cfg, entry)
	if len(got) != 1 || got[0].Override == "" || len(got[0].Native) != 0 || len(got[0].Widening) != 0 {
		t.Errorf("authoritative empty Codex policies = %+v, want override with no portable prefix", got)
	}
}

func TestExplainCapabilities_AugmentNativeRulePrecedesPortableAllow(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindSettings, Name: "policy", Path: "settings/policy.yaml", Meta: map[string]any{"permissions": map[string]any{"allow": []any{"read"}}, "x-augment": map[string]any{"toolPermissions": []any{map[string]any{"toolName": "read", "permission": map[string]any{"type": "deny"}}}}}}
	cfg := &config.Config{Targets: []string{"augment"}, OnUnsupported: "silent"}
	got := explainCapabilities(entry, cfg)
	if len(got) != 1 || got[0].Override == "" || !strings.Contains(strings.Join(got[0].Native, " "), "deny") {
		t.Errorf("Augment native policy = %+v, want earlier deny", got)
	}
	adapter, err := adapters.Resolve("augment")
	if err != nil {
		t.Fatal(err)
	}
	files, err := captureEmit(adapter, spec.Bundle{Settings: []spec.Entry{entry}}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range files {
		deny, allow := strings.Index(file.Content, `"type": "deny"`), strings.Index(file.Content, `"type": "allow"`)
		if deny >= 0 && allow > deny {
			found = true
		}
	}
	if !found {
		t.Errorf("native deny must precede portable allow: %+v", files)
	}
}

func TestExplainCapabilities_DroppedNativeToolListsStayUnsupported(t *testing.T) {
	for _, target := range []string{"codex", "cursor"} {
		t.Run(target, func(t *testing.T) {
			entry := spec.Entry{Kind: spec.KindAgent, Name: "reader", Meta: map[string]any{"can": []any{"read"}, "x-" + target: map[string]any{"tools": []any{"Read"}}}}
			got := explainCapabilities(entry, &config.Config{Targets: []string{target}})
			if len(got) != 1 || got[0].Supported || len(got[0].Native) != 0 {
				t.Errorf("dropped native tool list report=%+v,want unsupported without native output", got)
			}
			translated := adapters.TranslateAgentCapabilityIn(target, "read", entry)
			if translated.Supported || len(translated.Native) != 0 {
				t.Errorf("dropped native tools translation=%+v,want unsupported", translated)
			}
		})
	}
}

func TestExplainCapabilities_CodexNativeToolMapMatchesEmission(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindAgent, Name: "reader", Path: "agents/reader.md", Meta: map[string]any{"description": "Read source.", "x-codex": map[string]any{"tools": map[string]any{"web_search": false}}}}
	got := explainCapabilities(entry, &config.Config{Targets: []string{"codex"}})
	if len(got) != 1 || !got[0].Supported || got[0].Override != "x-codex.tools" || !strings.Contains(strings.Join(got[0].Native, " "), "web_search") {
		t.Errorf("native tool map report=%+v,want emitted tools configuration", got)
	}
	adapter, err := adapters.Resolve("codex")
	if err != nil {
		t.Fatal(err)
	}
	files, err := captureEmit(adapter, spec.Bundle{Agents: []spec.Entry{entry}}, &config.Config{Targets: []string{"codex"}, OnUnsupported: "silent"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range files {
		if strings.Contains(file.Content, "[tools]") && strings.Contains(file.Content, "web_search = false") {
			found = true
		}
	}
	if !found {
		t.Errorf("native Codex tool table absent: %+v", files)
	}
}
