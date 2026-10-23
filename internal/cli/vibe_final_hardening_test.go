package cli

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestSyncGlobal_VibeModelConfigIsPrivateFromFirstPlan(t *testing.T) {
	for _, scenario := range []string{"model-only", "model-and-hooks", "existing-mode"} {
		t.Run(scenario, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			path := filepath.Join(home, ".vibe/config.toml")
			original := ""
			if scenario == "existing-mode" {
				original = "# user configuration\nkey=1\n"
				mustWriteGlobalTest(t, path, original)
				if err := os.Chmod(path, 0o640); err != nil {
					t.Fatal(err)
				}
			}
			entries := []spec.Entry{{Kind: spec.KindSettings, Name: "model", Meta: map[string]any{"model": "devstral"}}}
			if scenario != "model-only" {
				entries = append(entries, spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{"target": "vibe", "event": "pre_tool", "command": "echo '{}'"}})
			}
			writes, _, err := buildGlobalWrites(home, source, []string{"vibe"}, nil, spec.NewBundle(entries), globalState{Version: globalStateVersion}, nil, io.Discard, "warn")
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, w := range writes {
				if w.path != path {
					continue
				}
				count++
				if w.mode != 0o600 {
					t.Errorf("first config plan mode = %o, want 600", w.mode)
				}
				if !strings.Contains(string(w.data), `active_model = "devstral"`) || !strings.HasPrefix(string(w.data), original) {
					t.Errorf("planned config lost settings or user bytes: %s", w.data)
				}
				if w.planned == nil || w.planned.absent != (scenario != "existing-mode") {
					t.Error("config plan missing original-file guard")
				}
			}
			if count != 1 {
				t.Fatalf("planned %d config writes, want 1", count)
			}
			if _, err := applyGlobalChanges(writes, nil, false, nil); err != nil {
				t.Fatal(err)
			}
			body := readGlobalTest(t, path)
			if !strings.Contains(body, `active_model = "devstral"`) || !strings.HasPrefix(body, original) {
				t.Errorf("written config lost settings or original bytes: %s", body)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			wantMode := os.FileMode(0o600)
			if scenario == "existing-mode" {
				wantMode = 0o640
			}
			if runtime.GOOS != "windows" && info.Mode().Perm() != wantMode {
				t.Errorf("written config mode = %o, want %o", info.Mode().Perm(), wantMode)
			}
			if scenario != "model-only" {
				hooks := readGlobalTest(t, filepath.Join(home, ".vibe/hooks.toml"))
				if !strings.Contains(hooks, "pre_tool") {
					t.Error("model plus hooks plan lost hook output")
				}
			}
		})
	}
}
