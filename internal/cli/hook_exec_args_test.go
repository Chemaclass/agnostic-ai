package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const execFormHook = "name: guard\nevent: PreToolUse\ncommand: echo\nargs: [shared, two words]\n"

const execFormFolded = "echo 'shared' 'two words'"

// A target with no `args` field runs an exec-form hook with its args
// folded into the command, and `hook run` starts that same command.
func TestSync_FoldsExecFormArgsOnTargetsWithoutAnArgsField(t *testing.T) {
	cases := []struct {
		target, path, command string
		// hookRun says `hook run` runs the hook on this target with
		// --bash alone, with no payload file.
		hookRun bool
		config  string
	}{
		{target: "crush", path: "crush.json", command: "export AGNOSTIC_AI_TARGET=crush; " + execFormFolded, hookRun: true},
		{target: "windsurf", path: ".devin/hooks.v1.json", command: execFormFolded, hookRun: true},
		{target: "kiro", path: ".kiro/hooks/guard.json", command: execFormFolded},
		{target: "goose", path: ".agents/plugins/agnostic-ai/hooks/hooks.json", command: "export AGNOSTIC_AI_TARGET=goose; " + execFormFolded, hookRun: true},
		{target: "factory", path: ".factory/hooks.json", command: execFormFolded, hookRun: true},
		{target: "trae", path: ".trae/hooks.json", command: execFormFolded, hookRun: true},
		{target: "openhands", path: ".openhands/hooks.json", command: execFormFolded, hookRun: true},
		{target: "antigravity", path: ".agents/hooks.json", command: execFormFolded, hookRun: true},
		{target: "cline", path: ".clinerules/hooks/PreToolUse", command: execFormFolded},
		{target: "opencode", path: ".opencode/plugins/guard.ts", command: execFormFolded},
		{target: "kilo", path: ".kilo/plugin/guard.ts", command: execFormFolded},
		{target: "zed", path: ".zed/tasks.json", command: execFormFolded, config: "outputs:\n  zed:\n    tasks-file: .zed/tasks.json\n"},
	}
	for _, tc := range cases {
		t.Run(tc.target, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+tc.target+"]\n"+tc.config)
			mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), execFormHook)
			mustSync(t)

			body, err := os.ReadFile(filepath.Join(dir, tc.path))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), tc.command) {
				t.Errorf("%s misses %q:\n%s", tc.path, tc.command, body)
			}
			if !tc.hookRun {
				return
			}
			skipWithoutPOSIXShell(t)
			// echo's stdout is not a reply some targets accept, so the
			// decision may fail; the command and its output are the point.
			out, _ := runHookRun(t, "guard", "--target", tc.target, "--bash", "ls")
			for _, want := range []string{"command: " + tc.command + "\n", "stdout: shared two words"} {
				if !strings.Contains(out, want) {
					t.Errorf("hook run misses %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, "warning:") {
				t.Errorf("a fresh sync warns:\n%s", out)
			}
		})
	}
}

// Crush runs a synced script by its ./ path, so the path keeps it when
// the args fold has to quote it.
func TestHookRun_CrushRunsAQuotedScriptPathWithArgs(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [crush]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "prüfen.sh")
	mustWrite(t, script, "[ \"$1\" = --strict ] && exit 2\nexit 0\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"),
		"name: guard\nevent: PreToolUse\ncommand: .agnostic-ai/scripts/prüfen.sh\nargs: [--strict]\n")
	mustSync(t)

	if body, err := os.ReadFile(filepath.Join(dir, "crush.json")); err != nil || !strings.Contains(string(body), `'./.crush/hooks/prüfen.sh' '--strict'`) {
		t.Errorf("crush.json = %s, %v", body, err)
	}
	if out, err := runHookRun(t, "guard", "--bash", "ls", "--expect", "block"); err != nil {
		t.Errorf("hook run: %v\n%s", err, out)
	}
}

// Trae and OpenHands fold args for a POSIX shell, which their Windows
// shells read differently, so sync says so.
func TestSync_NotesExecFormArgsWindowsCannotRead(t *testing.T) {
	dir := testutil.TempCwd(t)
	captureLogOut(t)
	var notes bytes.Buffer
	adapters.ResetCoverageNotes()
	adapters.SetWarner(&notes)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [trae, openhands]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "plain.yaml"), execFormHook)
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "quote.yaml"), "name: quote\nevent: PreToolUse\ncommand: echo\nargs: [\"it's\"]\n")
	mustSync(t)

	for _, want := range []string{"1 hook reaches trae but not Windows (Trae runs a hook in PowerShell", "2 hooks reach openhands but not Windows (OpenHands runs a hook with cmd.exe"} {
		if !strings.Contains(notes.String(), want) {
			t.Errorf("notes miss %q:\n%s", want, notes.String())
		}
	}
}

// Auggie reads a handler's `args`, so sync writes them apart.
func TestSync_WritesExecFormHookArgsOnAugment(t *testing.T) {
	dir := testutil.TempCwd(t)
	captureLogOut(t)
	var notes bytes.Buffer
	adapters.ResetCoverageNotes()
	adapters.SetWarner(&notes)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [augment]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), execFormHook)
	mustSync(t)

	body, err := os.ReadFile(filepath.Join(dir, ".augment", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"command": "echo"`) || !strings.Contains(string(body), `"two words"`) {
		t.Errorf("augment did not get the exec-form hook with its args:\n%s", body)
	}
	if strings.Contains(notes.String(), "args") {
		t.Errorf("want no args note, got:\n%s", notes.String())
	}
}

func TestSyncGlobal_WritesExecFormHookArgsOnAugment(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), execFormHook)
	_, warnings, err := runGlobalAgentTest("--only", "augment")
	if err != nil {
		t.Fatalf("sync --global: %v\n%s", err, warnings)
	}
	path := filepath.Join(home, ".augment", "settings.json")
	var doc map[string]any
	if err := json.Unmarshal([]byte(readGlobalTest(t, path)), &doc); err != nil {
		t.Fatal(err)
	}
	hooks, _ := doc["hooks"].(map[string]any)
	groups, _ := hooks["PreToolUse"].([]any)
	if len(groups) != 1 {
		t.Fatalf("hooks = %v", doc["hooks"])
	}
	handler := groups[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	if handler["command"] != "echo" || !reflect.DeepEqual(handler["args"], []any{"shared", "two words"}) {
		t.Errorf("handler = %v, want echo with its args", handler)
	}
	if strings.Contains(warnings, "args") {
		t.Errorf("want no args note, got:\n%s", warnings)
	}
	if _, _, err := runGlobalAgentTest("--only", "augment", "--check"); err != nil {
		t.Errorf("check: %v", err)
	}
}
