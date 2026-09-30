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
			}
		})
	}
}
