package qwen

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmitAgents_WritesBackgroundAndHooks(t *testing.T) {
	testutil.TempCwd(t)
	hooks := map[string]any{"PreToolUse": []any{map[string]any{
		"matcher": "Bash",
		"hooks":   []any{map[string]any{"type": "command", "command": "./guard.sh"}},
	}}}
	err := New().EmitAgents(emit.NewSession(), []spec.Entry{{
		Kind: spec.KindAgent, Name: "rev",
		Meta: map[string]any{"description": "Reviewer", "background": true, "hooks": hooks},
		Body: "Review.",
	}}, ".qwen/agents", false)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	raw, err := os.ReadFile(".qwen/agents/rev.md")
	if err != nil {
		t.Fatal(err)
	}
	entry, err := spec.ParseMarkdownBytes(spec.KindAgent, raw)
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, raw)
	}
	if entry.Meta["background"] != true {
		t.Errorf("background missing:\n%s", raw)
	}
	if !reflect.DeepEqual(entry.Meta["hooks"], hooks) {
		t.Errorf("hooks = %#v, want %#v\n%s", entry.Meta["hooks"], hooks, raw)
	}
}

func TestEmitAgents_SharedStopHookBecomesSubagentStop(t *testing.T) {
	testutil.TempCwd(t)
	stop := []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "./done.sh"}}}}
	native := []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "./native.sh"}}}}
	err := New().EmitAgents(emit.NewSession(), []spec.Entry{
		{Kind: spec.KindAgent, Name: "shared", Meta: map[string]any{"description": "d", "hooks": map[string]any{"Stop": stop}}},
		{Kind: spec.KindAgent, Name: "native", Meta: map[string]any{"description": "d", "x-qwen": map[string]any{"hooks": map[string]any{"Stop": native}}}},
	}, ".qwen/agents", false)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}
	for name, want := range map[string]map[string]any{
		"shared": {"SubagentStop": stop},
		"native": {"Stop": native},
	} {
		raw, err := os.ReadFile(".qwen/agents/" + name + ".md")
		if err != nil {
			t.Fatal(err)
		}
		entry, err := spec.ParseMarkdownBytes(spec.KindAgent, raw)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(entry.Meta["hooks"], want) {
			t.Errorf("%s hooks = %#v, want %#v", name, entry.Meta["hooks"], want)
		}
	}
}

func TestEmitAgents_NotesAgentHookEventQwenSkips(t *testing.T) {
	testutil.TempCwd(t)
	notes := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = notes
	emit.ResetCoverageNotes()
	t.Cleanup(func() { emit.Warner = prev; emit.ResetCoverageNotes() })
	group := []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "./x.sh"}}}}
	err := New().EmitAgents(emit.NewSession(), []spec.Entry{{
		Kind: spec.KindAgent, Name: "rev",
		Meta: map[string]any{"description": "d", "hooks": map[string]any{"TaskCompleted": group, "FileChanged": group}},
	}}, ".qwen/agents", false)
	if err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	for _, event := range []string{"TaskCompleted", "FileChanged"} {
		if !strings.Contains(notes.String(), event) {
			t.Errorf("note must name the skipped event %s: %q", event, notes.String())
		}
	}
}
