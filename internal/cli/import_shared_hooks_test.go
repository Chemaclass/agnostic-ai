package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// syncSharedHook writes a project with one shared hook spec for target
// and runs a real sync.
func syncSharedHook(t *testing.T, target, hook string) {
	t.Helper()
	testutil.TempCwd(t)
	syncSharedHookIn(t, target, hook)
}

// syncSharedHookIn writes the project into the working directory, for
// a test that adds files before sync. targets is a YAML flow list body.
func syncSharedHookIn(t *testing.T, targets, hook string) {
	t.Helper()
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	writeAgnosticFile(t, "# Shared\n")
	writeFile(t, filepath.Join(".agnostic-ai", "hooks", "sh.yaml"), hook)
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
}

// assertOnlySharedHook fails unless sh.yaml is the only shared hook
// spec, unchanged, and the note names it.
func assertOnlySharedHook(t *testing.T, hook, out string) {
	t.Helper()
	files := sharedHookFiles(t)
	if len(files) != 1 || files["sh.yaml"] != hook {
		t.Errorf("want only sh.yaml as written, got %v", files)
	}
	if !strings.Contains(out, "already sync: sh") {
		t.Errorf("want the note to name the shared hook:\n%s", out)
	}
}

func TestImport_SkipsHooksASharedSpecSyncs(t *testing.T) {
	hook := "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: echo shared\n"
	for _, target := range []string{"claude", "codex", "copilot", "crush", "factory", "gemini", "goose", "kiro", "openhands", "trae", "windsurf"} {
		t.Run(target, func(t *testing.T) {
			syncSharedHook(t, target, hook)

			assertOnlySharedHook(t, hook, importCapturing(t, target))
		})
	}
}

func TestImport_SkipsHooksAPortableSharedSpecSyncs(t *testing.T) {
	hook := "name: sh\non: before-tool\nmatch: shell\ncommand: echo shared\n"
	for _, target := range []string{"claude", "codex", "crush", "factory", "gemini", "goose", "openhands", "windsurf"} {
		t.Run(target, func(t *testing.T) {
			syncSharedHook(t, target, hook)

			assertOnlySharedHook(t, hook, importCapturing(t, target))
		})
	}
}

// A native handler no spec renders still imports, also when it shares a
// matcher group with a synced one.
func TestImport_KeepsNativeHooksNoSharedSpecSyncs(t *testing.T) {
	hook := "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: echo shared\n"
	cases := []struct{ target, path, native string }{
		{"claude", filepath.Join(".claude", "settings.json"),
			`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo shared"},{"type":"command","command":"echo native"}]}]}}`},
		{"codex", filepath.Join(".codex", "hooks.json"),
			`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"export AGNOSTIC_AI_TARGET=codex; echo shared"},{"type":"command","command":"echo native"}]}]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			syncSharedHook(t, tc.target, hook)
			writeFile(t, tc.path, tc.native)

			out := importCapturing(t, tc.target)

			files := sharedHookFiles(t)
			if len(files) != 2 || files["sh.yaml"] != hook {
				t.Fatalf("want sh.yaml and one imported hook, got %v", files)
			}
			for name, data := range files {
				if name != "sh.yaml" && (!strings.Contains(data, "echo native") || strings.Contains(data, "echo shared")) {
					t.Errorf("%s: want only the native handler:\n%s", name, data)
				}
			}
			if !strings.Contains(out, "already sync: sh") {
				t.Errorf("want the note to name the shared hook:\n%s", out)
			}
		})
	}
}

// A shared hook that runs a script under the hooks source reaches each
// target through its own hooks directory, and import still matches it.
func TestImport_SkipsSharedHooksThatRunAScript(t *testing.T) {
	hook := "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: sh .agnostic-ai/hooks/guard.sh\n"
	for _, target := range []string{"claude", "codex", "copilot", "crush", "factory", "gemini", "goose", "kiro", "openhands", "trae", "windsurf"} {
		t.Run(target, func(t *testing.T) {
			testutil.TempCwd(t)
			writeFile(t, filepath.Join(".agnostic-ai", "hooks", "guard.sh"), "#!/bin/sh\nexit 0\n")
			syncSharedHookIn(t, target, hook)

			out := importCapturing(t, target)

			files := sharedHookFiles(t)
			if len(files) != 2 || files["sh.yaml"] != hook || files["guard.sh"] != "#!/bin/sh\nexit 0\n" {
				t.Errorf("want sh.yaml and guard.sh as written, got %v", files)
			}
			if !strings.Contains(out, "already sync: sh") {
				t.Errorf("want the note to name the shared hook:\n%s", out)
			}
		})
	}
}

// A shared hook pinned to one target does not claim the same handler in
// another target's native file: sync never writes it there.
func TestImport_KeepsNativeHooksASharedSpecRendersOnlyElsewhere(t *testing.T) {
	testutil.TempCwd(t)
	hook := "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: echo shared\ntarget: claude\n"
	syncSharedHookIn(t, "claude, codex", hook)
	writeFile(t, filepath.Join(".codex", "hooks.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"export AGNOSTIC_AI_TARGET=codex; echo shared"}]}]}}`)

	out := importCapturing(t, "codex")

	files := sharedHookFiles(t)
	if len(files) != 2 || files["sh.yaml"] != hook {
		t.Fatalf("want sh.yaml and the imported codex hook, got %v", files)
	}
	if strings.Contains(out, "already sync") {
		t.Errorf("want no shared hook named as synced:\n%s", out)
	}
}

// Gemini keeps a matcher group's handlers in one definition, so import
// drops the synced handler and keeps the hand-added one.
func TestImport_KeepsNativeGeminiHandlersBesideASyncedOne(t *testing.T) {
	hook := "name: sh\nevent: BeforeTool\nmatcher: run_shell_command\ncommand: echo shared\n"
	syncSharedHook(t, "gemini", hook)
	path := filepath.Join(".gemini", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	groups := settings["hooks"].(map[string]any)["BeforeTool"].([]any)
	group := groups[0].(map[string]any)
	group["hooks"] = append(group["hooks"].([]any), map[string]any{"type": "command", "command": "echo native"})
	data, err = json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data))

	out := importCapturing(t, "gemini")

	files := sharedHookFiles(t)
	if len(files) != 2 || files["sh.yaml"] != hook {
		t.Fatalf("want sh.yaml and one imported hook, got %v", files)
	}
	for name, data := range files {
		if name != "sh.yaml" && (!strings.Contains(data, "echo native") || strings.Contains(data, "echo shared")) {
			t.Errorf("%s: want only the native handler:\n%s", name, data)
		}
	}
	if !strings.Contains(out, "already sync: sh") {
		t.Errorf("want the note to name the shared hook:\n%s", out)
	}
}

// A portable non-command hook left unpinned matches through the target
// being imported, not the spec's own target key.
func TestImport_SkipsAnUnpinnedPortableMCPToolHook(t *testing.T) {
	testutil.TempCwd(t)
	hook := "name: sh\non: before-tool\nmatch: shell\ntype: mcp_tool\nserver: guard\ntool: check\n"
	syncSharedHookIn(t, "claude, codex", hook)

	assertOnlySharedHook(t, hook, importCapturing(t, "claude"))
}

// Without the shared hook specs import cannot tell which native hooks
// sync wrote, so it stops rather than copy them all back.
func TestImport_FailsWhenTheSharedHooksDoNotLoad(t *testing.T) {
	syncSharedHook(t, "claude", "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: echo shared\n")
	writeFile(t, filepath.Join(".agnostic-ai", "hooks", "broken.yaml"), "name: [\n")

	out, err := runCLI(t, "import", "claude")
	if err == nil || !strings.Contains(err.Error()+out, "load shared hooks") {
		t.Fatalf("want a shared hook load error, got err=%v\n%s", err, out)
	}
}

// Exec-form args are part of a handler where the target writes them:
// some fold them into the command, Claude Code keeps them beside it, and
// the rest drop them (#1775).
func TestImport_SkipsSharedHooksWithArgs(t *testing.T) {
	hook := "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: echo\nargs: [shared, two words]\n"
	for _, target := range []string{"claude", "codex", "copilot", "crush", "factory", "gemini", "goose", "kiro", "openhands", "trae", "windsurf"} {
		t.Run(target, func(t *testing.T) {
			syncSharedHook(t, target, hook)

			assertOnlySharedHook(t, hook, importCapturing(t, target))
		})
	}
}

// Sync leaves a hook path in an arg as written, and import matches it.
func TestImport_SkipsSharedHooksWithHookPathsInArgs(t *testing.T) {
	hook := "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: cat\nargs: [.agnostic-ai/scripts/policy.json, .claude/hooks/policy.json]\n"
	for _, target := range []string{"claude", "codex", "copilot", "crush", "factory", "gemini", "goose", "kiro", "openhands", "trae", "windsurf"} {
		t.Run(target, func(t *testing.T) {
			syncSharedHook(t, target, hook)

			assertOnlySharedHook(t, hook, importCapturing(t, target))
		})
	}
}

func TestImport_KeepsANativeClaudeHookWithOtherArgs(t *testing.T) {
	hook := "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: echo\nargs: [shared]\n"
	syncSharedHook(t, "claude", hook)
	writeFile(t, filepath.Join(".claude", "settings.json"),
		`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"echo","args":["shared"]},{"type":"command","command":"echo","args":["native"]}]}]}}`)

	importCapturing(t, "claude")

	files := sharedHookFiles(t)
	if len(files) != 2 || files["sh.yaml"] != hook {
		t.Fatalf("want sh.yaml and the native hook, got %v", files)
	}
	for name, data := range files {
		if name != "sh.yaml" && !strings.Contains(data, "native") {
			t.Errorf("%s: want the native args:\n%s", name, data)
		}
	}
}

// Gemini emits an x-gemini handler group as written, without the
// spec's top-level args.
func TestImport_SkipsAGeminiHandlerGroupBesideTopLevelArgs(t *testing.T) {
	hook := "name: sh\nevent: BeforeTool\nmatcher: run_shell_command\nargs: [a]\nx-gemini:\n  hooks:\n    - type: command\n      command: echo one\n"
	syncSharedHook(t, "gemini", hook)

	assertOnlySharedHook(t, hook, importCapturing(t, "gemini"))
}

// Crush runs a synced script by its ./ path, and import reads that path
// back as the shared spec's, with or without args.
func TestImport_SkipsSharedCrushHooksThatRunASyncedScript(t *testing.T) {
	for name, hook := range map[string]string{
		"plain":  "name: sh\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/guard.sh\n",
		"args":   "name: sh\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/guard.sh\nargs: [--strict]\n",
		"quoted": "name: sh\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/prüfen.sh\nargs: [--strict]\n",
	} {
		t.Run(name, func(t *testing.T) {
			testutil.TempCwd(t)
			writeFile(t, filepath.Join(".agnostic-ai", "scripts", "guard.sh"), "#!/bin/sh\nexit 0\n")
			writeFile(t, filepath.Join(".agnostic-ai", "scripts", "prüfen.sh"), "#!/bin/sh\nexit 0\n")
			syncSharedHookIn(t, "crush", hook)

			assertOnlySharedHook(t, hook, importCapturing(t, "crush"))
		})
	}
}

// An `x-<target>` override of command and args applies to both, and
// import matches what sync wrote.
func TestImport_SkipsASharedHookWithATargetOverride(t *testing.T) {
	hook := "name: sh\nevent: PreToolUse\nmatcher: Execute\ncommand: echo\nargs: [base]\nx-factory:\n  command: printf\n  args: [target]\n"
	syncSharedHook(t, "factory", hook)
	data, err := os.ReadFile(filepath.Join(".factory", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"command": "printf 'target'"`) {
		t.Errorf("want the override's command and args:\n%s", data)
	}

	assertOnlySharedHook(t, hook, importCapturing(t, "factory"))
}

// A native hook that differs from a shared one only in the hooks
// directory its argument names is a different hook, so import keeps it.
func TestImport_KeepsANativeHookNamingAnotherHooksDirectory(t *testing.T) {
	hook := "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: cat\nargs: [.agnostic-ai/scripts/policy.json]\n"
	cases := []struct{ target, path, native string }{
		{"claude", filepath.Join(".claude", "settings.json"),
			`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"cat","args":[".agnostic-ai/scripts/policy.json"]},{"type":"command","command":"cat","args":[".claude/hooks/policy.json"]}]}]}}`},
		{"trae", filepath.Join(".trae", "hooks.json"),
			`{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"cat '.agnostic-ai/scripts/policy.json'"},{"type":"command","command":"cat '.claude/hooks/policy.json'"}]}]}}`},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			syncSharedHook(t, tc.target, hook)
			writeFile(t, tc.path, tc.native)

			importCapturing(t, tc.target)

			files := sharedHookFiles(t)
			if len(files) != 2 || files["sh.yaml"] != hook {
				t.Fatalf("want sh.yaml and the native hook, got %v", files)
			}
			for name, data := range files {
				if name != "sh.yaml" && !strings.Contains(data, ".claude/hooks/policy.json") {
					t.Errorf("%s: want the native hook:\n%s", name, data)
				}
			}
		})
	}
}

// Sync still writes a disabled hook, so import matches it.
func TestImport_SkipsADisabledSharedHook(t *testing.T) {
	hook := "name: sh\nevent: Stop\ncommand: echo done\ndisabled: true\n"
	for _, target := range []string{"kiro", "codex", "trae"} {
		t.Run(target, func(t *testing.T) {
			syncSharedHook(t, target, hook)

			assertOnlySharedHook(t, hook, importCapturing(t, target))
		})
	}
}

// A command that sets the target variable itself keeps it where sync
// sets the variable in the environment instead.
func TestImport_SkipsASharedHookThatExportsTheTargetItself(t *testing.T) {
	for _, target := range []string{"claude", "gemini", "codex"} {
		t.Run(target, func(t *testing.T) {
			hook := "name: sh\nevent: PreToolUse\ncommand: 'export AGNOSTIC_AI_TARGET=" + target + "; echo shared'\n"
			syncSharedHook(t, target, hook)

			assertOnlySharedHook(t, hook, importCapturing(t, target))
		})
	}
}

// Sync reads a hook's event and matcher after its `x-<target>` override,
// as it reads the command, so import matches the synced hook there and
// keeps a native one under the top-level event and matcher.
func TestImport_MatchesAnOverriddenEventAndMatcher(t *testing.T) {
	hook := "name: sh\nevent: BeforeTool\nmatcher: run_shell_command\ncommand: echo base\nx-gemini:\n  event: AfterTool\n  matcher: write_file\n  command: echo target\n"
	syncSharedHook(t, "gemini", hook)

	assertOnlySharedHook(t, hook, importCapturing(t, "gemini"))

	path := filepath.Join(".gemini", "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	hooks := settings["hooks"].(map[string]any)
	if _, ok := hooks["BeforeTool"]; ok {
		t.Fatalf("sync wrote the top-level event: %s", data)
	}
	hooks["BeforeTool"] = []any{map[string]any{"matcher": "run_shell_command", "hooks": []any{map[string]any{"type": "command", "command": "echo target"}}}}
	if data, err = json.Marshal(settings); err != nil {
		t.Fatal(err)
	}
	writeFile(t, path, string(data))

	importCapturing(t, "gemini")

	files := sharedHookFiles(t)
	if len(files) != 2 || files["sh.yaml"] != hook {
		t.Fatalf("want sh.yaml and the native hook, got %v", files)
	}
	for name, data := range files {
		if name != "sh.yaml" && (!strings.Contains(data, "BeforeTool") || !strings.Contains(data, "echo target")) {
			t.Errorf("%s: want the native BeforeTool hook:\n%s", name, data)
		}
	}
}

// A non-command hook moved to another event by its override leaves a
// native one under the original event to import.
func TestImport_KeepsANativeHookUnderTheEventAnOverrideLeft(t *testing.T) {
	hook := "name: sh\nevent: PreToolUse\nmatcher: Bash\ntype: http\nurl: https://example.com/hook\nx-claude:\n  event: PostToolUse\n"
	syncSharedHook(t, "claude", hook)
	writeFile(t, filepath.Join(".claude", "settings.json"),
		`{"hooks":{"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"http","url":"https://example.com/hook"}]}],"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"http","url":"https://example.com/hook"}]}]}}`)

	importCapturing(t, "claude")

	files := sharedHookFiles(t)
	if len(files) != 2 || files["sh.yaml"] != hook {
		t.Fatalf("want sh.yaml and the native PreToolUse hook, got %v", files)
	}
}
