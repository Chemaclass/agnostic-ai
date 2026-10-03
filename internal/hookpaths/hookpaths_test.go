package hookpaths

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRead_ClaudeEditToolsReportTheirFile(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		want    []Change
	}{
		{
			name:    "Edit",
			payload: `{"cwd":"/p","hook_event_name":"PostToolUse","tool_name":"Edit","tool_input":{"file_path":"/p/src/a.go","old_string":"a","new_string":"b"}}`,
			want:    []Change{{Action: ActionUpdate, Path: "/p/src/a.go"}},
		},
		{
			name:    "Write",
			payload: `{"cwd":"/p","tool_name":"Write","tool_input":{"file_path":"/p/new.go","content":"package x"}}`,
			want:    []Change{{Action: ActionUpdate, Path: "/p/new.go"}},
		},
		{
			name:    "MultiEdit",
			payload: `{"cwd":"/p","tool_name":"MultiEdit","tool_input":{"file_path":"/p/b.go","edits":[{"old_string":"a","new_string":"b"}]}}`,
			want:    []Change{{Action: ActionUpdate, Path: "/p/b.go"}},
		},
		{
			name:    "NotebookEdit",
			payload: `{"cwd":"/p","tool_name":"NotebookEdit","tool_input":{"notebook_path":"/p/n.ipynb","new_source":"x"}}`,
			want:    []Change{{Action: ActionUpdate, Path: "/p/n.ipynb"}},
		},
		{
			name:    "Bash is no edit",
			payload: `{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"*** Update File: a.go"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Read("claude", []byte(tc.payload))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got.Changes, tc.want) {
				t.Errorf("Changes = %#v, want %#v", got.Changes, tc.want)
			}
			if got.Cwd != "/p" {
				t.Errorf("Cwd = %q, want /p", got.Cwd)
			}
		})
	}
}

func TestRead_CodexApplyPatchReportsEveryFile(t *testing.T) {
	payload := `{"cwd":"/p","hook_event_name":"PostToolUse","tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch\n*** Add File: docs/new.md\n+hi\n*** Update File: src/a.go\n*** Move to: src/b.go\n@@\n-a\n+b\n*** Update File: src/c.go\n@@\n-c\n+d\n*** Delete File: old.go\n*** End Patch\n"}}`

	got, err := Read("codex", []byte(payload))
	if err != nil {
		t.Fatal(err)
	}
	want := []Change{
		{Action: ActionAdd, Path: "docs/new.md"},
		{Action: ActionDelete, Path: "src/a.go"},
		{Action: ActionMove, Path: "src/b.go", From: "src/a.go"},
		{Action: ActionUpdate, Path: "src/c.go"},
		{Action: ActionDelete, Path: "old.go"},
	}
	if !reflect.DeepEqual(got.Changes, want) {
		t.Errorf("Changes = %#v, want %#v", got.Changes, want)
	}
}

func TestRead_CodexIgnoresABashCommand(t *testing.T) {
	got, err := Read("codex", []byte(`{"cwd":"/p","tool_name":"Bash","tool_input":{"command":"echo '*** Delete File: a.go'"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Changes) != 0 {
		t.Errorf("Changes = %#v, want none", got.Changes)
	}
}

func TestRead_OtherTargetsReportTheirEditedFile(t *testing.T) {
	for _, tc := range []struct {
		target  string
		payload string
		want    Payload
	}{
		{
			target:  "cursor",
			payload: `{"hook_event_name":"afterFileEdit","workspace_roots":["/p"],"file_path":"/p/a.go","edits":[{"old_string":"a","new_string":"b"}]}`,
			want:    Payload{Changes: []Change{{Action: ActionUpdate, Path: "/p/a.go"}}},
		},
		{
			target:  "cursor",
			payload: `{"hook_event_name":"beforeReadFile","file_path":"/p/a.go","content":"x"}`,
		},
		{
			target:  "gemini",
			payload: `{"cwd":"/p","hook_event_name":"AfterTool","tool_name":"write_file","tool_input":{"file_path":"a.go","content":"x"}}`,
			want:    Payload{Cwd: "/p", Changes: []Change{{Action: ActionUpdate, Path: "a.go"}}},
		},
		{
			target:  "gemini",
			payload: `{"cwd":"/p","tool_name":"replace","tool_input":{"file_path":"/p/b.go","old_string":"a","new_string":"b"}}`,
			want:    Payload{Cwd: "/p", Changes: []Change{{Action: ActionUpdate, Path: "/p/b.go"}}},
		},
		{
			target:  "gemini",
			payload: `{"cwd":"/p","tool_name":"read_file","tool_input":{"file_path":"/p/b.go"}}`,
			want:    Payload{Cwd: "/p"},
		},
		{
			target:  "factory",
			payload: `{"cwd":"/p","hook_event_name":"PreToolUse","tool_name":"Create","tool_input":{"file_path":"/p/file.txt","content":"x"}}`,
			want:    Payload{Cwd: "/p", Changes: []Change{{Action: ActionAdd, Path: "/p/file.txt"}}},
		},
		{
			target:  "factory",
			payload: `{"cwd":"/p","tool_name":"Edit","tool_input":{"file_path":"/p/file.txt"}}`,
			want:    Payload{Cwd: "/p", Changes: []Change{{Action: ActionUpdate, Path: "/p/file.txt"}}},
		},
		{
			target:  "qoder",
			payload: `{"cwd":"/p","hook_event_name":"PostToolUse","tool_name":"Write","tool_input":{"file_path":"/p/file.ts","content":"x"}}`,
			want:    Payload{Cwd: "/p", Changes: []Change{{Action: ActionUpdate, Path: "/p/file.ts"}}},
		},
		{
			target:  "qoder",
			payload: `{"cwd":"/p","tool_name":"Read","tool_input":{"file_path":"/p/file.ts"}}`,
			want:    Payload{Cwd: "/p"},
		},
		{
			target:  "augment",
			payload: `{"hook_event_name":"PostToolUse","tool_name":"remove-files","tool_input":{},"file_changes":[{"path":"src/new.ts","changeType":"create"},{"path":"src/auth.ts","changeType":"edit"},{"path":"src/old.ts","changeType":"delete"}]}`,
			want: Payload{Changes: []Change{
				{Action: ActionAdd, Path: "src/new.ts"},
				{Action: ActionUpdate, Path: "src/auth.ts"},
				{Action: ActionDelete, Path: "src/old.ts"},
			}},
		},
		{
			target:  "augment",
			payload: `{"hook_event_name":"PreToolUse","tool_name":"str-replace-editor","tool_input":{"path":"src/auth.ts","old_str_1":"a","new_str_1":"b"}}`,
			want:    Payload{Changes: []Change{{Action: ActionUpdate, Path: "src/auth.ts"}}},
		},
	} {
		t.Run(tc.target, func(t *testing.T) {
			got, err := Read(tc.target, []byte(tc.payload))
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("Read() = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestRead_IgnoresOtherToolsWhateverTheirInputShape(t *testing.T) {
	for _, payload := range []string{
		`{"tool_name":"mcp__db__query","tool_input":{"path":3,"file_path":["a"],"command":["ls","-l"]}}`,
		`{"tool_name":"Task","tool_input":"free text"}`,
		`{"tool_name":"Bash","tool_input":null}`,
		"",
		" \n",
	} {
		for _, target := range Targets() {
			got, err := Read(target, []byte(payload))
			if err != nil {
				t.Errorf("Read(%s, %q) error = %v", target, payload, err)
			}
			if len(got.Changes) != 0 {
				t.Errorf("Read(%s, %q) = %#v, want no changes", target, payload, got.Changes)
			}
		}
	}
}

func TestRead_RejectsAnUnknownTargetAndBadJSON(t *testing.T) {
	// Devin CLI (the windsurf target) documents no edit tool input.
	for _, target := range []string{"aider", "windsurf"} {
		if _, err := Read(target, []byte(`{}`)); !errors.Is(err, ErrUnsupportedTarget) {
			t.Errorf("Read(%s) error = %v, want ErrUnsupportedTarget", target, err)
		}
	}
	if _, err := Read("claude", []byte(`not json`)); err == nil {
		t.Error("Read(bad JSON) returned no error")
	}
	if _, err := Read("claude", []byte(`{"tool_name":"Edit","tool_input":"a.go"}`)); err == nil {
		t.Error("Read(Edit with a string tool_input) returned no error")
	}
}

func TestGuessTarget_ReadsAClaudeShapedEditWithoutATarget(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    string
	}{
		{`{"tool_name":"Edit","tool_input":{"file_path":"/p/a.go"}}`, "claude"},
		{`{"tool_name":"MultiEdit","tool_input":{"edits":[]}}`, "claude"},
		{`{"tool_name":"NotebookEdit","tool_input":{"notebook_path":"/p/n.ipynb"}}`, "claude"},
		{`{"tool_name":"apply_patch","tool_input":{"command":"*** Begin Patch"}}`, ""},
		{`{"tool_name":"Edit","tool_input":"a.go"}`, ""},
		{`{"file_path":"/p/a.go"}`, ""},
		{`not json`, ""},
	} {
		if got := GuessTarget([]byte(tc.payload)); got != tc.want {
			t.Errorf("GuessTarget(%s) = %q, want %q", tc.payload, got, tc.want)
		}
	}
}

func TestRelative_ResolvesAgainstTheHookCwdAndPrintsFromTheRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	p := Payload{
		Cwd: filepath.Join(root, "sub"),
		Changes: []Change{
			{Action: ActionUpdate, Path: filepath.Join(root, "src", "a.go")},
			{Action: ActionMove, Path: "b.go", From: "a.go"},
			{Action: ActionUpdate, Path: filepath.Join(outside, "x.go")},
		},
	}

	got := p.Relative(root)
	want := []Change{
		{Action: ActionUpdate, Path: filepath.Join("src", "a.go")},
		{Action: ActionMove, Path: filepath.Join("sub", "b.go"), From: filepath.Join("sub", "a.go")},
		{Action: ActionUpdate, Path: filepath.Join(outside, "x.go")},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Relative() = %#v, want %#v", got, want)
	}
}

func TestRelative_ResolvesARootNamedThroughASymlink(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "project")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink: %v", err)
	}
	p := Payload{Changes: []Change{
		{Action: ActionAdd, Path: filepath.Join(real, "new", "dir", "a.go")},
		{Action: ActionDelete, Path: filepath.Join(real, "gone.go")},
	}}

	got := p.Relative(link)
	want := []Change{
		{Action: ActionAdd, Path: filepath.Join("new", "dir", "a.go")},
		{Action: ActionDelete, Path: "gone.go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Relative() = %#v, want %#v", got, want)
	}
}

func TestRelative_UsesTheRootWhenThePayloadHasNoCwd(t *testing.T) {
	got := Payload{Changes: []Change{{Action: ActionAdd, Path: "a.go"}}}.Relative(t.TempDir())
	if want := []Change{{Action: ActionAdd, Path: "a.go"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Relative() = %#v, want %#v", got, want)
	}
}

func TestRead_QoderRefusesEditBecauseItsInputIsUndocumented(t *testing.T) {
	_, err := Read("qoder", []byte(`{"tool_name":"Edit","tool_input":{"file_path":"/p/a.ts"}}`))
	if err == nil || !strings.Contains(err.Error(), "Qoder docs") {
		t.Errorf("Read(qoder Edit) error = %v, want a refusal naming the docs", err)
	}
}
