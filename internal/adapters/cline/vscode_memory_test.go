package cline

import (
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/builtins"
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

func nativeBuiltinClineMemoryHook(t *testing.T) spec.Entry {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv("LocalAppData", filepath.Join(home, "cache"))
	root, cleanup, err := builtins.Materialize("memory")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Error(err)
		}
	})
	bundle, err := spec.LoadLayered([]spec.Layer{{Name: "builtin", Root: root, Sources: config.Sources{Hooks: "hooks"}}})
	if err != nil {
		t.Fatal(err)
	}
	for _, hook := range bundle.Hooks {
		if hook.Name != "memory-cline-task-start" {
			continue
		}
		hook.Meta = maps.Clone(hook.Meta)
		// Isolate native metadata resolution from the unsupported portable event.
		delete(hook.Meta, "on")
		hook.Meta["event"] = "TaskStart"
		return emit.TargetHook("cline", hook)
	}
	t.Fatal("memory builtin lacks its Cline hook")
	return spec.Entry{}
}

func TestGlobalHooks_ActualBuiltinVSCodeMarkerOmitsSDKOutput(t *testing.T) {
	hook := nativeBuiltinClineMemoryHook(t)
	if files := GlobalHookFiles([]spec.Entry{hook}, "/cline-global/hook-scripts"); len(files) != 0 {
		t.Fatalf("actual builtin VS Code marker must omit global SDK hooks: %+v", files)
	}
}

func TestRenderWindowsHooks_ActualBuiltinReadsNativeOverride(t *testing.T) {
	hook := nativeBuiltinClineMemoryHook(t)
	override, ok := hook.Meta["x-cline"].(map[string]any)
	if !ok {
		t.Fatal("builtin lacks native Cline override")
	}
	override = maps.Clone(override)
	override["commandWindows"] = "Write-Output native-windows-command"
	hook.Meta["x-cline"] = override
	hook = emit.TargetHook("cline", hook)
	files := renderWindowsHookFiles([]spec.Entry{hook})
	if len(files) != 1 || files[0].Path != "TaskStart.ps1" {
		t.Fatalf("actual builtin must render one native Windows companion: %+v", files)
	}
	if !strings.Contains(string(files[0].Body), "Write-Output native-windows-command") || strings.Contains(string(files[0].Body), "hook memory --target cline") {
		t.Errorf("Windows native override must win: %s", files[0].Body)
	}
}
