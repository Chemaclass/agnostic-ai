package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestAcceptsHook(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
		want string
	}{
		{"bash tool", map[string]any{"event": "PreToolUse", "matcher": "Bash"}, ""},
		{"edit aliases", map[string]any{"event": "PostToolUse", "matcher": "Edit|Write"}, ""},
		{"mcp tool", map[string]any{"event": "PreToolUse", "matcher": "mcp__fs__read"}, ""},
		{"session source", map[string]any{"event": "SessionStart", "matcher": "startup"}, ""},
		{"no matcher", map[string]any{"event": "Stop"}, ""},
		{"anchored group", map[string]any{"event": "PreToolUse", "matcher": "^(Bash|exec)$"}, ""},
		{"anchored group of edits", map[string]any{"event": "PostToolUse", "matcher": "^(Edit|Write)$"}, ""},
		{"anchored single tool", map[string]any{"event": "PreToolUse", "matcher": "^Bash$"}, ""},
		{"group of one", map[string]any{"event": "PreToolUse", "matcher": "(Bash)"}, ""},
		{"non-capturing group", map[string]any{"event": "PreToolUse", "matcher": "(?:Bash|apply_patch)"}, ""},
		{"emitted union", map[string]any{"event": "PreToolUse", "matcher": "(?:^(Bash|exec)$)|(?:^(Bash|apply_patch)$)"}, ""},
		{"mcp regex", map[string]any{"event": "PreToolUse", "matcher": "mcp__fs__.*"}, ""},
		{"source regex", map[string]any{"event": "SessionStart", "matcher": "^(startup|resume)$"}, ""},
		{"anchored group names no Codex tool", map[string]any{"event": "PreToolUse", "matcher": "^(Grep|Read)$"}, `does not match "^(Grep|Read)$"`},
		{"group with one Codex tool", map[string]any{"event": "PreToolUse", "matcher": "(Bash|Read)"}, ""},
		{"plain list keeps every name", map[string]any{"event": "PreToolUse", "matcher": "Bash|Read"}, `does not match "Read"`},
		{"claude-only tool", map[string]any{"event": "PreToolUse", "matcher": "Read"}, `does not match "Read"`},
		{"claude-only event", map[string]any{"event": "Notification"}, "no Notification event"},
		{"http handler", map[string]any{"event": "Stop", "type": "http"}, "no http handler"},
		{"if filter", map[string]any{"event": "PreToolUse", "matcher": "Bash", "if": "Bash(git *)"}, "no if field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Adapter{}.AcceptsHook(tc.meta)
			if tc.want == "" && got != "" {
				t.Errorf("got %q, want accepted", got)
			}
			if tc.want != "" && !strings.Contains(got, tc.want) {
				t.Errorf("got %q, want it to contain %q", got, tc.want)
			}
		})
	}
}

func editHook(command string) spec.Entry {
	return spec.Entry{Kind: spec.KindHook, Name: "guard", Path: ".agnostic-ai/hooks/guard.yaml",
		Meta: map[string]any{"event": "PostToolUse", "matcher": "Edit|Write", "command": command}}
}

// Codex sends an edit's patch in tool_input.command, so an edit hook
// reading tool_input.file_path gets nothing there (#1328).
func TestEmit_NotesEditHookReadingFilePath(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	buf := swapWarner(t)
	hooks := []spec.Entry{
		editHook("jq -r '.tool_input.file_path // empty'"),
		{Kind: spec.KindHook, Name: "bash", Meta: map[string]any{"event": "PreToolUse", "matcher": "Bash", "command": "jq .tool_input.file_path"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(hooks), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	out := buf.String()
	if !strings.Contains(out, "`tool_input.file_path` on 1 hook") || !strings.Contains(out, "tool_input.command") {
		t.Errorf("want one edit-payload note, got:\n%s", out)
	}
	if !strings.Contains(out, "agnostic-ai hook paths") {
		t.Errorf("note does not suggest agnostic-ai hook paths:\n%s", out)
	}
}

func TestEmit_EditHookPayloadErrorsUnderOnUnsupportedError(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	swapWarner(t)
	cfg := &config.Config{OnUnsupported: emit.OnUnsupportedError}
	err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{editHook(`python3 -c 'print(d["tool_input"]["file_path"])'`)}), cfg, false)
	if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/hooks/guard.yaml") {
		t.Fatalf("err = %v, want one naming the hook", err)
	}
}

func TestEmit_EditHookReadingCommandHasNoNote(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	buf := swapWarner(t)
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{editHook("jq -r .tool_input.command")}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "file_path") {
		t.Errorf("unexpected note:\n%s", buf.String())
	}
}

func TestEmit_NonCommandHookDoesNotInspectUnusedScript(t *testing.T) {
	for _, kind := range []string{"http", "prompt", "mcp_tool"} {
		t.Run(kind, func(t *testing.T) {
			testutil.TempCwd(t)
			notes := swapWarner(t)
			hook := editHook(".agnostic-ai/scripts/unused.sh")
			hook.Meta["type"] = kind
			hook.Meta["server"] = "tools"
			hook.Meta["tool"] = "inspect"
			hook.Meta["url"] = "http://localhost/hook"
			hook.Meta["prompt"] = "Inspect the event."
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{}, false); err != nil {
				t.Fatalf("unused command in %s hook caused an error: %v", kind, err)
			}
			emit.FlushCoverageNotes()
			if strings.Contains(notes.String(), "tool_input.file_path") {
				t.Errorf("unused command produced a payload note: %s", notes)
			}
			if _, err := os.Stat(".codex/hooks/unused.sh"); !os.IsNotExist(err) {
				t.Errorf("unused command materialized a script: %v", err)
			}
		})
	}
}

func swapWarner(t *testing.T) *strings.Builder {
	t.Helper()
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	return buf
}

func TestEmit_EditPayloadChecksMaterializedScripts(t *testing.T) {
	for _, tc := range []struct {
		name, source, codex, command string
		unmanaged, note              bool
	}{
		{name: "source stash", source: "jq -r .tool_input.file_path", command: ".claude/hooks/guard.sh", note: true},
		{name: "target stash", source: "jq -r .tool_input.command", codex: "jq -r .tool_input.file_path", command: ".claude/hooks/guard.sh", note: true},
		{name: "safe target override", source: "jq -r .tool_input.file_path", codex: "jq -r .tool_input.command", command: ".claude/hooks/guard.sh"},
		{name: "user owned", source: "jq -r .tool_input.file_path", command: ".claude/hooks/guard.sh", unmanaged: true},
		{name: "unrelated command", source: "jq -r .tool_input.file_path", command: "./custom/guard.sh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			for tool, body := range map[string]string{"claude": tc.source, "codex": tc.codex} {
				if body == "" {
					continue
				}
				path := filepath.Join(".agnostic-ai", "scripts", tool, "guard.sh")
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(body), 0755); err != nil {
					t.Fatal(err)
				}
			}
			notes := swapWarner(t)
			sess := emit.NewSession()
			if tc.unmanaged {
				sess.SetUnmanaged([]string{".codex/hooks/guard.sh"})
			}
			err := New().Emit(sess, spec.NewBundle([]spec.Entry{editHook(tc.command)}), &config.Config{}, false)
			if err != nil {
				t.Fatal(err)
			}
			emit.FlushCoverageNotes()
			if got := strings.Contains(notes.String(), "tool_input.file_path"); got != tc.note {
				t.Errorf("payload note = %v want %v: %s", got, tc.note, notes)
			}
			if tc.note {
				body, err := os.ReadFile(".codex/hooks/guard.sh")
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(string(body), "tool_input.file_path") {
					t.Errorf("checked another script than emitted: %s", body)
				}
				err = New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{editHook(tc.command)}), &config.Config{OnUnsupported: "error"}, false)
				if err == nil || !strings.Contains(err.Error(), "guard.yaml") || !strings.Contains(err.Error(), "apply_patch") {
					t.Errorf("missing named payload error: %v", err)
				}
				if !strings.Contains(notes.String(), "agnostic-ai hook paths") {
					t.Errorf("script note does not suggest agnostic-ai hook paths: %s", notes)
				}
			}
		})
	}
}

func TestEmit_EditPayloadChecksExactNeutralScript(t *testing.T) {
	for _, tc := range []struct {
		name, filename, command, shared, override string
		args                                      []any
		unmanaged, note                           bool
	}{
		{name: "quoted shared", filename: "my guard.sh", command: "sh '.agnostic-ai/scripts/my guard.sh'", shared: "jq -r .tool_input.file_path", note: true},
		{name: "unsafe target override", filename: "my guard.sh", command: `sh ".agnostic-ai/scripts/my guard.sh"`, shared: "jq -r .tool_input.command", override: "jq -r .tool_input.file_path", note: true},
		{name: "safe target override", filename: "my guard.sh", command: "sh '.agnostic-ai/scripts/my guard.sh'", shared: "jq -r .tool_input.file_path", override: "jq -r .tool_input.command"},
		{name: "exec form", filename: "my guard;script.sh", command: ".agnostic-ai/scripts/my guard;script.sh", args: []any{"--strict"}, shared: "jq -r .tool_input.file_path", note: true},
		{name: "two references one note", filename: "guard.sh", command: ".agnostic-ai/scripts/guard.sh && .agnostic-ai/scripts/guard.sh", shared: "jq -r .tool_input.file_path", note: true},
		{name: "user owned", filename: "my guard.sh", command: "sh '.agnostic-ai/scripts/my guard.sh'", shared: "jq -r .tool_input.file_path", unmanaged: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			for dir, body := range map[string]string{".agnostic-ai/scripts": tc.shared, ".agnostic-ai/scripts/codex": tc.override} {
				if body == "" {
					continue
				}
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, tc.filename), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			notes := swapWarner(t)
			hook := editHook(tc.command)
			if len(tc.args) > 0 {
				hook.Meta["args"] = tc.args
			}
			sess := emit.NewSession()
			path := ".codex/hooks/" + tc.filename
			if tc.unmanaged {
				sess.SetUnmanaged([]string{path})
			}
			if err := New().Emit(sess, spec.NewBundle([]spec.Entry{hook}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			emit.FlushCoverageNotes()
			wantNotes := 0
			if tc.note {
				wantNotes = 1
			}
			if got := strings.Count(notes.String(), "`tool_input.file_path` on 1 hook"); got != wantNotes {
				t.Errorf("payload notes = %d, want %d: %s", got, wantNotes, notes)
			}
			if !tc.unmanaged {
				body, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				wantBody := tc.shared
				if tc.override != "" {
					wantBody = tc.override
				}
				if string(body) != wantBody {
					t.Errorf("copied script = %q, want %q", body, wantBody)
				}
			}
			if !tc.unmanaged {
				err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{hook}), &config.Config{OnUnsupported: "error"}, false)
				if tc.note && (err == nil || !strings.Contains(err.Error(), "guard.yaml") || !strings.Contains(err.Error(), "apply_patch")) {
					t.Errorf("missing named payload error: %v", err)
				} else if !tc.note && err != nil {
					t.Errorf("safe selected script rejected: %v", err)
				}
			}
		})
	}
}
