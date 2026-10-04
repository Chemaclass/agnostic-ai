package gemini

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_HooksReachNativeLoader(t *testing.T) {
	dir := testutil.TempCwd(t)
	bundle := spec.NewBundle([]spec.Entry{{Kind: spec.KindHook, Name: "check", Meta: map[string]any{
		"event": "BeforeTool", "matcher": "write_file", "command": []any{"echo first", "echo second"},
		"timeout": 5, "description": "Check changes", "x-gemini": map[string]any{"sequential": true},
	}}})
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hooks map[string][]struct {
			Matcher    string
			Sequential bool
			Hooks      []struct {
				Type, Command, Description string
				Timeout                    int
			}
		}
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".gemini/settings.json"))), &settings); err != nil {
		t.Fatal(err)
	}
	groups := settings.Hooks["BeforeTool"]
	if len(groups) != 1 || len(groups[0].Hooks) != 2 {
		t.Fatalf("native loader needs one definition with two nested handlers, got %+v", groups)
	}
	if groups[0].Matcher != "write_file" || !groups[0].Sequential {
		t.Errorf("definition lost its matcher or execution order: %+v", groups[0])
	}
	for i, command := range []string{"echo first", "echo second"} {
		hook := groups[0].Hooks[i]
		if hook.Type != "command" || hook.Command != command || hook.Timeout != 5000 || hook.Description != "Check changes" {
			t.Errorf("handler %d lost native fields: %+v", i, hook)
		}
	}
}

func TestEmit_HookCopiesScriptStashedUnderSourceTool(t *testing.T) {
	dir := testutil.TempCwd(t)
	stash := filepath.Join(dir, ".agnostic-ai", "scripts", "claude")
	if err := os.MkdirAll(stash, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stash, "fmt.sh"), []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bundle := spec.NewBundle([]spec.Entry{{Kind: spec.KindHook, Name: "fmt", Meta: map[string]any{
		"event": "AfterTool", "matcher": "write_file", "command": ".claude/hooks/fmt.sh", "args": []any{"--check"},
	}}})
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, ".gemini", "hooks", "fmt.sh")); got != "#!/bin/sh\necho hi\n" {
		t.Errorf("script body = %q", got)
	}
}

func TestEmit_StdoutDecisionWrapsNativeHandlers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("runs the wrapper with bash")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not on PATH")
	}
	dir := testutil.TempCwd(t)
	hook, problem := (spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: map[string]any{
		"on": "before-tool", "decision": "stdout", "x-gemini": map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "printf bad"}}},
	}}).NativeHook("gemini")
	if problem != "" {
		t.Fatal(problem)
	}
	bundle := spec.NewBundle([]spec.Entry{hook})
	if err := New().Emit(emit.NewSession(), bundle, &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hooks map[string][]struct{ Hooks []struct{ Command string } }
	}
	if err := json.Unmarshal([]byte(readFile(t, filepath.Join(dir, ".gemini/settings.json"))), &settings); err != nil {
		t.Fatal(err)
	}
	groups := settings.Hooks["BeforeTool"]
	if len(groups) != 1 || len(groups[0].Hooks) != 1 {
		t.Fatalf("handlers = %+v", groups)
	}
	command := groups[0].Hooks[0].Command
	if !strings.Contains(command, "--decision") {
		t.Errorf("native handler must read the decision: %q", command)
	}
	out, err := exec.Command("bash", "-c", command).Output()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 2 {
		t.Errorf("native handler must block malformed stdout: %v, %q", err, out)
	}
	var reply struct{ Decision string }
	if err := json.Unmarshal(out, &reply); err != nil {
		t.Errorf("reply = %q: %v", out, err)
	} else if reply.Decision != "deny" {
		t.Errorf("decision = %q", reply.Decision)
	}
}
