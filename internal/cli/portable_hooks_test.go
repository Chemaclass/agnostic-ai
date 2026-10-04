package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestValidateAndLint_ReportInvalidPortableHooks(t *testing.T) {
	dir := newProject(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex]\n")
	hooks := filepath.Join(dir, ".agnostic-ai", "hooks")
	mustWrite(t, filepath.Join(hooks, "typo.yaml"), "on: befor-tool\ncommand: x\n")
	mustWrite(t, filepath.Join(hooks, "read.yaml"), "on: before-tool\nmatch: read\ncommand: x\n")
	mustWrite(t, filepath.Join(hooks, "both.yaml"), "on: stop\nevent: Stop\ncommand: x\n")
	mustWrite(t, filepath.Join(hooks, "ok.yaml"), "on: before-tool\nmatch: read\ntarget-exclude: codex\ncommand: x\n")

	out, err := runCLI(t, "validate")
	if err == nil {
		t.Fatalf("validate must fail:\n%s", out)
	}
	lint, _ := runCLI(t, "lint")
	for _, want := range []string{
		`typo.yaml: unknown hook event "befor-tool" for on: (did you mean before-tool?)`,
		"read.yaml: codex has no read tool a hook can match; add target-exclude: codex or use event:",
		"both.yaml: sets both on: and event:; keep one",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("validate misses %q:\n%s", want, out)
		}
		if !strings.Contains(lint, want) {
			t.Errorf("lint misses %q:\n%s", want, lint)
		}
	}
	if strings.Contains(out, "ok.yaml") || strings.Contains(lint, "ok.yaml") || len(findingLines(lint, "LINT032")) != 3 {
		t.Errorf("want three LINT032 findings, none for ok.yaml:\n%s\n%s", out, lint)
	}
}

func TestSync_StopsOnAnInvalidPortableHook(t *testing.T) {
	dir := newProject(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "on: before-tool\nmatch: shel\ncommand: x\n")
	if _, err := runCLI(t, "sync", "--gitignore=off"); err == nil || !strings.Contains(err.Error(), "did you mean shell") {
		t.Errorf("sync = %v, want it to stop on the typo", err)
	}
}

func TestSync_PortableHookReachesClaudeAndCodexAndNotesTheRest(t *testing.T) {
	dir := newProject(t)
	captureLogOut(t)
	var notes bytes.Buffer
	adapters.ResetCoverageNotes()
	adapters.SetWarner(&notes)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, kiro]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "on: before-tool\nmatch: shell\ncommand: exit 2\n")
	mustSync(t)

	for _, path := range []string{".claude/settings.json", ".codex/hooks.json"} {
		body, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil || !strings.Contains(string(body), `"PreToolUse"`) || !strings.Contains(string(body), `"matcher": "Bash"`) {
			t.Errorf("%s = %s, %v; want a PreToolUse Bash hook", path, body, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".kiro", "hooks", "guard.json")); !os.IsNotExist(err) {
		t.Errorf("kiro got the portable hook before it translates: %v", err)
	}
	if !strings.Contains(notes.String(), "1 hook reaches kiro only in the source dir (on: has no kiro mapping yet; write event: for kiro)") {
		t.Errorf("notes = %q, want one kiro note", notes.String())
	}
}

// Cursor and Copilot read a block from a JSON reply, so sync wraps a
// portable before-tool command; the same hook in the native form syncs
// as written, and removing the portable hook removes the wrapper.
func TestSync_WrapsAPortableBeforeToolHookOnCursorAndCopilotOnly(t *testing.T) {
	dir := newProject(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cursor, copilot]\n")
	guard := filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml")
	mustWrite(t, guard, "on: before-tool\nmatch: shell\ncommand: ./guard.sh\n")
	mustSync(t)

	for path, want := range map[string]string{
		".claude/settings.json":          `"command": "[ \"$AGNOSTIC_AI_TARGET\" = cursor ] && exit 0; ./guard.sh"`,
		".cursor/hooks.json":             `"command": ".cursor/hooks/agnostic-ai-portable-hook.sh './guard.sh'"`,
		".github/hooks/agnostic-ai.json": `"command": ".github/hooks/scripts/agnostic-ai-portable-hook.sh './guard.sh'"`,
	} {
		if body, err := os.ReadFile(filepath.Join(dir, path)); err != nil || !strings.Contains(string(body), want) {
			t.Errorf("%s = %s, %v; want %s", path, body, err, want)
		}
	}
	wrappers := []string{".cursor/hooks/agnostic-ai-portable-hook.sh", ".github/hooks/scripts/agnostic-ai-portable-hook.sh"}
	for _, path := range wrappers {
		if _, err := os.Stat(filepath.Join(dir, path)); err != nil {
			t.Errorf("%s: %v", path, err)
		}
	}
	if out, err := runCLI(t, "sync", "--check", "--gitignore=off"); err != nil {
		t.Fatalf("sync --check: %v\n%s", err, out)
	}

	mustWrite(t, guard, "event: preToolUse\nmatcher: ^Shell$\ntarget: cursor\ncommand: ./guard.sh\n")
	mustSync(t)
	if body, err := os.ReadFile(filepath.Join(dir, ".cursor", "hooks.json")); err != nil || !strings.Contains(string(body), `"command": "./guard.sh"`) {
		t.Errorf("a native hook syncs as written: %s, %v", body, err)
	}
	for _, path := range wrappers {
		if _, err := os.Stat(filepath.Join(dir, path)); !os.IsNotExist(err) {
			t.Errorf("%s must go with the last portable hook: %v", path, err)
		}
	}
}

func TestImportCopilot_UnwrapsAPortableHookCommand(t *testing.T) {
	dir := newProject(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [copilot]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "on: before-tool\nmatch: shell\ncommand: ./guard.sh 'it'\\''s'\n")
	mustSync(t)

	imported := t.TempDir()
	if _, err := importCopilotHooks(dir, imported); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(imported)
	if err != nil || len(entries) != 1 {
		t.Fatalf("imported %v, %v; want one hook", entries, err)
	}
	body, err := os.ReadFile(filepath.Join(imported, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(body, &doc); err != nil || doc["command"] != `./guard.sh 'it'\''s'` || doc["on"] != "before-tool" || doc["match"] != "shell" || doc["event"] != nil {
		t.Errorf("import must restore the portable spec, not the wrapper:\n%s %v", body, err)
	}
}

// sync, import copilot, sync again: the hook keeps its wrapper, so a
// guard's exit 2 still denies with its reason.
func TestImportCopilot_PortableHookRoundTrips(t *testing.T) {
	dir := newProject(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [copilot]\n")
	guard := filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml")
	mustWrite(t, guard, "on: before-tool\nmatch: shell\ncommand: ./guard.sh\n")
	mustSync(t)
	synced, err := os.ReadFile(filepath.Join(dir, ".github", "hooks", "agnostic-ai.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.Remove(guard); err != nil {
		t.Fatal(err)
	}
	if _, err := importCopilotHooks(dir, filepath.Join(dir, ".agnostic-ai", "hooks")); err != nil {
		t.Fatal(err)
	}
	mustSync(t)
	if again, err := os.ReadFile(filepath.Join(dir, ".github", "hooks", "agnostic-ai.json")); err != nil || string(again) != string(synced) {
		t.Errorf("round trip changed the hooks file:\n%s\nwant:\n%s", again, synced)
	}
}

func TestHookRun_PortableHookRunsOnClaudeAndCodex(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := hookRunProject(t, "name: guard\non: before-tool\nmatch: shell\n"+
		`command: 'if grep -q "push --force"; then echo "no force push" >&2; exit 2; fi'`+"\n")
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, kiro]\n")

	out, err := runHookRun(t, "guard", "--bash", "git push --force", "--expect", "block")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, want := range []string{
		"claude: block (exit 2", "codex: block (exit 2", "event: PreToolUse (Bash)",
		"kiro: not run (on: has no kiro mapping yet; write event: for kiro)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
}

func TestHookRun_PortableEditHookRunsOnEveryEditTool(t *testing.T) {
	skipWithoutPOSIXShell(t)
	hookRunProject(t, "name: guard\non: before-tool\nmatch: edit\ncommand: 'echo protected >&2; exit 2'\n")

	out, err := runHookRun(t, "guard", "--edit", "src/app.go", "--expect", "block", "--target", "claude,codex")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, want := range []string{"claude: block (exit 2", "codex: block (exit 2"} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
}

func TestSyncGlobal_StopsOnAnInvalidPortableHook(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\non: befor-tool\ncommand: 'true'\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err == nil || !strings.Contains(err.Error(), "did you mean before-tool") {
		t.Errorf("sync --global = %v, want it to stop on the typo", err)
	}
}

func TestSyncGlobal_PortableHookReachesClaudeAndCursor(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "guard.yaml"), "name: guard\non: before-tool\nmatch: shell\ncommand: 'exit 2'\n")
	_, warnings, err := runGlobalAgentTest("--only", "claude,cursor")
	if err != nil {
		t.Fatalf("sync --global: %v\n%s", err, warnings)
	}
	if got := firstGlobalHandler(t, readGlobalJSON(t, filepath.Join(home, ".claude", "settings.json")), "PreToolUse")["command"]; got != "exit 2" {
		t.Errorf("claude PreToolUse handler = %v", got)
	}
	wrapper := filepath.ToSlash(filepath.Join(home, ".cursor", "hooks", "agnostic-ai-portable-hook.sh"))
	entries, _ := readGlobalJSON(t, filepath.Join(home, ".cursor", "hooks.json"))["hooks"].(map[string]any)["preToolUse"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["command"] != wrapper+" 'exit 2'" {
		got := entries
		t.Errorf("cursor preToolUse handler = %v, want it through %s", got, wrapper)
	}
	if info, err := os.Stat(filepath.FromSlash(wrapper)); err != nil || runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
		t.Errorf("wrapper = %v, %v; want an executable file", info, err)
	}
	if strings.Contains(warnings, "reaches cursor only in the source dir") {
		t.Errorf("cursor gets the hook, so no note:\n%s", warnings)
	}
}

// A portable event maps only where the target reads exit 0, exit 1, and
// exit 2 with stderr as Claude Code does, so one script decides alike.
func TestPortableHookTargets_DecideLikeClaudeCode(t *testing.T) {
	results := []hookrun.Result{{Exit: 0}, {Exit: 1, Stderr: "failed"}, {Exit: 2, Stderr: "blocked"}}
	// What the command sync wraps returns on Cline, and on Cursor and
	// Copilot before-tool; the adapter tests run the wrappers themselves.
	synced := func(target, on string, r hookrun.Result) hookrun.Result {
		switch {
		case target == "cline" && r.Exit == 2:
			return hookrun.Result{Stdout: "HOOK_CONTROL\t{\"cancel\": true, \"errorMessage\": \"blocked\"}\n", Stderr: r.Stderr}
		case target == "cursor" && on == "prompt-submit" && r.Exit == 0:
			return hookrun.Result{Stdout: `{"continue":true}` + "\n"}
		case target == "cursor" && on == "prompt-submit" && r.Exit == 2:
			return hookrun.Result{Stdout: `{"continue":false,"user_message":"blocked"}` + "\n", Stderr: r.Stderr}
		case target == "cursor" && on == "before-tool" && r.Exit == 0:
			return hookrun.Result{Stdout: `{"permission":"allow"}` + "\n"}
		case target == "cursor" && on == "before-tool" && r.Exit == 2:
			return hookrun.Result{Stdout: `{"permission":"deny","user_message":"blocked","agent_message":"blocked"}` + "\n", Stderr: r.Stderr}
		case target == "copilot" && on == "before-tool" && r.Exit == 2:
			return hookrun.Result{Exit: 2, Stdout: `{"permissionDecision":"deny","permissionDecisionReason":"blocked"}` + "\n", Stderr: r.Stderr}
		}
		return r
	}
	for _, target := range spec.PortableHookTargets() {
		for _, on := range spec.PortableHookEvents {
			event, ok := spec.PortableHookEvent(target, on)
			if !ok {
				continue
			}
			if _, known := hookEventsByTarget[target][event]; !known {
				t.Errorf("%s %s: %s is not one of its events", target, on, event)
			}
			claudeEvent, _ := spec.PortableHookEvent("claude", on)
			for _, r := range results {
				want := hookrun.DecideHandler("claude", claudeEvent, hookrun.Handler{}, r)
				got := hookrun.DecideHandler(target, event, hookrun.Handler{}, synced(target, on, r))
				// Cline never reads the exit code, so exit 1 lets the call
				// go on unreported.
				if target == "cline" && r.Exit == 1 && got == hookrun.Allow && want == hookrun.Error {
					continue
				}
				// Copilot fails a tool call closed on exit 1, the safe side:
				// a broken guard keeps blocking.
				if target == "copilot" && on == "before-tool" && r.Exit == 1 && got == hookrun.Block && want == hookrun.Error {
					continue
				}
				if got != want {
					t.Errorf("%s %s (%s) exit %d = %s, Claude Code %s", target, on, event, r.Exit, got, want)
				}
			}
		}
	}
}

func TestHookRun_PortableShellHookBlocksOnEveryMappedTarget(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, codex, gemini, factory, qoder, trae, openhands, goose, augment, crush, windsurf, cline, copilot, cursor, kiro]\n")
	script := filepath.Join(dir, ".agnostic-ai", "scripts", "guard.sh")
	mustWrite(t, script, "#!/bin/sh\nif grep -q \"push --force\"; then echo \"no force push\" >&2; exit 2; fi\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "name: guard\non: before-tool\nmatch: shell\ncommand: .agnostic-ai/scripts/guard.sh\n")
	mustSync(t)

	out, err := runHookRun(t, "guard", "--bash", "git push --force", "--expect", "block", "--include-assumed")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, target := range []string{"claude", "codex", "gemini", "factory", "qoder", "openhands", "goose", "augment", "crush", "windsurf", "copilot"} {
		if !strings.Contains(out, target+": block (exit 2") {
			t.Errorf("%s must block:\n%s", target, out)
		}
	}
	// The wrappers reply at exit 0: Cursor with a deny, Cline with a cancel.
	for _, want := range []string{
		"cursor: block (exit 0", `"permission":"deny","user_message":"no force push"`,
		"cline: block (exit 0", `"cancel": true, "errorMessage": "no force push"`,
		`"permissionDecision":"deny","permissionDecisionReason":"no force push"`,
		"kiro: not run (on: has no kiro mapping yet",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}

	allowed, err := runHookRun(t, "guard", "--bash", "git push", "--expect", "allow", "--include-assumed")
	if err != nil {
		t.Fatalf("a plain push must pass everywhere: %v\n%s", err, allowed)
	}
	if !strings.Contains(allowed, `stdout: {"permission":"allow"}`) {
		t.Errorf("cursor needs an allow reply at exit 0:\n%s", allowed)
	}
}

// A native hook that shares Cline's script with a filtered portable one
// is still found in the synced script, so hook run warns of no drift.
func TestHookRun_NativeClineHookBesideAFilteredOneShowsNoDrift(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := hookRunProject(t, "name: guard\non: before-tool\nmatch: shell\ncommand: 'exit 0'\n")
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cline]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "audit.yaml"), "name: audit\nevent: PreToolUse\ncommand: [\"cat >/dev/null\", 'exit 0']\n")
	mustSync(t)

	for _, hook := range []string{"guard", "audit"} {
		out, err := runHookRun(t, hook, "--bash", "ls", "--include-assumed")
		if err != nil || strings.Contains(out, "warning:") {
			t.Errorf("%s: %v\n%s", hook, err, out)
		}
	}
}

// A Cline hook with match: shell runs only on shell calls; an edit call
// passes it by.
func TestHookRun_PortableShellHookSkipsAnEditOnCline(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := hookRunProject(t, "name: guard\non: before-tool\nmatch: shell\ncommand: 'echo never >&2; exit 2'\n")
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cline]\n")
	mustSync(t)

	out, err := runHookRun(t, "guard", "--edit", "src/app.go", "--expect", "allow", "--include-assumed")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if !strings.Contains(out, "cline: allow (exit 0") || strings.Contains(out, "never") || strings.Contains(out, "Cline has no matcher") {
		t.Errorf("cline must skip the guard on an editor call, with no note that sync drops the matcher:\n%s", out)
	}
}

func TestHookRun_PortableHookWithoutMatchBlocksOnCline(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := hookRunProject(t, "name: guard\non: before-tool\n"+
		`command: 'if grep -q "push --force"; then echo "no force push" >&2; exit 2; fi'`+"\n")
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cline]\n")

	out, err := runHookRun(t, "guard", "--bash", "git push --force", "--expect", "block", "--include-assumed")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, want := range []string{"claude: block (exit 2", "cline: block"} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
}
