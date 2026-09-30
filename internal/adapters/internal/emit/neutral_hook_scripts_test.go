package emit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestRewriteNeutralHookPath_PreservesShellAndIgnoresOtherDirectories(t *testing.T) {
	cases := []struct{ command, want string }{
		{".agnostic-ai/scripts/guard.sh", ".codex/hooks/guard.sh"},
		{"sh .agnostic-ai/scripts/guard.sh --strict", "sh .codex/hooks/guard.sh --strict"},
		{"sh '.agnostic-ai/scripts/guard.sh'", "sh '.codex/hooks/guard.sh'"},
		{`node "$(git rev-parse --show-toplevel)/.agnostic-ai/scripts/guard.mjs"`, `node "$(git rev-parse --show-toplevel)/.codex/hooks/guard.mjs"`},
		{".agnostic-ai/scripts/pre.sh && .agnostic-ai/scripts/post.sh", ".codex/hooks/pre.sh && .codex/hooks/post.sh"},
		{".agnostic-ai/scripts/nested/.agnostic-ai/scripts/guard.sh", ".codex/hooks/nested/.agnostic-ai/scripts/guard.sh"},
		{"other.agnostic-ai/scripts/guard.sh", "other.agnostic-ai/scripts/guard.sh"},
		{"sh backup/.agnostic-ai/scripts/guard.sh", "sh backup/.agnostic-ai/scripts/guard.sh"},
		{"sh /external/.agnostic-ai/scripts/guard.sh", "sh /external/.agnostic-ai/scripts/guard.sh"},
		{"sh backup/./.agnostic-ai/scripts/guard.sh", "sh backup/./.agnostic-ai/scripts/guard.sh"},
		{"sh ./.agnostic-ai/scripts/guard.sh", "sh ./.codex/hooks/guard.sh"},
	}
	for _, c := range cases {
		if got := RewriteHookPath(c.command, "codex"); got != c.want {
			t.Errorf("%q became %q, want %q", c.command, got, c.want)
		}
	}
}

func TestRewriteNeutralHookPath_TranslatesRootedSharedScripts(t *testing.T) {
	for _, target := range []string{"gemini", "qoder", "factory"} {
		t.Run(target, func(t *testing.T) {
			command := `sh "$CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/guard.sh"`
			want := `sh "${` + nativeHookRoots[target] + `}/.` + target + `/hooks/guard.sh"`
			if got := RewriteHookPath(command, target); got != want {
				t.Errorf("rooted command = %q, want %q", got, want)
			}
		})
	}
}

func TestNeutralHookScripts_MaterializesAllReferencesAndUsesTargetVariant(t *testing.T) {
	t.Chdir(t.TempDir())
	for path, body := range map[string]string{".agnostic-ai/scripts/pre.sh": "shared", ".agnostic-ai/scripts/codex/pre.sh": "codex", ".agnostic-ai/scripts/nested/post.sh": "post"} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	scripts, err := NeutralHookScripts("sh '.agnostic-ai/scripts/pre.sh' && sh .agnostic-ai/scripts/nested/post.sh", "codex", ".agnostic-ai/scripts", ".codex/hooks")
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) != 2 {
		t.Fatalf("got %d script references", len(scripts))
	}
	if string(scripts[0].Body) != "codex" || filepath.ToSlash(scripts[1].Path) != ".codex/hooks/nested/post.sh" || string(scripts[1].Body) != "post" {
		t.Errorf("wrong scripts: %+v", scripts)
	}
}

func TestNeutralHookScripts_MissingOrEscapingSourceFails(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, command := range []string{".agnostic-ai/scripts/missing.sh", ".agnostic-ai/scripts/../outside.sh", ".agnostic-ai/scripts/"} {
		if _, err := NeutralHookScripts(command, "codex", ".agnostic-ai/scripts", ".codex/hooks"); err == nil || !strings.Contains(err.Error(), ".agnostic-ai/scripts/") {
			t.Errorf("%q error lacks source path: %v", command, err)
		}
	}
}

func TestNeutralHookScripts_RejectsSourceSymlinksOutsideScriptsDirectory(t *testing.T) {
	for _, name := range []string{"guard.sh", "codex/guard.sh", "nested/guard.sh"} {
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.MkdirAll(".agnostic-ai/scripts/codex", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll("outside", 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile("outside/guard.sh", []byte("outside source"), 0o755); err != nil {
				t.Fatal(err)
			}
			link, destination := ".agnostic-ai/scripts/"+name, "../../../outside/guard.sh"
			command := ".agnostic-ai/scripts/guard.sh"
			switch name {
			case "guard.sh":
				destination = "../../outside/guard.sh"
			case "nested/guard.sh":
				link, destination = ".agnostic-ai/scripts/nested", "../../outside"
				command = ".agnostic-ai/scripts/nested/guard.sh"
			}
			if err := os.Symlink(destination, link); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}
			if _, err := NeutralHookScripts(command, "codex", ".agnostic-ai/scripts", ".codex/hooks"); err == nil || !strings.Contains(err.Error(), "outside") {
				t.Errorf("escaping source returned %v", err)
			}
		})
	}
}

func TestNeutralHookScripts_IgnoresUnusedNonCommandHandlerFields(t *testing.T) {
	for _, handler := range []string{"http", "prompt", "agent"} {
		t.Run(handler, func(t *testing.T) {
			t.Chdir(t.TempDir())
			hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
				"event": "PreToolUse", "type": handler, "command": ".agnostic-ai/scripts/missing.sh",
			}}
			if err := NewSession().MaterializeNeutralHookScripts([]spec.Entry{hook}, "claude", ".claude/hooks", false); err != nil {
				t.Errorf("unused command failed: %v", err)
			}
		})
	}
}

func TestNeutralHookScripts_AllowsSourceSymlinksWithinScriptsDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll(".agnostic-ai/scripts", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".agnostic-ai/scripts/body.sh", []byte("shared body"), 0o755); err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(".agnostic-ai/scripts/body.sh")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(absolute, ".agnostic-ai/scripts/guard.sh"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	scripts, err := NeutralHookScripts(".agnostic-ai/scripts/guard.sh", "codex", ".agnostic-ai/scripts", ".codex/hooks")
	if err != nil || len(scripts) != 1 {
		t.Fatalf("internal symlink: %v, %v", scripts, err)
	}
	if string(scripts[0].Body) != "shared body" {
		t.Errorf("copied %q", scripts[0].Body)
	}
}

func TestRewriteGlobalHookPath_UsesCopiedUserScriptForRootedNeutralReference(t *testing.T) {
	const dir = "/tmp/user home/.codex/hooks"
	cases := []struct{ command, want string }{
		{`sh "$CLAUDE_PROJECT_DIR/.agnostic-ai/scripts/guard.sh"`, `sh "/tmp/user home/.codex/hooks/guard.sh"`},
		{`sh "$(git rev-parse --show-toplevel)/.agnostic-ai/scripts/guard.sh"`, `sh "/tmp/user home/.codex/hooks/guard.sh"`},
		{`sh "${GEMINI_PROJECT_DIR}/.agnostic-ai/scripts/guard.sh"`, `sh "/tmp/user home/.codex/hooks/guard.sh"`},
		{`sh "$QODER_PROJECT_DIR/.agnostic-ai/scripts/guard.sh"`, `sh "/tmp/user home/.codex/hooks/guard.sh"`},
		{`sh "${FACTORY_PROJECT_DIR}/.agnostic-ai/scripts/guard.sh"`, `sh "/tmp/user home/.codex/hooks/guard.sh"`},
		{`sh backup/./.agnostic-ai/scripts/guard.sh && sh .agnostic-ai/scripts/guard.sh`, `sh backup/./.agnostic-ai/scripts/guard.sh && sh '/tmp/user home/.codex/hooks'/guard.sh`},
		{`.agnostic-ai/scripts/guard.sh`, `'/tmp/user home/.codex/hooks'/guard.sh`},
		{`sh '.agnostic-ai/scripts/pre.sh' && sh ".agnostic-ai/scripts/post.sh"`, `sh '/tmp/user home/.codex/hooks/pre.sh' && sh "/tmp/user home/.codex/hooks/post.sh"`},
	}
	for _, c := range cases {
		if got := RewriteGlobalHookPath(c.command, "codex", dir); got != c.want {
			t.Errorf("%q became %q, want %q", c.command, got, c.want)
		}
	}
	if got := RewriteGlobalHookPath(".agnostic-ai/scripts/guard.sh", "claude", dir, map[string]any{"args": []any{"--strict"}}); got != dir+"/guard.sh" {
		t.Errorf("exec-form script path was shell quoted: %q", got)
	}
}

func TestRewriteWindowsNeutralHookPath_QuotesWholePathForWindowsShells(t *testing.T) {
	cases := []struct{ command, want string }{
		{`pwsh -File .agnostic-ai/scripts/guard.ps1`, `pwsh -File "C:/Users/My Name/.codex/hooks/guard.ps1"`},
		{`pwsh -File ".agnostic-ai/scripts/guard.ps1" -Check`, `pwsh -File "C:/Users/My Name/.codex/hooks/guard.ps1" -Check`},
		{`pwsh -File '.agnostic-ai/scripts/guard.ps1'`, `pwsh -File 'C:/Users/My Name/.codex/hooks/guard.ps1'`},
	}
	for _, c := range cases {
		if got := RewriteWindowsNeutralHookPath(c.command, "C:/Users/My Name/.codex/hooks"); got != c.want {
			t.Errorf("Windows command = %q, want %q", got, c.want)
		}
	}
}
