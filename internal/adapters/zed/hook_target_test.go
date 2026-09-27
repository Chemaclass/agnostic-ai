package zed

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

func TestEmit_HookTasksCarryTheTargetEnv(t *testing.T) {
	cfg := &config.Config{Outputs: map[string]config.Output{"zed": {TasksFile: ".zed/tasks.json"}}}
	emitTargetHooks(t, cfg,
		spec.Entry{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{"event": "PostToolUse", "command": "gofmt -w ."}},
		spec.Entry{Kind: spec.KindHook, Name: "own", Meta: map[string]any{"event": "PostToolUse", "command": "lint", "x-zed": map[string]any{"env": map[string]any{"FOO": "bar"}}}},
	)
	got := readTargetFile(t, ".zed/tasks.json")
	if n := strings.Count(got, `"AGNOSTIC_AI_TARGET": "zed"`); n != 2 {
		t.Errorf("target env on %d tasks, want 2:\n%s", n, got)
	}
	assertContainsAll(t, got, `"FOO": "bar"`)
}
