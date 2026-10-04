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
