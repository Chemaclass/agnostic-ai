package cursor

import (
	"encoding/json"
	"os"
	"os/exec"
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

// Cursor runs hooks through PowerShell on Windows, where a POSIX prefix
// would fail. A sessionStart hook's `env` reaches every later hook.
func TestEmit_HooksSetTheTargetThroughSessionStartEnv(t *testing.T) {
	emitTargetHooks(t, &config.Config{}, spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"event": "beforeShellExecution", "command": "guard.sh"}})
	got := readTargetFile(t, ".cursor/hooks.json")
	assertContainsAll(t, got,
		`"command": "guard.sh"`,
		`"sessionStart": [`,
		`"command": "echo '{\"env\":{\"AGNOSTIC_AI_TARGET\":\"cursor\"}}'"`,
	)
}

func TestHookTargetCommand_PrintsTheSessionEnv(t *testing.T) {
	for _, shell := range []string{"sh", "bash", "zsh", "fish", "pwsh"} {
		if _, err := exec.LookPath(shell); err != nil {
			continue
		}
		args := []string{"-c", HookTargetCommand}
		if shell == "pwsh" {
			args = []string{"-NoProfile", "-Command", HookTargetCommand}
		}
		out, err := exec.Command(shell, args...).Output()
		if err != nil {
			t.Errorf("%s: %v", shell, err)
			continue
		}
		var reply struct {
			Env map[string]string `json:"env"`
		}
		if err := json.Unmarshal(out, &reply); err != nil || reply.Env[emit.HookTargetEnv] != "cursor" {
			t.Errorf("%s printed %q: %v", shell, out, err)
		}
	}
}
