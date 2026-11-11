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
