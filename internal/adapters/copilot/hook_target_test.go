package copilot

import (
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

func TestEmit_HookEntriesCarryTheTargetEnv(t *testing.T) {
	emitTargetHooks(t, &config.Config{},
		spec.Entry{Kind: spec.KindHook, Name: "shell", Meta: map[string]any{"event": "preToolUse", "command": "guard.sh", "env": map[string]any{"FOO": "bar"}}},
		spec.Entry{Kind: spec.KindHook, Name: "exec", Meta: map[string]any{"event": "sessionEnd", "command": "node", "args": []any{"done.js"}}},
	)
	got := readTargetFile(t, ".github/hooks/agnostic-ai.json")
	if n := strings.Count(got, `"AGNOSTIC_AI_TARGET": "copilot"`); n != 2 {
		t.Errorf("target env on %d entries, want 2:\n%s", n, got)
	}
	assertContainsAll(t, got, `"FOO": "bar"`, `"command": "guard.sh"`)
}

func TestEmit_NonPOSIXHookRootStaysUnchanged(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.Mkdir(".git", 0o755); err != nil {
		t.Fatal(err)
	}
	command := `"$CLAUDE_PROJECT_DIR/custom/guard.ps1"`
	hook := spec.Entry{Kind: spec.KindHook, Name: "powershell-guard", Meta: map[string]any{"event": "preToolUse", "command": command, "shell": "powershell"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{OnUnsupported: "warn"}, false); err != nil {
		t.Fatal(err)
	}
	got := readTargetFile(t, ".github/hooks/agnostic-ai.json")
	if !strings.Contains(got, `\"$CLAUDE_PROJECT_DIR/custom/guard.ps1\"`) || strings.Contains(got, "git rev-parse") {
		t.Errorf("non-POSIX root changed: %s", got)
	}
}
