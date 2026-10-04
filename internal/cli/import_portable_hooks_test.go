package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Import writes on: and match: where the portable form gives the target
// the same native event and matcher, and the native names elsewhere.
// Either way, sync then writes the native file back byte for byte.
func TestImport_WritesPortableHooksThatSyncBackUnchanged(t *testing.T) {
	cases := []struct {
		name, target, native string
		want                 []string
	}{
		{"claude", "claude", "event: PreToolUse\nmatcher: Bash\n", []string{"on: before-tool\n", "match: shell\n"}},
		{"claude wider edit", "claude", "event: PostToolUse\nmatcher: Edit|Write\n", []string{"event: PostToolUse\n", "matcher: Edit|Write\n"}},
		{"codex", "codex", "event: PreToolUse\nmatcher: Bash\n", []string{"on: before-tool\n", "match: shell\n"}},
		{"codex stop", "codex", "event: Stop\n", []string{"on: stop\n"}},
		{"copilot", "copilot", "event: SessionStart\n", []string{"on: session-start\n"}},
		{"crush", "crush", "event: PreToolUse\nmatcher: ^bash$\n", []string{"on: before-tool\n", "match: shell\n"}},
		{"factory", "factory", "event: PreToolUse\nmatcher: ^Execute$\n", []string{"on: before-tool\n", "match: shell\n"}},
		{"gemini", "gemini", "event: BeforeTool\nmatcher: ^run_shell_command$\n", []string{"on: before-tool\n", "match: shell\n"}},
		{"goose", "goose", "event: PreToolUse\nmatcher: ^shell$\n", []string{"on: before-tool\n", "match: shell\n"}},
		{"openhands", "openhands", "event: PreToolUse\nmatcher: terminal\n", []string{"on: before-tool\n", "match: shell\n"}},
		{"trae", "trae", "event: PreToolUse\nmatcher: Bash\n", []string{"event: PreToolUse\n", "matcher: Bash\n"}},
		{"windsurf", "windsurf", "event: PreToolUse\nmatcher: ^exec$\n", []string{"on: before-tool\n", "match: shell\n"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			syncSharedHook(t, tc.target, "name: sh\n"+tc.native+"command: echo shared\n")
			synced := snapshotEmitted(t, ".")
			if err := os.Remove(filepath.Join(".agnostic-ai", "hooks", "sh.yaml")); err != nil {
				t.Fatal(err)
			}

			importCapturing(t, tc.target)

			files := sharedHookFiles(t)
			if len(files) != 1 {
				t.Fatalf("want one imported hook, got %v", files)
			}
			for name, body := range files {
				for _, want := range tc.want {
					if !strings.Contains(body, want) {
						t.Errorf("%s misses %q:\n%s", name, want, body)
					}
				}
			}
			mustSync(t)
			resynced := snapshotEmitted(t, ".")
			for path, before := range synced {
				if resynced[path] != before {
					t.Errorf("%s changed after import and sync:\nbefore:\n%s\nafter:\n%s", path, before, resynced[path])
				}
			}
			if out, err := runCLI(t, "sync", "--check", "--gitignore=off"); err != nil {
				t.Errorf("sync --check: %v\n%s", err, out)
			}
		})
	}
}

// A Claude Code hook Codex runs as written imports without a target pin,
// so the portable form must give both tools the same native hook.
func TestImport_UnpinnedClaudeHookIsPortableOnClaudeAndCodex(t *testing.T) {
	syncSharedHook(t, "claude, codex", "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: echo shared\n")
	synced := snapshotEmitted(t, ".")
	if err := os.Remove(filepath.Join(".agnostic-ai", "hooks", "sh.yaml")); err != nil {
		t.Fatal(err)
	}

	importCapturing(t, "claude")

	files := sharedHookFiles(t)
	for name, body := range files {
		if !strings.Contains(body, "on: before-tool\n") || !strings.Contains(body, "match: shell\n") || strings.Contains(body, "target:") {
			t.Errorf("%s: want an unpinned portable hook:\n%s", name, body)
		}
	}
	if len(files) != 1 {
		t.Fatalf("want one imported hook, got %v", files)
	}
	mustSync(t)
	resynced := snapshotEmitted(t, ".")
	for _, path := range []string{".claude/settings.json", ".codex/hooks.json"} {
		if synced[path] == "" || resynced[path] != synced[path] {
			t.Errorf("%s changed after import and sync:\nbefore:\n%s\nafter:\n%s", path, synced[path], resynced[path])
		}
	}
}

// A re-import leaves a native spec an older import wrote as it is: the
// spec already syncs the hook, so import skips it and needs no
// --overwrite. LINT033 offers the rewrite instead.
func TestImport_KeepsAnIdenticalNativeSpecOnReimport(t *testing.T) {
	syncSharedHook(t, "claude", "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: echo shared\n")
	if err := os.Remove(filepath.Join(".agnostic-ai", "hooks", "sh.yaml")); err != nil {
		t.Fatal(err)
	}
	importCapturing(t, "claude")
	files := sharedHookFiles(t)
	if len(files) != 1 {
		t.Fatalf("want one imported hook, got %v", files)
	}
	var name, native string
	for n, body := range files {
		name = n
		native = strings.NewReplacer("on: before-tool", "event: PreToolUse", "match: shell", "matcher: Bash").Replace(body)
	}
	mustWrite(t, filepath.Join(".agnostic-ai", "hooks", name), native)

	importCapturing(t, "claude")

	if got := sharedHookFiles(t); len(got) != 1 || got[name] != native {
		t.Errorf("want %s kept native as written, got %v", name, got)
	}
}
