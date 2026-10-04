package cli

import (
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
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\n")
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
	for _, target := range []string{"claude", "codex", "crush", "factory", "gemini", "goose", "openhands"} {
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
