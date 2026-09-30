package gemini

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func emitTargetHooks(t *testing.T, cfg *config.Config, entries ...spec.Entry) {
	t.Helper()
	testutil.TempCwd(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
}

func readTargetFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertContainsAll(t *testing.T, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("missing %s in:\n%s", w, got)
		}
	}
}

func TestEmit_HookHandlersCarryTheTargetEnv(t *testing.T) {
	emitTargetHooks(t, &config.Config{},
		spec.Entry{Kind: spec.KindHook, Name: "plain", Meta: map[string]any{"event": "BeforeTool", "command": "guard.sh"}},
		spec.Entry{Kind: spec.KindHook, Name: "native", Meta: map[string]any{"event": "AfterTool", "command": "log.sh", "x-gemini": map[string]any{"env": map[string]any{"FOO": "bar"}}}},
		spec.Entry{Kind: spec.KindHook, Name: "group", Meta: map[string]any{"event": "SessionStart", "x-gemini": map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "start.sh"}}}}},
	)
	got := readTargetFile(t, ".gemini/settings.json")
	if n := strings.Count(got, `"AGNOSTIC_AI_TARGET": "gemini"`); n != 3 {
		t.Errorf("target env on %d handlers, want 3:\n%s", n, got)
	}
	assertContainsAll(t, got, `"FOO": "bar"`, `"command": "guard.sh"`)
}

func TestEmit_NativeHookRootWarnsAndHonorsError(t *testing.T) {
	testutil.TempCwd(t)
	hook := spec.Entry{Kind: spec.KindHook, Name: "native-guard", Path: "hooks/native-guard.yaml", Meta: map[string]any{"event": "BeforeTool", "x-gemini": map[string]any{"hooks": []any{map[string]any{"type": "command", "command": `"${CLAUDE_PROJECT_DIR:-/tmp}/guard.sh"`}}}}}
	bundle := spec.NewBundle([]spec.Entry{hook})
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{OnUnsupported: "error"}, false); err == nil || !strings.Contains(err.Error(), "native-guard") {
		t.Errorf("want named native hook error, got %v", err)
	}
	previous := emit.Warner
	var warnings bytes.Buffer
	emit.Warner = &warnings
	emit.ResetCoverageNotes()
	t.Cleanup(func() { emit.Warner = previous; emit.ResetCoverageNotes() })
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(warnings.String(), "native-guard") || !strings.Contains(warnings.String(), "CLAUDE_PROJECT_DIR") {
		t.Errorf("missing native hook warning: %s", warnings.String())
	}
	got := readTargetFile(t, ".gemini/settings.json")
	if !strings.Contains(got, "${CLAUDE_PROJECT_DIR:-/tmp}") {
		t.Errorf("unsupported native command changed: %s", got)
	}
}

func TestEmit_NativeHookRootIgnoresOverriddenTopLevelCommand(t *testing.T) {
	testutil.TempCwd(t)
	hook := spec.Entry{Kind: spec.KindHook, Name: "native-safe", Meta: map[string]any{"event": "BeforeTool", "command": `"${CLAUDE_PROJECT_DIR:-/tmp}/ignored.sh"`, "x-gemini": map[string]any{"hooks": []any{map[string]any{"type": "command", "command": `"${CLAUDE_PROJECT_DIR}/safe.sh"`, "shell": "bash"}}}, "shell": "powershell"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{OnUnsupported: "error"}, false); err != nil {
		t.Fatal(err)
	}
	got := readTargetFile(t, ".gemini/settings.json")
	if !strings.Contains(got, "safe.sh") || !strings.Contains(got, "${GEMINI_PROJECT_DIR}") || strings.Contains(got, "ignored.sh") {
		t.Errorf("native override rendered wrong command: %s", got)
	}
}
