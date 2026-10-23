package vibe

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_NativeSixKinds(t *testing.T) {
	testutil.TempCwd(t)
	b := kitSinkBundle()
	cfg := &config.Config{Outputs: map[string]config.Output{"vibe": {RulesFile: "rules.md"}}}
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatal(err)
	}
	for path, wants := range map[string][]string{
		".vibe/agents/reviewer.toml":  {`agent_type = "subagent"`, `enabled_tools = ["read_file"]`, "Review changes"},
		".vibe/skills/check/SKILL.md": {"name: check", "disable-model-invocation: true", "Run checks"},
		".vibe/config.toml":           {`active_model = "devstral"`, `[[mcp_servers]]`, `transport = "streamable-http"`, `[mcp_servers.auth]`},
		".vibe/hooks.toml":            {`type = "pre_tool"`, `match = "bash"`, `command =`},
		"rules.md":                    {"Keep changes small"},
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s missing %q: %s", path, want, data)
			}
		}
		if filepath.Ext(path) == ".toml" {
			if _, err := toml.Decode(string(data), new(map[string]any)); err != nil {
				t.Fatal(err)
			}
		}
	}
	data, err := os.ReadFile(".vibe/hooks.toml")
	if err != nil {
		t.Fatal(err)
	}
	var hooks struct{ Hooks []struct{ Command string } }
	if _, err := toml.Decode(string(data), &hooks); err != nil {
		t.Fatal(err)
	}
	if len(hooks.Hooks) != 1 || DeclaredHookCommand(hooks.Hooks[0].Command) != "echo '{}'" {
		t.Fatalf("native hook command or target wrapper changed: %s", data)
	}

}
func TestEmit_UnknownAgentToolFailsClosed(t *testing.T) {
	testutil.TempCwd(t)
	a := spec.Entry{Kind: spec.KindAgent, Name: "restricted", Meta: map[string]any{"tools": []any{"UnknownTool"}}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{Agents: []spec.Entry{a}}, &config.Config{}, false); err == nil {
		t.Fatal("unknown allowlist must not become all tools")
	}
}
func TestEmit_RejectsInvalidSkill(t *testing.T) {
	testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.Bundle{Skills: []spec.Entry{{Name: "INVALID", Meta: map[string]any{"description": "d"}}}}, &config.Config{}, false); err == nil {
		t.Fatal("invalid native skill must fail")
	}
}
func kitSinkBundle() spec.Bundle {
	return spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "small", Body: "Keep changes small"},
		{Kind: spec.KindAgent, Name: "reviewer", Body: "Review changes", Meta: map[string]any{"description": "Review", "can": []any{"read"}}},
		{Kind: spec.KindSkill, Name: "check", Body: "Run checks", Meta: map[string]any{"description": "Run tests", "disable-model-invocation": true}},
		{Kind: spec.KindSettings, Name: "model", Meta: map[string]any{"model": "devstral"}},
		{Kind: spec.KindMCP, Name: "docs", Meta: map[string]any{"type": "http", "url": "https://example.com/mcp", "headers": map[string]any{"X-Test": "ok"}}},
		{Kind: spec.KindMCP, Name: "local", Meta: map[string]any{"command": "node", "args": []any{"server.js"}}},
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "pre_tool", "matcher": "bash", "command": "echo '{}'", "timeout": 10}},
	})
}
func TestKitSink_GoldenSnapshot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{Outputs: map[string]config.Output{"vibe": {RulesFile: "rules.md"}}}, false); err != nil {
		t.Fatal(err)
	}
	// Fixed golden bytes use POSIX wrapping; Windows emission is checked separately.
	if err := emitHooks(emit.NewSession(), kitSinkBundle().Hooks, ".vibe/hooks.toml", ".vibe/hooks", false, "linux"); err != nil {
		t.Fatal(err)
	}
	testutil.AssertGoldenTree(t, dir, filepath.Join(cwd, "testdata", "kitsink"))
}

func TestHookCommand_PreservesTargetAndDeclaredCommandOnWindows(t *testing.T) {
	for _, platform := range []string{"windows", "linux"} {
		command := `python guard.py "words with spaces" && echo '{}'`
		wrapped := hookCommand(command, platform)
		if platform == "windows" && strings.Contains(wrapped, "export ") {
			t.Fatal("POSIX export reached native Windows shells")
		}
		if got := DeclaredHookCommand(wrapped); got != command {
			t.Errorf("%s roundtrip: got %q want %q", platform, got, command)
		}
	}
}

func TestEmit_UsesOutputOverridesAndCopiesScopedSkillAssets(t *testing.T) {
	dir := testutil.TempCwd(t)
	assetDir := filepath.Join(dir, "source", "check")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "SKILL.md"), []byte("source"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(assetDir, "guide.txt"), []byte("resource"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle := kitSinkBundle()
	bundle.Skills[0].Path = filepath.Join(assetDir, "SKILL.md")
	bundle.Skills[0].Scope = "backend"
	cfg := &config.Config{Outputs: map[string]config.Output{"vibe": {AgentsDir: "native/agents", SkillsDir: "native/skills", ConfFile: "native/config.toml", MCPFile: "native/mcp.toml", HooksFile: "native/hooks.toml", RulesFile: "native/rules.md"}}}
	if err := New().Emit(emit.NewSession(), bundle, cfg, false); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"native/agents/reviewer.toml": "subagent", "backend/native/skills/check/guide.txt": "resource", "native/config.toml": "active_model", "native/mcp.toml": "mcp_servers", "native/hooks.toml": "pre_tool", "native/rules.md": "Keep changes small"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s missing %q", path, want)
		}
	}
	data, err := os.ReadFile("native/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "mcp_servers") {
		t.Fatal("separate MCP override leaked into model config")
	}
}
func TestEmit_RemovesStaleManagedConfiguration(t *testing.T) {
	testutil.TempCwd(t)
	sess := emit.NewSession()
	if err := New().Emit(sess, kitSinkBundle(), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(sess, spec.Bundle{}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".vibe/config.toml", ".vibe/hooks.toml"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("stale %s remains: %v", path, err)
		}
	}
}

func TestEmit_NativeModelOverrideWinsPortableModel(t *testing.T) {
	testutil.TempCwd(t)
	b := spec.Bundle{Settings: []spec.Entry{{Meta: map[string]any{"model": "portable", "x-vibe": map[string]any{"active_model": "native"}}}}}
	if err := New().Emit(emit.NewSession(), b, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".vibe/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `active_model = "native"`) {
		t.Fatalf("native model lost: %s", data)
	}
}
func TestEmit_RejectsMCPWithoutNativeLaunchFields(t *testing.T) {
	testutil.TempCwd(t)
	for _, meta := range []map[string]any{{"type": "stdio"}, {"type": "http", "url": ""}} {
		if err := New().Emit(emit.NewSession(), spec.Bundle{MCPs: []spec.Entry{{Name: "bad", Meta: meta}}}, &config.Config{}, false); err == nil {
			t.Errorf("invalid native MCP accepted: %v", meta)
		}
	}
}

func TestHookCommand_ExecutesNativeJSONDecision(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX command contract; Windows encoding has a separate round-trip test")
	}
	wrapped := hookCommand(`printf '{"decision":"deny","reason":"%s"}' "$AGNOSTIC_AI_TARGET"`, "linux")
	command := exec.Command("sh", "-c", wrapped)
	command.Stdin = strings.NewReader(`{"tool_name":"bash","tool_input":{"command":"rm -rf /tmp/example"}}`)
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	var decision struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(output, &decision); err != nil {
		t.Fatal(err)
	}
	if decision.Decision != "deny" || decision.Reason != "vibe" {
		t.Fatalf("native decision changed: %s", output)
	}
}

func TestEmit_CapabilityMatrixCoversEveryDeclaredKind(t *testing.T) {
	testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"vibe": {RulesFile: "rules.md"}}}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatal(err)
	}
	paths := map[spec.Kind]string{spec.KindAgent: ".vibe/agents/reviewer.toml", spec.KindSkill: ".vibe/skills/check/SKILL.md", spec.KindRule: "rules.md", spec.KindHook: ".vibe/hooks.toml", spec.KindMCP: ".vibe/config.toml", spec.KindSettings: ".vibe/config.toml"}
	for _, kind := range caps.Supports {
		path, ok := paths[kind]
		if !ok {
			t.Errorf("declared kind %s has no output assertion", kind)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Error(err)
			continue
		}
		if len(data) == 0 {
			t.Errorf("declared kind %s emitted an empty file", kind)
		}
	}
}

func TestEmit_DropsUnknownToolsAndKeepsKnownRestriction(t *testing.T) {
	testutil.TempCwd(t)
	agent := spec.Entry{Kind: spec.KindAgent, Name: "restricted", Meta: map[string]any{"tools": []any{"Read", "Glob", "WebFetch"}}, Body: "Review"}
	if err := New().Emit(emit.NewSession(), spec.Bundle{Agents: []spec.Entry{agent}}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".vibe/agents/restricted.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `enabled_tools = ["read_file"]`) {
		t.Fatalf("known restrictive tools lost: %s", data)
	}
}

func TestHookCommand_WindowsArgsFailWhenPowerShellWouldChangeThem(t *testing.T) {
	if _, err := nativeHookCommand("python", []string{"guard.py", "two words"}, "windows"); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{""}, {"a'b"}, {`a"b`}} {
		if _, err := nativeHookCommand("python", args, "windows"); err == nil {
			t.Errorf("Windows arguments silently changed: %q", args)
		}
	}
}

func TestEmit_NativeToolPolicyKeepsPortableAllowlist(t *testing.T) {
	testutil.TempCwd(t)
	agent := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"can": []any{"read"}, "x-vibe": map[string]any{"tools": map[string]any{"read_file": map[string]any{"permission": "always"}}}}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{Agents: []spec.Entry{agent}}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".vibe/agents/reviewer.toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`enabled_tools = ["read_file"]`, `permission = "always"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("native policy lost portable restriction %q: %s", want, data)
		}
	}
}

func TestEmit_MaterializesWindowsHookVariant(t *testing.T) {
	testutil.TempCwd(t)
	for path, body := range map[string]string{".agnostic-ai/scripts/guard.sh": "posix body\n", ".agnostic-ai/scripts/guard.ps1": "windows body\n"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "pre_tool", "command": "sh .agnostic-ai/scripts/guard.sh", "commandWindows": "powershell -File .agnostic-ai/scripts/guard.ps1"}}
	if err := emitHooks(emit.NewSession(), []spec.Entry{hook}, ".vibe/hooks.toml", ".vibe/hooks", false, "windows"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".vibe/hooks/guard.ps1")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "windows body\n" {
		t.Fatalf("Windows variant changed: %s", data)
	}
	doc, err := os.ReadFile(".vibe/hooks.toml")
	if err != nil {
		t.Fatal(err)
	}
	var native map[string]any
	if _, err := toml.Decode(string(doc), &native); err != nil {
		t.Fatal(err)
	}
	records := native["hooks"].([]map[string]any)
	command, _ := records[0]["command"].(string)
	declared := DeclaredHookCommand(command)
	if !strings.Contains(declared, "guard.ps1") || strings.Contains(declared, "guard.sh") {
		t.Fatalf("forced Windows output selected wrong command: %s", declared)
	}
}

func TestEmit_WindowsVariantRejectsUnsafeArguments(t *testing.T) {
	testutil.TempCwd(t)
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "pre_tool", "command": "python", "commandWindows": "powershell", "args": []any{"a'b"}}}
	if err := emitHooks(emit.NewSession(), []spec.Entry{hook}, ".vibe/hooks.toml", ".vibe/hooks", false, "windows"); err == nil {
		t.Fatal("selected Windows command ignored unsafe arguments")
	}
	if _, err := os.Stat(".vibe/hooks.toml"); !os.IsNotExist(err) {
		t.Fatal("failed Windows command wrote hooks")
	}
}

func TestEmit_GrepDoesNotAdvertiseRemovedRuntimeTool(t *testing.T) {
	testutil.TempCwd(t)
	agent := spec.Entry{Kind: spec.KindAgent, Name: "searcher", Meta: map[string]any{"tools": []any{"Grep"}}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{Agents: []spec.Entry{agent}}, &config.Config{}, false); err == nil {
		t.Fatal("search-only agent must fail instead of enabling a removed tool")
	}
	if _, err := os.Stat(".vibe/agents/searcher.toml"); !os.IsNotExist(err) {
		t.Fatalf("failed search-only agent wrote output: %v", err)
	}
	if names, supported := New().TranslateAgentCapability("Grep"); supported || len(names) != 0 {
		t.Errorf("removed Grep capability is still advertised: %v", names)
	}
	if _, supported := New().ToolCapabilities()["Grep"]; supported {
		t.Error("tool capability table still advertises the removed Grep tool")
	}
}

func TestEmit_GrepOmissionKeepsReadRestrictionAndReportsCoverage(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	agent := spec.Entry{Kind: spec.KindAgent, Name: "reader", Meta: map[string]any{"tools": []any{"Read", "Grep"}}}
	if err := New().Emit(emit.NewSession(), spec.Bundle{Agents: []spec.Entry{agent}}, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".vibe/agents/reader.toml")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `enabled_tools = ["read_file"]`) {
		t.Fatalf("search omission changed the remaining restriction: %s", data)
	}
	for _, forbidden := range []string{`"grep"`, `"bash"`} {
		if strings.Contains(string(data), forbidden) {
			t.Errorf("search omission emitted %s: %s", forbidden, data)
		}
	}
	for _, note := range emit.PendingCoverageNotes() {
		if note.Target == "vibe" && note.Kind == spec.KindAgent && strings.Contains(note.Message, "Grep") {
			return
		}
	}
	t.Fatal("omitted search capability has no coverage note")
}
