package copilot

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

// Regression for the target-audit finding: VS Code's own docs
// (code.visualstudio.com/docs/agent-customization/mcp-servers) state
// "The enable/disable state is stored separately from the server
// configuration in mcp.json, so it does not affect shared configuration
// files." Writing `disabled: true` into `.vscode/mcp.json` would let a
// user believe the server stopped connecting when Copilot ignores the
// key and keeps using it, so the field must not reach the file, and the
// drop must be loud rather than silent.
func TestEmit_MCP_DisabledHasNoFileBasedEffect(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "disabled": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".vscode", "mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	fs := parsed["servers"].(map[string]any)["fs"].(map[string]any)
	if _, ok := fs["disabled"]; ok {
		t.Errorf("copilot has no file-based disable key; must not emit one: %s", raw)
	}

	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "`disabled` on 1 mcp has no effect on copilot") {
		t.Errorf("expected a field no-op note, got: %s", buf.String())
	}
}

// Copilot's agent profile table (docs.github.com/en/copilot/reference/
// custom-agents-configuration) has no effort key, and per-agent
// effortLevel exists only in the user-tier subagents.agents setting, so
// a portable `effort` cannot reach any repository file. The drop must be
// reported rather than silent (#1066).
func TestEmitAgents_EffortDropRaisesCoverageNote(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	agents := []spec.Entry{
		{Kind: spec.KindAgent, Name: "rev", Meta: map[string]any{"description": "Review", "effort": "high", "model": "gpt-6-sol"}, Body: "Review."},
		{Kind: spec.KindAgent, Name: "plain", Meta: map[string]any{"description": "Plain"}, Body: "Plain."},
	}
	if err := New().EmitAgents(emit.NewSession(), agents, filepath.Join(dir, ".github", "agents"), false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".github", "agents", "rev.agent.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "effort") {
		t.Errorf("copilot agent profiles have no effort key; must not emit one:\n%s", raw)
	}

	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "`effort` on 1 agent has no effect on copilot") {
		t.Errorf("expected an effort no-op note for one agent, got: %s", buf.String())
	}
}

// An explicit x-copilot.effort is the author's own passthrough: it lands
// in the profile, so reporting it as dropped would contradict the file.
// A per-target map with no copilot entry collapses away for copilot and
// is not a drop either.
func TestEmitAgents_EffortNoteSkipsPassthroughAndOtherTargets(t *testing.T) {
	dir := testutil.TempCwd(t)
	buf := swapNoteWarner(t)

	agents := []spec.Entry{
		{Kind: spec.KindAgent, Name: "explicit", Meta: map[string]any{"description": "Explicit", "x-copilot": map[string]any{"effort": "high"}}, Body: "Explicit."},
		{Kind: spec.KindAgent, Name: "other", Meta: map[string]any{"description": "Other", "effort": map[string]any{"claude": "xhigh"}}, Body: "Other."},
	}
	if err := New().EmitAgents(emit.NewSession(), agents, filepath.Join(dir, ".github", "agents"), false); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".github", "agents", "explicit.agent.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "effort: high") {
		t.Errorf("x-copilot.effort must pass through to the profile:\n%s", raw)
	}

	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "`effort`") {
		t.Errorf("no effort note expected for a passthrough or another target's value, got: %s", buf.String())
	}
}
