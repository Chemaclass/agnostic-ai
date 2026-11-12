package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func augmentHookProject(t *testing.T, event, extra string) {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [augment]\n")
	writeFile(t, ".agnostic-ai/hooks/probe.yaml", "name: probe\nevent: "+event+"\n"+extra+"command: echo probe\n")
}

func TestAugmentHooks_ValidateAcceptsPromptSubmitAndNotification(t *testing.T) {
	for _, event := range []string{"PromptSubmit", "Notification"} {
		t.Run(event, func(t *testing.T) {
			augmentHookProject(t, event, "")
			for _, cmd := range []string{"validate", "lint"} {
				out, err := runRoot(t, cmd)
				if err != nil || strings.Contains(out, "unknown hook event") {
					t.Errorf("%s rejected %s: %v\n%s", cmd, event, err, out)
				}
			}
		})
	}
}

func TestAugmentHooks_ValidateStillRejectsUnknownEvent(t *testing.T) {
	augmentHookProject(t, "UserPromptSubmit", "")
	out, err := runRoot(t, "validate")
	if err == nil || !strings.Contains(out, `unknown hook event "UserPromptSubmit"`) {
		t.Fatalf("expected an unknown-event failure, got %v\n%s", err, out)
	}
	for _, want := range []string{"Notification", "PromptSubmit"} {
		if !strings.Contains(out, want) {
			t.Errorf("supported list lacks %s:\n%s", want, out)
		}
	}
}

func TestAugmentHooks_SyncWritesPromptSubmitAndNotificationWithoutMatcher(t *testing.T) {
	for _, event := range []string{"PromptSubmit", "Notification"} {
		t.Run(event, func(t *testing.T) {
			augmentHookProject(t, event, "matcher: should-be-dropped\n")
			syncNotes(t)
			data, err := os.ReadFile(".augment/settings.json")
			if err != nil {
				t.Fatal(err)
			}
			got := string(data)
			if !strings.Contains(got, `"`+event+`"`) || !strings.Contains(got, "echo probe") {
				t.Errorf("event not written: %s", got)
			}
			if strings.Contains(got, `"matcher"`) {
				t.Errorf("%s takes no matcher, got %s", event, got)
			}
			syncNotes(t, "--check")
		})
	}
}

func TestAugmentHooks_ToolEventsKeepTheirMatcher(t *testing.T) {
	augmentHookProject(t, "PreToolUse", "matcher: launch-process\n")
	syncNotes(t)
	data, err := os.ReadFile(".augment/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"matcher": "launch-process"`) {
		t.Errorf("PreToolUse lost its matcher: %s", data)
	}
}

// Auggie has no UserPromptSubmit event. Its PromptSubmit passes the
// prompt as user_prompt and cannot block, so sync skips the hook with
// a note instead of renaming the event (#3348).
func TestAugmentHooks_SyncSkipsUserPromptSubmitAsValidateRejectsIt(t *testing.T) {
	augmentHookProject(t, "UserPromptSubmit", "targets: [augment]\n")
	if out, err := runRoot(t, "validate"); err == nil {
		t.Fatalf("validate accepted UserPromptSubmit for augment:\n%s", out)
	}
	notes := strings.Join(syncNotes(t), "\n")
	if !strings.Contains(notes, "PromptSubmit") || !strings.Contains(notes, "x-augment") {
		t.Errorf("sync note does not name the event to use:\n%s", notes)
	}
	if data, err := os.ReadFile(".augment/settings.json"); err == nil && strings.Contains(string(data), "UserPromptSubmit") {
		t.Errorf("sync wrote an event Auggie never fires: %s", data)
	}
	out, _ := runHookRun(t, "probe", "--prompt", "hi")
	if !strings.Contains(out, "augment: not run") || !strings.Contains(out, "PromptSubmit") {
		t.Errorf("hook run output = %s", out)
	}
}

func TestAugmentHooks_SyncReplacesAUserPromptSubmitEntryBesideAnotherHook(t *testing.T) {
	augmentHookProject(t, "UserPromptSubmit", "")
	writeFile(t, ".agnostic-ai/hooks/other.yaml", "name: other\nevent: Stop\ncommand: echo other\n")
	writeFile(t, ".augment/settings.json", `{
  "theme": "dark",
  "hooks": {
    "UserPromptSubmit": [{"hooks": [{"type": "command", "command": "echo probe"}]}]
  }
}`)
	syncNotes(t)
	data, err := os.ReadFile(".augment/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "UserPromptSubmit") || !strings.Contains(got, "echo other") {
		t.Errorf("hooks = %s, want only the Stop hook", got)
	}
	if !strings.Contains(got, `"theme"`) {
		t.Errorf("sync dropped the user's own setting: %s", got)
	}
	syncNotes(t, "--check")
}

func TestAugmentHooks_XAugmentEventOverrideSyncsValidatesAndRuns(t *testing.T) {
	skipWithoutPOSIXShell(t)
	augmentHookProject(t, "UserPromptSubmit", "x-augment:\n  event: PromptSubmit\n")
	if out, err := runRoot(t, "validate"); err != nil {
		t.Fatalf("validate rejected the x-augment event: %v\n%s", err, out)
	}
	syncNotes(t)
	data, err := os.ReadFile(".augment/settings.json")
	if err != nil || !strings.Contains(string(data), `"PromptSubmit"`) || strings.Contains(string(data), "UserPromptSubmit") {
		t.Fatalf("settings = %s, %v", data, err)
	}
	if out, err := runHookRun(t, "probe", "--prompt", "hi", "--expect", "allow"); err != nil || !strings.Contains(out, "augment: allow") {
		t.Errorf("hook run: %v\n%s", err, out)
	}
}

func TestValidate_HookScopedToATargetWithoutHooksChecksEveryTarget(t *testing.T) {
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, aider]\n")
	writeFile(t, ".agnostic-ai/hooks/probe.yaml", "name: probe\nevent: PreToolUse\ntargets: [aider]\ncommand: echo probe\n")
	if out, err := runRoot(t, "validate"); strings.Contains(out, "unknown hook event") {
		t.Errorf("validate: %v\n%s", err, out)
	}
}

func TestValidate_HookScopedToCursorSuggestsCursorsEvent(t *testing.T) {
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\n")
	writeFile(t, ".agnostic-ai/hooks/probe.yaml", "name: probe\nevent: PreToolUse\ntargets: [cursor]\ncommand: echo probe\n")
	out, err := runRoot(t, "validate")
	if err == nil || !strings.Contains(out, "did you mean preToolUse?") {
		t.Errorf("validate: %v\n%s", err, out)
	}
}

func TestValidate_QuotesTheXTargetEventThatDecided(t *testing.T) {
	augmentHookProject(t, "UserPromptSubmit", "x-augment:\n  event: PromptSubmt\n")
	out, err := runRoot(t, "validate")
	if err == nil || !strings.Contains(out, `"PromptSubmt"`) || strings.Contains(out, `"UserPromptSubmit"`) {
		t.Errorf("validate: %v\n%s", err, out)
	}
}

func TestAugmentHooks_ClaudeStyleMatcherNoteNamesTheAuggie1Tools(t *testing.T) {
	augmentHookProject(t, "PreToolUse", "matcher: Bash\n")
	notes := strings.Join(syncNotes(t), "\n")
	for _, want := range []string{"bash, read, edit, write", "launch-process"} {
		if !strings.Contains(notes, want) {
			t.Errorf("matcher note lacks %q:\n%s", want, notes)
		}
	}
}
