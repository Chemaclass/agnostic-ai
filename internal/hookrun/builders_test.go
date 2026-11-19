package hookrun

import (
	"path/filepath"
	"reflect"
	"testing"
)

type builderCase struct {
	name, event, matcher string
	in                   Input
	trigger              string
	fires                bool
	fields               map[string]any
}

// runBuilderCases builds each case under a temporary root. cases gets
// that root and the absolute path of --edit a.go under it.
func runBuilderCases(t *testing.T, target string, cases func(root, edit string) []builderCase) {
	t.Helper()
	root := t.TempDir()
	for _, tc := range cases(root, filepath.Join(root, "a.go")) {
		t.Run(tc.name, func(t *testing.T) {
			p, err := Build(target, tc.event, tc.matcher, root, tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if p.Trigger != tc.trigger || p.Fires != tc.fires {
				t.Errorf("trigger=%q fires=%v, want %q %v", p.Trigger, p.Fires, tc.trigger, tc.fires)
			}
			doc := decode(t, p.Body)
			for k, want := range tc.fields {
				if got, ok := doc[k]; !ok || !reflect.DeepEqual(got, want) {
					t.Errorf("%s = %#v, want %#v in %v", k, got, want, doc)
				}
			}
		})
	}
}

type builderError struct {
	name, event, matcher string
	in                   Input
	want                 string
}

func runBuilderErrors(t *testing.T, target string, cases []builderError) {
	t.Helper()
	root := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Build(target, tc.event, tc.matcher, root, tc.in); err == nil || err.Error() != tc.want {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestBuildCursor_EventsAndTheirFields(t *testing.T) {
	t.Parallel()
	runBuilderCases(t, "cursor", func(root, edit string) []builderCase {
		return []builderCase{
			{"afterShellExecution", "afterShellExecution", "", Input{Bash: "git push"}, "git push", true,
				map[string]any{"command": "git push", "sandbox": false, "output": "", "duration": float64(0)}},
			{"afterShellExecution with another command", "afterShellExecution", "^npm", Input{Bash: "git push"}, "git push", false, nil},
			{"postToolUse", "postToolUse", "", Input{Bash: "ls"}, "Shell", true,
				map[string]any{"tool_name": "Shell", "tool_output": "", "duration": float64(0), "cwd": root}},
			{"afterFileEdit", "afterFileEdit", "", Input{Edit: "a.go"}, "Write", true,
				map[string]any{"file_path": edit, "edits": []any{map[string]any{"old_string": "", "new_string": ""}}}},
			{"afterFileEdit with a Read matcher", "afterFileEdit", "Read", Input{Edit: "a.go"}, "Write", false, nil},
			{"beforeSubmitPrompt", "beforeSubmitPrompt", "", Input{Prompt: "deploy"}, "prompt", true,
				map[string]any{"prompt": "deploy", "attachments": []any{}}},
			{"sessionStart fires whatever the matcher", "sessionStart", "nothing", Input{}, "session", true,
				map[string]any{"session_id": SessionID, "is_background_agent": false, "composer_mode": "agent"}},
		}
	})
}

func TestBuildCursor_RefusesMissingInputUnknownEventsAndBadMatchers(t *testing.T) {
	t.Parallel()
	runBuilderErrors(t, "cursor", []builderError{
		{"afterShellExecution without --bash", "afterShellExecution", "", Input{}, "afterShellExecution needs --bash <command> or --payload <file>"},
		{"postToolUse without --bash", "postToolUse", "", Input{}, "postToolUse needs --bash <command> or --payload <file>"},
		{"afterFileEdit without --edit", "afterFileEdit", "", Input{}, "afterFileEdit needs --edit <path> or --payload <file>"},
		{"beforeSubmitPrompt without --prompt", "beforeSubmitPrompt", "", Input{}, "beforeSubmitPrompt needs --prompt <text> or --payload <file>"},
		{"an event hook run does not build", "stop", "", Input{}, "hook run builds no cursor stop payload; pass --payload <file>"},
		{"a matcher that does not compile", "afterShellExecution", "(", Input{Bash: "ls"}, "matcher \"(\"\nerror parsing regexp: missing closing ): `(`"},
	})
}

func TestBuildGoose_EventsAndTheirFields(t *testing.T) {
	t.Parallel()
	runBuilderCases(t, "goose", func(root, edit string) []builderCase {
		shell := map[string]any{"tool_name": "shell", "tool_input": map[string]any{"command": "git push"}, "working_dir": root, "matcher_context": "git push"}
		file := map[string]any{"working_dir": root, "matcher_context": edit}
		return []builderCase{
			{"BeforeShellExecution", "BeforeShellExecution", "", Input{Bash: "git push"}, "git push", true, shell},
			{"AfterShellExecution", "AfterShellExecution", "", Input{Bash: "git push"}, "git push", true, shell},
			{"BeforeShellExecution with another command", "BeforeShellExecution", "^npm", Input{Bash: "git push"}, "git push", false, nil},
			{"BeforeReadFile", "BeforeReadFile", "", Input{Edit: "a.go"}, edit, true, file},
			{"AfterFileEdit", "AfterFileEdit", "", Input{Edit: "a.go"}, edit, true, file},
			{"SessionStart", "SessionStart", "", Input{}, "session", true, map[string]any{"event": "SessionStart", "matcher_context": ""}},
			{"SessionStart with a matcher", "SessionStart", "startup", Input{}, "session", false, nil},
			{"PreToolUse edit picks write first", "PreToolUse", "", Input{Edit: "a.go"}, "write", true,
				map[string]any{"tool_name": "write", "tool_input": map[string]any{"path": edit, "content": ""}}},
		}
	})
}

func TestBuildGoose_RefusesMissingInput(t *testing.T) {
	t.Parallel()
	runBuilderErrors(t, "goose", []builderError{
		{"BeforeShellExecution without --bash", "BeforeShellExecution", "", Input{}, "BeforeShellExecution needs --bash <command> or --payload <file>"},
		{"AfterFileEdit without --edit", "AfterFileEdit", "", Input{}, "AfterFileEdit needs --edit <path> or --payload <file>"},
	})
}

func TestBuildCopilot_EventsAndTheirFields(t *testing.T) {
	t.Parallel()
	runBuilderCases(t, "copilot", func(root, edit string) []builderCase {
		return []builderCase{
			{"sessionStart", "sessionStart", "", Input{}, "session", true,
				map[string]any{"sessionId": SessionID, "source": "startup", "cwd": root}},
			{"SessionStart", "SessionStart", "", Input{}, "session", true,
				map[string]any{"session_id": SessionID, "hook_event_name": "SessionStart", "source": "startup"}},
			{"userPromptSubmitted", "userPromptSubmitted", "", Input{Prompt: "deploy"}, "prompt", true,
				map[string]any{"sessionId": SessionID, "prompt": "deploy"}},
			{"UserPromptSubmit", "UserPromptSubmit", "", Input{Prompt: "deploy"}, "prompt", true,
				map[string]any{"session_id": SessionID, "prompt": "deploy"}},
			{"postToolUse", "postToolUse", "", Input{Bash: "ls"}, "bash", true,
				map[string]any{"toolName": "bash", "toolArgs": `{"command":"ls"}`, "toolResult": map[string]any{"resultType": "success", "textResultForLlm": ""}}},
			{"postToolUse with another tool", "postToolUse", "edit", Input{Bash: "ls"}, "bash", false, nil},
		}
	})
}

// PascalCase PostToolUse is refused before the result is written, so no
// payload carries the snake_case tool_result spelling.
func TestBuildCopilot_RefusesMissingInputEditsAndPascalPostToolUse(t *testing.T) {
	t.Parallel()
	runBuilderErrors(t, "copilot", []builderError{
		{"userPromptSubmitted without --prompt", "userPromptSubmitted", "", Input{}, "userPromptSubmitted needs --prompt <text> or --payload <file>"},
		{"postToolUse without --bash", "postToolUse", "", Input{}, "postToolUse needs --bash <command> or --payload <file>"},
		{"preToolUse with --edit", "preToolUse", "", Input{Edit: "a.go"}, "Copilot documents no toolArgs for edit, create, or apply_patch; pass --payload <file>"},
		{"PostToolUse", "PostToolUse", "", Input{Bash: "ls"}, "Copilot documents the Claude tool name on PreToolUse only, not the tool_name of PostToolUse; pass --payload <file>"},
	})
}

func TestBuildTrae_EventsAndTheirFields(t *testing.T) {
	t.Parallel()
	runBuilderCases(t, "trae", func(root, edit string) []builderCase {
		return []builderCase{
			{"UserPromptSubmit", "UserPromptSubmit", "", Input{Prompt: "deploy"}, "prompt", true,
				map[string]any{"prompt": "deploy", "hook_event_name": "UserPromptSubmit", "cwd": root}},
			{"SessionStart", "SessionStart", "", Input{}, "session", true,
				map[string]any{"hook_event_name": "SessionStart", "workspace_roots": []any{root}}},
			{"PostToolUse", "PostToolUse", "", Input{Bash: "ls"}, "RunCommand", true,
				map[string]any{"tool_name": "RunCommand", "tool_input": map[string]any{"command": "ls"}, "tool_response": map[string]any{}}},
			{"a matcher that does not compile does not fire", "PreToolUse", "(", Input{Bash: "ls"}, "RunCommand", false, nil},
		}
	})
}

func TestBuildTrae_RefusesEditsAndEventsItDoesNotBuild(t *testing.T) {
	t.Parallel()
	runBuilderErrors(t, "trae", []builderError{
		{"PreToolUse with --edit", "PreToolUse", "", Input{Edit: "a.go"}, "Trae documents no tool_input for its edit tools; pass --payload <file>"},
		{"an event hook run does not build", "Stop", "", Input{}, "hook run builds no trae Stop payload; pass --payload <file>"},
	})
}
