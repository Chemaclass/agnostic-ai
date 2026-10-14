package cline

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_VSCodeHookWritesNativePowerShellCompanion(t *testing.T) {
	dir := testutil.TempCwd(t)
	hook := spec.Entry{Kind: spec.KindHook, Name: "memory-cline-task-start", Meta: map[string]any{
		"event": "TaskStart", "runtime": "vscode",
		"command":        `printf '{"contextModification":"fixture"}'`,
		"commandWindows": `Write-Output '{"contextModification":"fixture"}'`,
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	unix, err := os.ReadFile(filepath.Join(dir, ".clinerules", "hooks", "TaskStart"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(unix), hookShebang) || !strings.Contains(string(unix), `printf '{"contextModification":"fixture"}'`) {
		t.Errorf("Unix hook lost command or launcher: %s", unix)
	}
	windows, err := os.ReadFile(filepath.Join(dir, ".clinerules", "hooks", "TaskStart.ps1"))
	if runtime.GOOS != "windows" {
		if !os.IsNotExist(err) {
			t.Fatalf("Unix sync must not create a PowerShell hook the SDK would also run: %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(windows), `Write-Output '{"contextModification":"fixture"}'`) || strings.Contains(string(windows), "#!/") {
		t.Errorf("Windows hook must contain native PowerShell, got: %s", windows)
	}
}

func TestGlobalHooks_ExcludeVSCodeOnlyMemoryHook(t *testing.T) {
	hooks := []spec.Entry{
		{Kind: spec.KindHook, Name: "memory-cline-task-start", Meta: map[string]any{"event": "TaskStart", "runtime": "vscode", "command": "echo memory"}},
		{Kind: spec.KindHook, Name: "ordinary", Meta: map[string]any{"event": "PreToolUse", "command": "echo ordinary"}},
	}
	files := GlobalHookFiles(hooks, "/cline-global/hook-scripts")
	if len(files) != 1 || files[0].Path != "PreToolUse" {
		t.Fatalf("VS Code-only memory hook must not reach SDK global hooks: %+v", files)
	}
}
