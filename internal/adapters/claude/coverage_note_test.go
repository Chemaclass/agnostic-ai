package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func swapNoteWarner(t *testing.T) *strings.Builder {
	t.Helper()
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	return buf
}

// Project rejections belong in settings, not in each .mcp.json server.
func TestEmit_MCP_DisabledUsesProjectRejection(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "disabled": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	fs := parsed["mcpServers"].(map[string]any)["fs"].(map[string]any)
	if _, ok := fs["disabled"]; ok {
		t.Errorf("claude has no file-based disable key; must not emit one: %s", raw)
	}

	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "`disabled`") {
		t.Errorf("unexpected field no-op note: %s", buf.String())
	}
	raw, err = os.ReadFile(".claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"disabledMcpjsonServers"`) || !strings.Contains(string(raw), `"fs"`) {
		t.Errorf("missing rejection: %s", raw)
	}
}

// "Only honored for hooks declared in skill frontmatter; ignored in
// settings files and agent frontmatter" (code.claude.com/docs/en/hooks).
// This adapter's only hook sink is `.claude/settings.json`, so `once`
// still emits (the bytes round-trip) but never takes effect there (#1078).
func TestEmit_Hook_OnceNotesFieldNoOp(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindHook, Name: "boot", Meta: map[string]any{
			"event": "SessionStart", "command": "echo hi", "once": true,
		}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(".claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"once": true`) {
		t.Errorf("expected once to still emit, got %s", raw)
	}

	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "once") {
		t.Errorf("expected a once field no-op note, got: %q", buf.String())
	}
}

func TestEmit_Hook_FailClosedNotesIgnoredFailuresAndPreservesNativeKey(t *testing.T) {
	cases := []struct {
		name       string
		event      string
		meta       map[string]any
		wantNote   bool
		wantAction string
	}{
		{"stop", "Stop", nil, true, "PreToolUse"},
		{"subagent stop", "SubagentStop", nil, true, "PreToolUse"},
		{"task completed", "TaskCompleted", nil, true, "PreToolUse"},
		{"teammate idle", "TeammateIdle", nil, true, "PreToolUse"},
		{"async", "PreToolUse", map[string]any{"async": true}, true, "synchronous"},
		{"async rewake", "PostToolUse", map[string]any{"asyncRewake": true}, true, "synchronous"},
		{"permission request", "PermissionRequest", nil, false, ""},
		{"pre tool use", "PreToolUse", nil, false, ""},
		{"false async flags", "PreToolUse", map[string]any{"async": false, "asyncRewake": false}, false, ""},
		{"http async field is not emitted", "PreToolUse", map[string]any{"type": "http", "url": "https://example.test/check", "async": true}, false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			testutil.TempCwd(t)
			buf := swapNoteWarner(t)
			meta := map[string]any{"event": c.event, "command": "guard.sh", "failClosed": true}
			for k, v := range c.meta {
				meta[k] = v
			}
			entry := spec.Entry{Kind: spec.KindHook, Name: "guard", Meta: meta}
			if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(".claude/settings.json")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `"onFailure": "block"`) {
				t.Errorf("onFailure must remain for import round trips: %s", raw)
			}
			emit.FlushCoverageNotes()
			note := buf.String()
			if got := strings.Contains(note, "`failClosed`"); got != c.wantNote {
				t.Errorf("failClosed note = %t, want %t: %s", got, c.wantNote, note)
			}
			if c.wantNote && !strings.Contains(note, c.wantAction) {
				t.Errorf("note must give an action containing %q: %s", c.wantAction, note)
			}
		})
	}
}

func TestEmit_Hook_FailClosedFalseDoesNotNoteIgnoredFailures(t *testing.T) {
	testutil.TempCwd(t)
	buf := swapNoteWarner(t)
	entry := spec.Entry{Kind: spec.KindHook, Name: "stop", Meta: map[string]any{
		"event": "Stop", "command": "guard.sh", "failClosed": false,
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle([]spec.Entry{entry}), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(".claude/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"onFailure"`) {
		t.Errorf("false failClosed must keep the native default: %s", raw)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "`failClosed`") {
		t.Errorf("false failClosed must not produce a note: %s", buf.String())
	}
}
