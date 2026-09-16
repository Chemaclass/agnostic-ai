package antigravity

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

// Antigravity documents a custom-subagent file at
// `.agents/agents/<name>/agent.md` with `name` and `description` required
// (antigravity.google/docs/subagents). Flattening an Agent spec into
// `.agents/rules/agent-<name>.md` put it on the rules path, where the
// subagent loader never looks (#638).
func TestEmit_AgentsWriteNativeSubagentFiles(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "auditor", Body: "auditor body",
		Meta: map[string]any{
			"description": "Audits code for security issues",
			"tools":       []any{"Read", "Grep"},
			"x-antigravity": map[string]any{
				"tools": []any{"view_file", "grep_search"},
				"model": "pro",
			},
		},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readFile(t, filepath.Join(dir, ".agents/agents/auditor/agent.md"))
	for _, want := range []string{
		"name: auditor\n",
		"description: Audits code for security issues\n",
		"model: pro\n",
		// Only the author-declared x-antigravity vocabulary reaches
		// `tools`. The vendor warns that an unmapped name "may cause the
		// subagent process to hang during execution".
		"  - view_file\n",
		"  - grep_search\n",
		"auditor body",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in .agents/agents/auditor/agent.md:\n%s", want, got)
		}
	}
	for _, banned := range []string{"Read", "Grep"} {
		if strings.Contains(got, banned) {
			t.Errorf("Claude-style tool name %q must never reach antigravity:\n%s", banned, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/rules/agent-auditor.md")); !os.IsNotExist(err) {
		t.Errorf("expected no rule-form .agents/rules/agent-auditor.md, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents/agents/auditor.md")); !os.IsNotExist(err) {
		t.Errorf("expected no legacy flat agent file, err=%v", err)
	}
}

func TestEmit_AgentsDirOverrideKeepsNestedProfileLayout(t *testing.T) {
	dir := testutil.TempCwd(t)
	cfg := &config.Config{Outputs: map[string]config.Output{"antigravity": {AgentsDir: "custom/agents"}}}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "auditor", Body: "Audit."}}

	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom/agents/auditor/agent.md")); err != nil {
		t.Errorf("expected nested profile under override dir: %v", err)
	}
}

// Antigravity's `model` is a three-value tier enum (`inherit`, `flash`,
// `pro`), not a model ID. A cross-target `model: sonnet` names no tier,
// so it never reaches the file.
func TestEmit_AgentModelOutsideTierEnumIsDropped(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "auditor", Body: "auditor body",
		Meta: map[string]any{"model": "sonnet"},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(dir, ".agents/agents/auditor/agent.md")); strings.Contains(got, "model:") {
		t.Errorf("expected no model key for an out-of-enum value:\n%s", got)
	}
}
