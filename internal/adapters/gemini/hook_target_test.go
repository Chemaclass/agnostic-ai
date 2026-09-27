package gemini

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
