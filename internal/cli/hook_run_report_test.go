package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

type hookRunJSONReport struct {
	Hook    string `json:"hook"`
	Error   string `json:"error"`
	Targets []struct {
		Target   string   `json:"target"`
		Decision string   `json:"decision"`
		Reason   string   `json:"reason"`
		Event    string   `json:"event"`
		Trigger  string   `json:"trigger"`
		Warnings []string `json:"warnings"`
		Commands []struct {
			Command   string `json:"command"`
			Decision  string `json:"decision"`
			ExitCode  *int   `json:"exit_code"`
			ElapsedMS *int64 `json:"elapsed_ms"`
			TimedOut  bool   `json:"timed_out"`
			Stdout    string `json:"stdout"`
			Stderr    string `json:"stderr"`
		} `json:"commands"`
	} `json:"targets"`
}

func decodeHookRunJSON(t *testing.T, out string) hookRunJSONReport {
	t.Helper()
	var r hookRunJSONReport
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("output is no JSON report: %v\n%s", err, out)
	}
	return r
}

func TestHookRun_FormatJSONPrintsOneResultPerTarget(t *testing.T) {
	skipWithoutPOSIXShell(t)
	hookRunProject(t, "name: guard\nevent: PreToolUse\nmatcher: Edit|Write\n"+
		`command: 'if grep -q "\.github/workflows/"; then echo "protected file" >&2; exit 2; fi'`+"\n")

	out, err := runHookRun(t, "guard", "--edit", ".github/workflows/tests.yml", "--expect", "block", "--format", "json")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if !strings.Contains(out, ` >&2; exit 2`) {
		t.Errorf("commands are not printed as written:\n%s", out)
	}
	r := decodeHookRunJSON(t, out)
	if r.Hook != "guard" || r.Error != "" || len(r.Targets) != 3 {
		t.Fatalf("report = %+v, want guard with three targets and no error", r)
	}
	for i, target := range []string{"claude", "codex"} {
		got := r.Targets[i]
		if got.Target != target || got.Decision != "block" || got.Event != "PreToolUse" || len(got.Commands) != 1 {
			t.Fatalf("targets[%d] = %+v, want %s blocking with one command", i, got, target)
		}
		c := got.Commands[0]
		if c.Decision != "block" || c.ExitCode == nil || *c.ExitCode != 2 || c.ElapsedMS == nil || c.Stderr != "protected file\n" || c.Command == "" {
			t.Errorf("%s command = %+v, want exit 2 with its time, command, and stderr", target, c)
		}
	}
	if cursor := r.Targets[2]; cursor.Target != "cursor" || cursor.Decision != "not run" || cursor.Reason == "" {
		t.Errorf("cursor = %+v, want not run with a reason", cursor)
	}
}

func TestHookRun_FormatJSONNamesTheFailure(t *testing.T) {
	skipWithoutPOSIXShell(t)
	hookRunProject(t, "name: guard\nevent: SessionStart\nmatcher: compact\ntarget: claude\ntimeout: 1\ncommand: 'sleep 10'\n")

	out, err := runHookRun(t, "guard", "--format", "json")
	if err == nil {
		t.Fatalf("err = nil, want the timeout to fail the run\n%s", out)
	}
	r := decodeHookRunJSON(t, out)
	if r.Error != err.Error() || len(r.Targets) == 0 {
		t.Fatalf("report = %+v, want error %q", r, err)
	}
	c := r.Targets[0].Commands[0]
	if r.Targets[0].Decision != "timeout" || !c.TimedOut || c.ExitCode != nil {
		t.Errorf("claude = %+v, want a timeout with no exit code", r.Targets[0])
	}
}

func TestHookRun_RejectsAnUnknownFormat(t *testing.T) {
	hookRunProject(t, "name: guard\nevent: PreToolUse\ntarget: claude\ncommand: 'true'\n")

	if _, err := runHookRun(t, "guard", "--bash", "ls", "--format", "yaml"); err == nil || !strings.Contains(err.Error(), "--format") {
		t.Errorf("err = %v, want a --format error", err)
	}
}

func TestHookRun_WarnsWhenTheSyncedCommandDiffersFromTheSpec(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := hookRunProject(t, "name: guard\nevent: UserPromptSubmit\ncommand: 'echo synced'\n")
	mustSync(t)

	out, err := runHookRun(t, "guard", "--prompt", "hi")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	if strings.Contains(out, "warning:") {
		t.Errorf("a fresh sync warns:\n%s", out)
	}

	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "name: guard\nevent: UserPromptSubmit\ncommand: 'echo edited'\n")
	out, err = runHookRun(t, "guard", "--prompt", "hi")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	for _, want := range []string{
		`warning: .claude/settings.json has no UserPromptSubmit command "echo edited"; run agnostic-ai sync`,
		`warning: .codex/hooks.json has no UserPromptSubmit command "export AGNOSTIC_AI_TARGET=codex; echo edited"; run agnostic-ai sync`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
}

func TestHookRun_WarnsWhenTheNativeFileIsMissingOrBroken(t *testing.T) {
	skipWithoutPOSIXShell(t)
	dir := testutil.TempCwd(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [gemini]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"), "name: guard\nevent: BeforeAgent\ncommand: 'exit 0'\n")

	out, err := runHookRun(t, "guard", "--prompt", "hi", "--format", "json")
	if err != nil {
		t.Fatalf("err = %v\n%s", err, out)
	}
	r := decodeHookRunJSON(t, out)
	if w := r.Targets[0].Warnings; len(w) != 1 || !strings.Contains(w[0], ".gemini/settings.json does not exist; run agnostic-ai sync") {
		t.Errorf("warnings = %q, want the missing settings file named", w)
	}

	if err := os.MkdirAll(filepath.Join(dir, ".gemini"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(dir, ".gemini", "settings.json"), "{")
	out, _ = runHookRun(t, "guard", "--prompt", "hi")
	if !strings.Contains(out, "warning: .gemini/settings.json: ") {
		t.Errorf("output misses the parse warning:\n%s", out)
	}
}
