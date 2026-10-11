package opencode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_HookV2RegistersNativeGuard(t *testing.T) {
	emitHookSpec(t, spec.Entry{
		Kind: spec.KindHook, Name: "guard-shell",
		Meta: map[string]any{
			"event": "PreToolUse", "matcher": "shell", "command": "echo guard",
			"x-opencode": map[string]any{"plugin-api": "v2"},
		},
	})
	got := readFile(t, ".opencode/plugins/guard-shell.ts")
	for _, want := range []string{
		`import type { Plugin } from "@opencode/plugin"`,
		`id: "agnostic-ai.guard-shell"`,
		`ctx.tool.hook("execute.before"`,
		`export default plugin`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("native V2 guard missing %q in:\n%s", want, got)
		}
	}
}

func TestEmit_HookV2RejectsUnsupportedSelectionAndEvents(t *testing.T) {
	for _, tt := range []struct {
		name, selector, event, want string
	}{
		{"unknown API", "v3", "PreToolUse", "plugin-api"},
		{"bus event", "v2", "session.idle", "session.idle"},
		{"mutation event", "v2", "shell.env", "shell.env"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testutil.TempCwd(t)
			e := spec.Entry{
				Kind: spec.KindHook, Name: "guard", Path: ".agnostic-ai/hooks/guard.yaml",
				Meta: map[string]any{
					"event": tt.event, "command": "echo guard",
					"x-opencode": map[string]any{"plugin-api": tt.selector},
				},
			}
			err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{e}), &config.Config{}, false)
			if err == nil || !strings.Contains(err.Error(), e.Path) || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected path-specific %s rejection, got %v", tt.want, err)
			}
		})
	}
}

func TestEmit_HookV2RuntimeFixtures(t *testing.T) {
	root := testutil.TempCwd(t)
	output := os.Getenv("AGNOSTIC_AI_OPENCODE_FIXTURE_DIR")
	if output == "" {
		output = filepath.Join(root, "native-fixtures")
	}
	if !filepath.IsAbs(output) {
		t.Fatalf("fixture output must be absolute: %s", output)
	}
	if err := os.MkdirAll(output, 0o755); err != nil {
		t.Fatal(err)
	}
	node := os.Getenv("AGNOSTIC_AI_OPENCODE_NODE")
	if node == "" {
		node = "node"
	}
	child := `const fs = require("node:fs"); const cp = require("node:child_process");
let raw = ""; process.stdin.setEncoding("utf8"); process.stdin.on("data", x => raw += x);
process.stdin.on("end", () => {
 const event = JSON.parse(raw); const input = event.input;
 const record = { marker: process.argv[1], argv: process.argv.slice(2), raw, cwd: process.cwd(), target: process.env.AGNOSTIC_AI_TARGET, custom: process.env.HOOK_TEST_ENV, inherited: process.env.HOOK_INHERITED_ENV };
 fs.appendFileSync(input.file, JSON.stringify(record) + "\n");
 if (input.mode === "block") { console.error("guard refused operation"); process.exit(2); }
 if (input.mode === "failure") { console.error("ordinary hook failure"); process.exit(7); }
 if (["tree", "block-orphan"].includes(input.mode)) {
  const descendant = cp.spawn(process.execPath, ["-e", "const fs=require('node:fs');setInterval(()=>fs.writeFileSync(process.argv[1],String(Date.now())),40)", input.heartbeat], { stdio: "inherit" });
  fs.writeFileSync(input.pid, String(descendant.pid));
  if (input.mode === "block-orphan") { console.error("guard refused operation with held stderr"); process.exit(2); }
  setInterval(() => {}, 1000);
 }
});`
	args := []any{"-e", child, "first", "two words", "", "'quote\"", "a;b", "unicode-ñ", "space and trailing\\", "two\\\\"}
	cases := []struct {
		name, event, api, matcher string
		command                   any
		args                      []any
		timeout                   int
	}{
		{"guard", "PreToolUse", "v2", "shell", []any{node, node}, args, 0},
		{"timeout", "PreToolUse", "v2", "shell", node, args, 1},
		{"post", "PostToolUse", "v2", "shell", node, args, 0},
		{"after", "tool.execute.after", "v2", "shell", node, args, 0},
		{"spawn-failure", "PreToolUse", "v2", "shell", filepath.Join(root, "missing-program"), []any{}, 0},
		{"parse-failure", "PreToolUse", "v2", "shell", "if then", nil, 0},
		{"bad-matcher", "PreToolUse", "v2", "[", "echo unused", nil, 0},
		{"v1", "PreToolUse", "v1", "bash", "exit 2", nil, 0},
		{"shell", "PreToolUse", "v2", "shell", "printf shell-ran", nil, 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			meta := map[string]any{"event": tt.event, "matcher": tt.matcher, "command": tt.command, "x-opencode": map[string]any{"plugin-api": tt.api, "env": map[string]any{"HOOK_TEST_ENV": "per-hook"}}}
			if tt.args != nil {
				meta["args"] = tt.args
			}
			if tt.timeout > 0 {
				meta["timeout"] = tt.timeout
			}
			e := spec.Entry{Kind: spec.KindHook, Name: tt.name, Meta: meta}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{e}), &config.Config{}, false); err != nil {
				t.Fatalf("emit: %v", err)
			}
			body, err := os.ReadFile(filepath.Join(".opencode/plugins", tt.name+".ts"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(output, tt.name+".ts"), body, 0o644); err != nil {
				t.Fatal(err)
			}

		})
	}
}

func TestEmit_HookV1SelectionKeepsDefaultBytes(t *testing.T) {
	testutil.TempCwd(t)
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "PreToolUse", "matcher": "bash", "command": "echo guard"}}
	if err := emitHooks(emit.NewSession(), []spec.Entry{hook}, "default", false); err != nil {
		t.Fatal(err)
	}
	hook.Meta["x-opencode"] = map[string]any{"plugin-api": "v1"}
	if err := emitHooks(emit.NewSession(), []spec.Entry{hook}, "explicit", false); err != nil {
		t.Fatal(err)
	}
	if want, got := readFile(t, "default/guard.ts"), readFile(t, "explicit/guard.ts"); want != got {
		t.Errorf("explicit V1 changed default bytes:\n%s", got)
	}
}

func TestEmit_HookV2MaterializesExecScriptAtOverriddenPluginPath(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\nprintf guard\n"
	if err := os.WriteFile(".agnostic-ai/scripts/guard with spaces.sh", []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
		"event": "PreToolUse", "command": ".agnostic-ai/scripts/guard with spaces.sh", "args": []any{"two words", ""},
		"x-opencode": map[string]any{"plugin-api": "v2"},
	}}
	cfg := &config.Config{Outputs: map[string]config.Output{"opencode": {HooksDir: "custom/plugins"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{hook}), cfg, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, ".opencode/hooks/guard with spaces.sh"); got != body {
		t.Errorf("materialized script = %q, want %q", got, body)
	}
	module := readFile(t, "custom/plugins/guard.ts")
	if !strings.Contains(module, `"command":".opencode/hooks/guard with spaces.sh","args":["two words",""]`) {
		t.Errorf("exec command or argument boundaries changed:\n%s", module)
	}
	if _, err := os.Stat(".opencode/plugins/guard.ts"); !os.IsNotExist(err) {
		t.Errorf("default plugin output exists: %v", err)
	}
}

func TestEmit_HookV2RefusesInvalidBatchBeforeWritingPlugins(t *testing.T) {
	testutil.TempCwd(t)
	hooks := []spec.Entry{
		{Kind: spec.KindHook, Name: "legacy", Meta: map[string]any{"event": "PreToolUse", "command": "echo guard"}},
		{Kind: spec.KindHook, Name: "unsupported", Path: "hooks/unsupported.yaml", Meta: map[string]any{"event": "session.idle", "command": "echo guard", "x-opencode": map[string]any{"plugin-api": "v2"}}},
	}
	if err := emitHooks(emit.NewSession(), hooks, ".opencode/plugins", false); err == nil {
		t.Fatal("unsupported V2 event accepted")
	}
	if _, err := os.Stat(".opencode/plugins"); !os.IsNotExist(err) {
		t.Errorf("invalid hook batch wrote plugins: %v", err)
	}
}

func TestEmit_HookV2InvalidMixedBundlePreservesNativeFiles(t *testing.T) {
	testutil.TempCwd(t)
	existing := map[string]string{
		legacyEntryPointFile:             emit.WithHeader("# existing legacy instructions\n", emit.FormatMarkdown),
		"AGENTS.md":                      "existing root instructions\n",
		".opencode/agents/existing.md":   "existing agent bytes\n",
		".opencode/commands/existing.md": "existing command bytes\n",
		"opencode.json":                  "{\"custom\":true}\n",
	}
	for path, body := range existing {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	invalid := spec.Entry{Kind: spec.KindHook, Name: "invalid", Path: ".agnostic-ai/hooks/invalid.yaml", Meta: map[string]any{
		"event": "PreToolUse", "command": "node", "args": []any{"valid", "broken\x00argument"},
		"x-opencode": map[string]any{"plugin-api": "v2"},
	}}
	bundle := spec.NewBundle([]spec.Entry{
		{Kind: spec.KindAgent, Name: "existing", Body: "replacement agent"},
		{Kind: spec.KindAgent, Name: "fresh", Body: "fresh agent"},
		{Kind: spec.KindCommand, Name: "existing", Body: "replacement command"},
		{Kind: spec.KindCommand, Name: "fresh", Body: "fresh command"},
		invalid,
	})
	err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false)
	if err == nil || !strings.Contains(err.Error(), invalid.Path) {
		t.Errorf("expected source-specific V2 preflight failure, got %v", err)
	}
	for path, expected := range existing {
		got, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("preflight removed existing native file %s: %v", path, err)
			continue
		}
		if string(got) != expected {
			t.Errorf("preflight changed native file %s: got %q, want %q", path, got, expected)
		}
	}
	for _, path := range []string{".opencode/agents/fresh.md", ".opencode/commands/fresh.md", ".opencode/plugins"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("invalid V2 bundle wrote fresh native output %s: %v", path, err)
		}
	}
}

func TestEmit_HookV2RejectsNULProcessFieldsBeforeWritingPlugins(t *testing.T) {
	for _, tt := range []struct {
		name, command string
		args          []any
	}{
		{"command", "node\x00ignored-executable", []any{"valid"}},
		{"first argument", "node", []any{"first\x00truncated", "later argument"}},
		{"later argument", "node", []any{"valid first argument", "later\x00truncated", "final argument"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testutil.TempCwd(t)
			hook := spec.Entry{Kind: spec.KindHook, Name: "invalid", Path: ".agnostic-ai/hooks/invalid.yaml", Meta: map[string]any{
				"event": "PreToolUse", "command": tt.command, "args": tt.args,
				"x-opencode": map[string]any{"plugin-api": "v2"},
			}}
			valid := spec.Entry{Kind: spec.KindHook, Name: "valid-first", Meta: map[string]any{
				"event": "PreToolUse", "command": "echo guard", "x-opencode": map[string]any{"plugin-api": "v2"},
			}}
			err := emitHooks(emit.NewSession(), []spec.Entry{valid, hook}, ".opencode/plugins", false)
			if err == nil || !strings.Contains(err.Error(), hook.Path) || !strings.Contains(err.Error(), "NUL") {
				t.Errorf("expected source-specific NUL %s refusal, got %v", tt.name, err)
			}
			if _, err := os.Stat(".opencode/plugins"); !os.IsNotExist(err) {
				t.Errorf("invalid process field wrote a plugin or Windows helper: %v", err)
			}
		})
	}
}

func TestEmit_HookV2RejectsInvalidEnvironmentNamesBeforeWritingPlugins(t *testing.T) {
	for _, key := range []string{"", "INVALID=NAME"} {
		t.Run(key, func(t *testing.T) {
			testutil.TempCwd(t)
			hook := spec.Entry{Kind: spec.KindHook, Name: "invalid-env-name", Path: ".agnostic-ai/hooks/invalid-env-name.yaml", Meta: map[string]any{
				"event": "PreToolUse", "command": "echo guard",
				"x-opencode": map[string]any{"plugin-api": "v2", "env": map[string]any{key: "value"}},
			}}
			err := emitHooks(emit.NewSession(), []spec.Entry{hook}, ".opencode/plugins", false)
			if err == nil || !strings.Contains(err.Error(), hook.Path) || !strings.Contains(err.Error(), "environment name") {
				t.Errorf("expected source-specific invalid environment name refusal, got %v", err)
			}
			if _, err := os.Stat(".opencode/plugins"); !os.IsNotExist(err) {
				t.Errorf("invalid environment name wrote a plugin or Windows helper: %v", err)
			}
		})
	}
}

func TestEmit_HookV2RejectsNULEnvironmentBeforeWritingPlugins(t *testing.T) {
	for _, tt := range []struct {
		name string
		env  map[string]any
	}{
		{"environment name", map[string]any{"VALID": "first", "BROKEN\x00NAME": "value"}},
		{"environment value", map[string]any{"VALID": "first", "BROKEN": "value\x00truncated"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testutil.TempCwd(t)
			hook := spec.Entry{Kind: spec.KindHook, Name: "invalid-env", Path: ".agnostic-ai/hooks/invalid-env.yaml", Meta: map[string]any{
				"event": "PreToolUse", "command": "echo guard",
				"x-opencode": map[string]any{"plugin-api": "v2", "env": tt.env},
			}}
			valid := spec.Entry{Kind: spec.KindHook, Name: "valid-first", Meta: map[string]any{
				"event": "PreToolUse", "command": "echo guard", "x-opencode": map[string]any{"plugin-api": "v2"},
			}}
			err := emitHooks(emit.NewSession(), []spec.Entry{valid, hook}, ".opencode/plugins", false)
			if err == nil || !strings.Contains(err.Error(), hook.Path) || !strings.Contains(err.Error(), "NUL") {
				t.Errorf("expected source-specific NUL %s refusal, got %v", tt.name, err)
			}
			if _, err := os.Stat(".opencode/plugins"); !os.IsNotExist(err) {
				t.Errorf("invalid environment wrote a plugin or Windows helper: %v", err)
			}
		})
	}
}

func TestEmit_HookV2RejectsMalformedProcessOptions(t *testing.T) {
	for _, tt := range []struct {
		name, key string
		value     any
	}{
		{"fractional timeout", "timeout", 1.5},
		{"invalid timeout", "timeout", "5garbage"},
		{"negative timeout", "timeout", -1},
		{"invalid argument", "args", []any{"valid", 4}},
		{"invalid environment", "env", map[string]any{"TEST": 4}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			testutil.TempCwd(t)
			override := map[string]any{"plugin-api": "v2", tt.key: tt.value}
			hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Path: "hooks/guard.yaml", Meta: map[string]any{"event": "PreToolUse", "command": "echo guard", "x-opencode": override}}
			if err := emitHooks(emit.NewSession(), []spec.Entry{hook}, ".opencode/plugins", false); err == nil || !strings.Contains(err.Error(), hook.Path) {
				t.Errorf("malformed %s accepted or missing path: %v", tt.key, err)
			}
		})
	}
}

func TestEmit_HookV2MaterializesLiteralExecScriptWithEmptyArgs(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\nprintf guard\n"
	if err := os.WriteFile(".agnostic-ai/scripts/guard with spaces.sh", []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	hook := spec.Entry{Kind: spec.KindHook, Name: "literal-empty-args", Path: "hooks/literal-empty-args.yaml", Meta: map[string]any{
		"event": "PreToolUse", "command": ".agnostic-ai/scripts/guard with spaces.sh", "args": []any{},
		"x-opencode": map[string]any{"plugin-api": "v2"},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{}, false); err != nil {
		t.Fatalf("literal exec script must be materialized: %v", err)
	}
	if got := readFile(t, ".opencode/hooks/guard with spaces.sh"); got != body {
		t.Errorf("materialized script = %q, want %q", got, body)
	}
	module := readFile(t, ".opencode/plugins/literal-empty-args.ts")
	if !strings.Contains(module, `"command":".opencode/hooks/guard with spaces.sh","args":[]`) {
		t.Errorf("empty args lost literal exec command:\n%s", module)
	}
}

func TestEmit_HookV2WritesOnlyNativePlugin(t *testing.T) {
	emitHookSpec(t, spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
		"event": "PreToolUse", "command": "echo guard", "x-opencode": map[string]any{"plugin-api": "v2"},
	}})
	files, err := os.ReadDir(".opencode/plugins")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Name() != "guard.ts" {
		t.Errorf("expected only the native plugin, got %v", files)
	}
}
