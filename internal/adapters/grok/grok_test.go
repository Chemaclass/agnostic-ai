package grok

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func kitSinkBundle() spec.Bundle {
	return spec.NewBundle([]spec.Entry{
		{Kind: spec.KindRule, Name: "style", Body: "Keep functions short."},
		{Kind: spec.KindRule, Name: "database", Scope: "backend", Body: "Use transactions."},
		{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"description": "Review changes", "can": []any{"read"}, "effort": "high", "maxTurns": 8, "background": true}, Body: "Find defects."},
		{Kind: spec.KindSkill, Name: "review", Meta: map[string]any{"description": "Review work", "disable-model-invocation": true}, Body: "Read the diff."},
		{Kind: spec.KindCommand, Name: "check", Meta: map[string]any{"description": "Check work"}, Body: "Check $ARGUMENTS."},
		{Kind: spec.KindMCP, Name: "local", Meta: map[string]any{"command": "server", "args": []any{"--stdio"}, "cwd": "backend", "disabled": true}},
		{Kind: spec.KindMCP, Name: "remote", Meta: map[string]any{"type": "sse", "url": "https://example.com/sse", "headers": map[string]any{"Authorization": "${TOKEN}"}, "startup_timeout_sec": 30, "tool_timeout_sec": 90}},
		{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "command": "echo ok", "args": []any{"a b"}, "timeout": 7}},
		{Kind: spec.KindSettings, Name: "permissions", Meta: map[string]any{"permissions": map[string]any{"deny": []any{"Bash(git push *)"}}}},
	})
}
func visibleConfig() *config.Config {
	return &config.Config{Gitignore: config.Gitignore{Enabled: true, Commit: []string{"grok:instructions"}}}
}
func TestEmit_RequiresVisibleInstructionsBeforeAnyWrite(t *testing.T) {
	dir := testutil.TempCwd(t)
	err := New().Emit(emit.NewSession(), kitSinkBundle(), &config.Config{Gitignore: config.Gitignore{Enabled: true}}, false)
	if err == nil || !strings.Contains(err.Error(), "grok:instructions") {
		t.Fatalf("got %v", err)
	}
	if len(testutil.WalkRel(t, dir)) != 0 {
		t.Fatal("wrote output before validation")
	}
}
func TestEmit_RejectsUserIgnoredRules(t *testing.T) {
	testutil.TempCwd(t)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %s: %v", out, err)
	}
	if err := os.WriteFile(".gitignore", []byte(".grok/\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := New().Emit(emit.NewSession(), kitSinkBundle(), visibleConfig(), false)
	if err == nil || !strings.Contains(err.Error(), "ignored") {
		t.Fatalf("got %v", err)
	}
	if _, err := os.Stat(".grok"); !os.IsNotExist(err) {
		t.Fatal("wrote ignored files")
	}
}
func TestEmit_WritesNativeSurfaces(t *testing.T) {
	testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), visibleConfig(), false); err != nil {
		t.Fatal(err)
	}
	cases := map[string][]string{
		".grok/rules/style.md": {"Keep functions short."}, "backend/.grok/rules/database.md": {"Use transactions."}, ".grok/agents/reviewer.md": {"tools:", "Read", "effort: high", "maxTurns: 8"}, ".grok/skills/review/SKILL.md": {"disable-model-invocation: true", "Read the diff."}, ".grok/commands/check.md": {"$ARGUMENTS"}, ".grok/hooks/guard.json": {"PreToolUse", "a b", "AGNOSTIC_AI_TARGET", "7"}, ".grok/config.toml": {"enabled = false", "headers", "startup_timeout_sec = 30", "tool_timeout_sec = 90", "Bash(git push *)"}}
	for path, wants := range cases {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range wants {
			if !strings.Contains(string(data), want) {
				t.Errorf("%s missing %q: %s", path, want, data)
			}
		}
	}
	if _, err := os.Stat("AGENTS.md"); !os.IsNotExist(err) {
		t.Fatal("duplicate AGENTS.md")
	}
}
func TestEmit_ValidatesNamesAndEffortBeforeWriting(t *testing.T) {
	for _, e := range []spec.Entry{{Kind: spec.KindCommand, Name: "../escape", Body: "bad"}, {Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"effort": "minimal"}}, {Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"can": []any{}}}} {
		t.Run(string(e.Kind)+e.Name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{e}), visibleConfig(), true)
			if err == nil {
				t.Fatal("accepted invalid spec")
			}
			if len(testutil.WalkRel(t, dir)) > 0 {
				t.Fatal("wrote invalid spec")
			}
		})
	}
}
func TestEmit_PreservesUnrelatedTOML(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".grok", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".grok/config.toml", []byte("[plugins]\nenabled = [\"manual\"]\n[mcp_servers.manual]\ncommand = \"keep\"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), visibleConfig(), false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".grok/config.toml")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"manual", "keep", "plugins"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("missing %s", want)
		}
	}
}
func TestKitSink_GoldenSnapshot(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(cwd, "testdata", "kitsink")
	dir := testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), visibleConfig(), false); err != nil {
		t.Fatal(err)
	}
	testutil.AssertGoldenTree(t, dir, expected)
}

func TestEmit_RejectsIgnoredCustomRuleAndGlobalIgnore(t *testing.T) {
	for _, global := range []bool{false, true} {
		t.Run(fmt.Sprint(global), func(t *testing.T) {
			testutil.TempCwd(t)
			if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
				t.Fatalf("git: %v %s", err, out)
			}
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			cfg := visibleConfig()
			cfg.Outputs = map[string]config.Output{"grok": {RulesDir: "custom/rules"}}
			ignore := ".gitignore"
			if global {
				ignore = filepath.Join(home, ".gitignore")
			}
			if err := os.WriteFile(ignore, []byte("custom/\n"), 0600); err != nil {
				t.Fatal(err)
			}
			err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, true)
			if err == nil || !strings.Contains(err.Error(), "ignored") {
				t.Fatalf("got %v", err)
			}
			if _, err := os.Stat("custom"); !os.IsNotExist(err) {
				t.Fatal("dry run wrote custom path")
			}
		})
	}
}
func TestEmit_ManagedIgnoreCanReleaseRules(t *testing.T) {
	testutil.TempCwd(t)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git: %v %s", err, out)
	}
	before := "# >>> agnostic-ai (managed) >>>\n/.grok/\n# <<< agnostic-ai (managed) <<<\n"
	if err := os.WriteFile(".gitignore", []byte(before), 0600); err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), visibleConfig(), false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != before {
		t.Fatal("adapter changed ignore policy")
	}
}
func TestEmit_CustomPathsMoveEverySurface(t *testing.T) {
	testutil.TempCwd(t)
	cfg := visibleConfig()
	cfg.Outputs = map[string]config.Output{"grok": {Dir: "native", RulesDir: "rules", AgentsDir: "agents", SkillsDir: "skills", CommandsDir: "commands", HooksDir: "hooks", MCPFile: "mcp.toml", ConfFile: "permissions.toml"}}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), cfg, false); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"rules/style.md", "backend/rules/database.md", "agents/reviewer.md", "skills/review/SKILL.md", "commands/check.md", "hooks/guard.json", "mcp.toml", "permissions.toml"} {
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			t.Errorf("%s: %v", path, err)
		}
	}
}
func TestEmit_VisibilityUsesOnlyNativeIgnoreSources(t *testing.T) {
	testutil.TempCwd(t)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git: %v %s", err, out)
	}
	if err := os.MkdirAll("backend", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("backend/.gitignore", []byte(".grok/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".git/info/exclude", []byte(".grok/\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := New().Emit(emit.NewSession(), kitSinkBundle(), visibleConfig(), false); err != nil {
		t.Fatalf("non-native ignore channel rejected: %v", err)
	}
}
func TestEmit_CustomRootHookCommandReachesCopiedScript(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".agnostic-ai/scripts", 0755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\necho safe\n"
	if err := os.WriteFile(".agnostic-ai/scripts/check.sh", []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	cfg := visibleConfig()
	cfg.Outputs = map[string]config.Output{"grok": {Dir: "native"}}
	b := spec.NewBundle([]spec.Entry{{Kind: spec.KindHook, Name: "check", Meta: map[string]any{"event": "PreToolUse", "command": ".agnostic-ai/scripts/check.sh"}}})
	if err := New().Emit(emit.NewSession(), b, cfg, false); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("native/hooks/check.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "native/hook-scripts/check.sh") {
		t.Errorf("wrong script path %s", data)
	}
	body, err := os.ReadFile("native/hook-scripts/check.sh")
	if err != nil || string(body) != script {
		t.Fatalf("script: %q %v", body, err)
	}
}
func TestEmit_AgentRestrictionsNeverRestoreAllTools(t *testing.T) {
	for _, tools := range []any{[]any{"Read", "typo"}, []any{"run_terminal_command"}} {
		testutil.TempCwd(t)
		entry := spec.Entry{Kind: spec.KindAgent, Name: "restricted", Meta: map[string]any{"tools": tools}}
		if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), visibleConfig(), true); err == nil {
			t.Fatalf("unsafe native tools accepted %v", tools)
		}
	}
	testutil.TempCwd(t)
	entry := spec.Entry{Kind: spec.KindAgent, Name: "restricted", Meta: map[string]any{"can": []any{"read", "web"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), visibleConfig(), false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(".grok/agents/restricted.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Read", "WebFetch", "WebSearch", "search_tool", "use_tool"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("restriction missing %s: %s", want, got)
		}
	}
}

func TestEmit_AgentReadAliasesKeepTheSameBridgeRestrictions(t *testing.T) {
	var first string
	for _, entry := range []spec.Entry{
		{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"can": []any{"read"}}},
		{Kind: spec.KindAgent, Name: "reviewer", CapabilityField: "can", Meta: map[string]any{"tools": []any{"Read"}}},
		{Kind: spec.KindAgent, Name: "reviewer", Meta: map[string]any{"tools": []any{"Read"}}},
	} {
		testutil.TempCwd(t)
		if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), visibleConfig(), false); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(".grok/agents/reviewer.md")
		if err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = string(data)
		}
		if string(data) != first || !strings.Contains(string(data), "disallowedTools:\n  - search_tool\n  - use_tool") {
			t.Error("equivalent read aliases must emit the same MCP bridge restrictions")
		}
	}
}

func TestAgentMeta_PreservesExplicitNativeBridgeChoices(t *testing.T) {
	for _, tc := range []struct {
		name  string
		meta  map[string]any
		tools []string
		deny  []string
	}{
		{"one bridge", map[string]any{"tools": []any{"Read", "search_tool"}}, []string{"Read", "search_tool"}, []string{"use_tool"}},
		{"native override", map[string]any{"can": []any{"read"}, "x-grok": map[string]any{"tools": []any{"Read", "search_tool", "use_tool"}, "disallowedTools": []any{"grep"}}}, []string{"Read", "search_tool", "use_tool"}, []string{"grep"}},
		{"native only", map[string]any{"x-grok": map[string]any{"tools": []any{"Read"}}}, []string{"Read"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			meta, err := agentMeta(spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: tc.meta})
			if err != nil {
				t.Fatal(err)
			}
			if got := stringList(meta["disallowedTools"]); !slices.Equal(got, tc.deny) {
				t.Errorf("explicit native bridge choice changed: got %v, want %v", got, tc.deny)
			}
			if got := stringList(meta["tools"]); !slices.Equal(got, tc.tools) {
				t.Errorf("explicit native tools changed: got %v, want %v", got, tc.tools)
			}
		})
	}
}
