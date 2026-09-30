package emit

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestMaterializeNeutralHookScripts_WindowsOverrideHasSeparateArguments(t *testing.T) {
	dir := testutil.TempCwd(t)
	if err := os.MkdirAll(".agnostic-ai/scripts/codex", 0o755); err != nil {
		t.Fatal(err)
	}
	const body = "Write-Output 'codex variant'\n"
	if err := os.WriteFile(".agnostic-ai/scripts/codex/guard.ps1", []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	hook := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
		"event":          "PreToolUse",
		"command":        "true",
		"args":           []any{"--posix-only"},
		"commandWindows": `pwsh -File ".agnostic-ai/scripts/guard.ps1" --strict`,
	}}
	if err := NewSession().MaterializeNeutralHookScripts([]spec.Entry{hook}, "codex", ".codex/hooks", false); err != nil {
		t.Fatal(err)
	}
	copied, err := os.ReadFile(filepath.Join(dir, ".codex/hooks/guard.ps1"))
	if err != nil {
		t.Errorf("Windows script was not copied: %v", err)
	} else if string(copied) != body {
		t.Errorf("copied Windows script = %q, want %q", copied, body)
	}
}
