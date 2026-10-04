package cli

import (
	"bytes"
	"os"
	"path/filepath"
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

// Augment starts a hook command as a script path, so it cannot pass
// args: sync writes no hook and names `args` in a note.
func TestSync_SkipsExecFormHooksOnAugmentWithANote(t *testing.T) {
	dir := testutil.TempCwd(t)
	captureLogOut(t)
	var notes bytes.Buffer
	adapters.ResetCoverageNotes()
	adapters.SetWarner(&notes)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [augment]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), execFormHook)
	mustSync(t)

	if body, err := os.ReadFile(filepath.Join(dir, ".augment", "settings.json")); err == nil && strings.Contains(string(body), "echo") {
		t.Errorf("augment got the exec-form hook:\n%s", body)
	}
	if !strings.Contains(notes.String(), "1 hook reaches augment only in the source dir") || !strings.Contains(notes.String(), "a hook with `args` is skipped") {
		t.Errorf("want a note naming args, got:\n%s", notes.String())
	}
}

func TestSyncGlobal_SkipsExecFormHooksOnAugmentWithANote(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), execFormHook)
	_, warnings, err := runGlobalAgentTest("--only", "augment")
	if err != nil {
		t.Fatalf("sync --global: %v\n%s", err, warnings)
	}
	if body, err := os.ReadFile(filepath.Join(home, ".augment", "settings.json")); err == nil && strings.Contains(string(body), "echo") {
		t.Errorf("augment got the exec-form hook:\n%s", body)
	}
	if !strings.Contains(warnings, "a hook with `args` is skipped") {
		t.Errorf("want a note naming args, got:\n%s", warnings)
	}
}

// Import reads the folded command back as the shared spec's own, so it
// does not copy the hook again.
func TestImport_SkipsExecFormHooksASharedSpecSyncs(t *testing.T) {
	hook := "name: sh\nevent: PreToolUse\nmatcher: Bash\ncommand: echo\nargs: [shared, two words]\n"
	for _, target := range []string{"claude", "codex", "copilot", "crush", "factory", "gemini", "goose", "kiro", "openhands", "trae", "windsurf"} {
		t.Run(target, func(t *testing.T) {
			syncSharedHook(t, target, hook)

			assertOnlySharedHook(t, hook, importCapturing(t, target))
		})
	}
}
