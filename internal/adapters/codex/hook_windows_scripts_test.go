package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_WindowsHookUsesCopiedTargetScript(t *testing.T) {
	for _, tc := range []struct {
		name, windows, want string
		args                []any
	}{
		{"script path", ".agnostic-ai/scripts/guard.ps1", ".codex/hooks/guard.ps1", nil},
		{"PowerShell command", `pwsh -File ".agnostic-ai/scripts/guard.ps1" --strict`, `pwsh -File ".codex/hooks/guard.ps1" --strict`, nil},
		{"separate POSIX arguments", `pwsh -File ".agnostic-ai/scripts/guard.ps1" --strict`, `pwsh -File ".codex/hooks/guard.ps1" --strict`, []any{"--posix-only"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			for path, body := range map[string]string{
				".agnostic-ai/scripts/guard.ps1":       "Write-Output 'shared'\n",
				".agnostic-ai/scripts/codex/guard.ps1": "Write-Output 'codex'\n",
			} {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			entry := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
				"event":          "PreToolUse",
				"command":        "true",
				"commandWindows": tc.windows,
			}}
			if tc.args != nil {
				entry.Meta["args"] = tc.args
			}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join(dir, ".codex/hooks.json"))
			if err != nil {
				t.Fatal(err)
			}
			var doc struct {
				Hooks map[string][]struct {
					Hooks []struct {
						CommandWindows string `json:"commandWindows"`
					} `json:"hooks"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatal(err)
			}
			groups := doc.Hooks["PreToolUse"]
			if len(groups) != 1 || len(groups[0].Hooks) != 1 {
				t.Fatalf("expected one hook handler:\n%s", raw)
			}
			if got := groups[0].Hooks[0].CommandWindows; got != tc.want {
				t.Errorf("commandWindows = %q, want %q", got, tc.want)
			}
			copied, err := os.ReadFile(filepath.Join(dir, ".codex/hooks/guard.ps1"))
			if err != nil {
				t.Errorf("Windows script was not copied: %v", err)
			} else if string(copied) != "Write-Output 'codex'\n" {
				t.Errorf("copied Windows script = %q, want target variant", copied)
			}
		})
	}
}

func TestHookCommands_WindowsRootKeepsProjectPathBelowGitRoot(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(repo, "packages", "a$b")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, project)
	entry := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
		"event":          "PreToolUse",
		"command":        `"$CLAUDE_PROJECT_DIR/tools/guard.sh"`,
		"commandWindows": `& "$CLAUDE_PROJECT_DIR/tools/guard.ps1"`,
	}}
	got := HookCommands(entry)
	if len(got) != 1 {
		t.Fatalf("HookCommands = %v", got)
	}
	if want := "& \"$(git rev-parse --show-toplevel)/packages/a`$b/tools/guard.ps1\""; got[0].CommandWindows != want {
		t.Errorf("CommandWindows = %q\nwant %q", got[0].CommandWindows, want)
	}
	if want := `"$(git rev-parse --show-toplevel)/packages/a\$b/tools/guard.sh"`; !strings.HasSuffix(got[0].Command, want) {
		t.Errorf("Command = %q\nwant suffix %q", got[0].Command, want)
	}
}
