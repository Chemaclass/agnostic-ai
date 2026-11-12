package hookrun

import (
	"reflect"
	"testing"
)

func TestBuildAugment_PromptStopSessionEndAndNotificationPayloads(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		event, trigger string
		in             Input
		fields         map[string]any
	}{
		{"PromptSubmit", "prompt", Input{Prompt: "deploy now"}, map[string]any{"user_prompt": "deploy now"}},
		{"Stop", "stop", Input{}, map[string]any{"agent_stop_cause": "end_turn"}},
		{"SessionEnd", "session", Input{}, map[string]any{}},
		{"Notification", "notification", Input{}, map[string]any{"notification_type": "info", "notification_message": ""}},
	} {
		t.Run(tc.event, func(t *testing.T) {
			p, err := Build("augment", tc.event, "", root, tc.in)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]any{
				"hook_event_name": tc.event, "hook_type": tc.event, "cwd": root,
				"conversation_id": SessionID, "workspace_roots": []any{root},
			}
			for k, v := range tc.fields {
				want[k] = v
			}
			if got := decode(t, p.Body); !reflect.DeepEqual(got, want) {
				t.Errorf("payload = %v, want %v", got, want)
			}
			if !p.Fires || p.Trigger != tc.trigger {
				t.Errorf("fires=%v trigger=%q, want true %q", p.Fires, p.Trigger, tc.trigger)
			}
		})
	}
}

func TestBuildAugment_CommonFieldsOnEveryBuiltEvent(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		event string
		in    Input
	}{
		{"PreToolUse", Input{Bash: "git push"}},
		{"PostToolUse", Input{Edit: "a.go"}},
		{"SessionStart", Input{}},
	} {
		doc := decode(t, mustBuild(t, "augment", tc.event, root, tc.in).Body)
		if doc["hook_type"] != tc.event || doc["cwd"] != root {
			t.Errorf("%s payload lacks hook_type or cwd: %v", tc.event, doc)
		}
	}
}

func TestBuildAugment_RefusesInputTheEventDoesNotTake(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name, event string
		in          Input
		want        string
	}{
		{"PromptSubmit without --prompt", "PromptSubmit", Input{}, "PromptSubmit needs --prompt <text> or --payload <file>"},
		{"--prompt on Stop", "Stop", Input{Prompt: "hi"}, "--prompt builds PromptSubmit, not Stop"},
		{"--bash on Notification", "Notification", Input{Bash: "ls"}, "--edit and --bash build tool events, not Notification"},
		{"an event Augment does not have", "UserPromptSubmit", Input{}, "hook run builds no augment UserPromptSubmit payload; pass --payload <file>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Build("augment", tc.event, "", root, tc.in); err == nil || err.Error() != tc.want {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuildAugment_PayloadFileWorksOnEveryEvent(t *testing.T) {
	for _, event := range []string{"PreToolUse", "PostToolUse", "Stop", "SessionStart", "SessionEnd", "PromptSubmit", "Notification"} {
		t.Run(event, func(t *testing.T) {
			body := []byte(`{"hook_event_name":"` + event + `","tool_name":"launch-process"}`)
			p, err := Build("augment", event, "", t.TempDir(), Input{Raw: body})
			if err != nil || !p.Fires || string(p.Body) != string(body) {
				t.Errorf("payload = %s fires=%v, %v", p.Body, p.Fires, err)
			}
		})
	}
}

func mustBuild(t *testing.T, target, event, root string, in Input) Payload {
	t.Helper()
	p, err := Build(target, event, "", root, in)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
