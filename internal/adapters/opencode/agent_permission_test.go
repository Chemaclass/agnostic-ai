package opencode

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func emitAgent(t *testing.T, meta map[string]any) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	entry, problem := spec.Entry{Kind: spec.KindAgent, Name: "reviewer", Meta: meta, Body: "Review."}.NativeTools()
	if problem != "" {
		t.Fatalf("capability problem: %s", problem)
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	return readFile(t, filepath.Join(dir, ".opencode/agents/reviewer.md"))
}

func TestEmit_Agent_CanReadBecomesReadOnlyPermission(t *testing.T) {
	got := emitAgent(t, map[string]any{"description": "d", "can": []any{"read"}})
	want := "permission:\n  \"*\": deny\n  read: allow\n"
	if !strings.Contains(got, want) {
		t.Errorf("want read-only permission map %q in:\n%s", want, got)
	}
}

func TestEmit_Agent_ScopedToolPutsCatchAllDenyFirst(t *testing.T) {
	got := emitAgent(t, map[string]any{"description": "d", "can": []any{"read", "shell(git diff *)"}})
	want := "  bash:\n    \"*\": deny\n    git diff *: allow\n"
	if !strings.Contains(got, want) {
		t.Errorf("want scoped bash map %q in:\n%s", want, got)
	}
}

func TestEmit_Agent_XOpencodePermissionWinsForItsKeys(t *testing.T) {
	got := emitAgent(t, map[string]any{
		"description": "d",
		"tools":       []any{"Read", "Bash"},
		"x-opencode":  map[string]any{"permission": map[string]any{"bash": "ask"}},
	})
	for _, want := range []string{"\"*\": deny", "bash: ask", "read: allow"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "bash: allow") {
		t.Errorf("x-opencode.permission must win for bash:\n%s", got)
	}
}

func TestEmit_Agent_XOpencodeScalarPermissionWinsOutright(t *testing.T) {
	got := emitAgent(t, map[string]any{
		"description": "d",
		"tools":       []any{"Read"},
		"x-opencode":  map[string]any{"permission": "ask"},
	})
	if !strings.Contains(got, "permission: ask\n") || strings.Contains(got, "read: allow") {
		t.Errorf("scalar x-opencode.permission must win outright:\n%s", got)
	}
}

func TestEmit_Agent_UnknownToolPrintsCoverageNote(t *testing.T) {
	buf := swapNoteWarner(t)
	got := emitAgent(t, map[string]any{"description": "d", "tools": []any{"Read", "NotebookEdit"}})
	if !strings.Contains(got, "read: allow") {
		t.Errorf("known tool must still translate:\n%s", got)
	}
	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "opencode") || !strings.Contains(buf.String(), "tools") {
		t.Errorf("want a coverage note for the unmapped tool, got: %q", buf.String())
	}
}

func TestEmit_Agent_OnlyUnknownToolsKeepsDefaultPermissions(t *testing.T) {
	got := emitAgent(t, map[string]any{"description": "d", "tools": []any{"NotebookEdit"}})
	if strings.Contains(got, "permission:") {
		t.Errorf("an untranslatable list must not lock the agent out:\n%s", got)
	}
}
