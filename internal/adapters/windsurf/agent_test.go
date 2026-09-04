package windsurf

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

// Devin CLI documents a custom-subagent file at `.devin/agents/<name>.md`
// with `name`, `description`, `model`, `allowed-tools`, and `max-nesting`
// frontmatter (docs.devin.ai/cli/subagents). Flattening an Agent spec into
// `.devin/rules/agent-<name>.md` put it on the rules path and left every
// one of those fields unreachable (#638).
func TestEmit_AgentsWriteNativeSubagentFiles(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "reviewer", Body: "reviewer body",
		Meta: map[string]any{
			"description": "Reviews code changes",
			"model":       "sonnet",
			"tools":       []any{"Read", "Grep", "Bash"},
			"x-windsurf":  map[string]any{"max-nesting": 3},
		},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}

	got := readAgentFile(t, filepath.Join(dir, ".devin/agents/reviewer.md"))
	for _, want := range []string{
		"name: reviewer\n",
		"description: Reviews code changes\n",
		"model: sonnet\n",
		// Devin's tool vocabulary is `read`, `edit`, `grep`, `glob`,
		// `exec` (docs.devin.ai/cli/reference/permissions), so the
		// Claude-style names translate rather than pass through.
		"allowed-tools:\n  - read\n  - grep\n  - exec\n",
		"max-nesting: 3\n",
		"reviewer body",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in .devin/agents/reviewer.md:\n%s", want, got)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".devin/rules/agent-reviewer.md")); !os.IsNotExist(err) {
		t.Errorf("expected no rule-form .devin/rules/agent-reviewer.md, err=%v", err)
	}
}

// A tool name outside agnostic-ai's Claude-style set has no confirmed
// Devin equivalent, so it drops from the list instead of shipping a name
// the vendor never documented.
func TestEmit_AgentToolsOutsideDevinVocabularyDrop(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "researcher", Body: "researcher body",
		Meta: map[string]any{"tools": []any{"Read", "WebSearch", "mcp__github__get_issue"}},
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readAgentFile(t, filepath.Join(dir, ".devin/agents/researcher.md"))
	if !strings.Contains(got, "  - read\n") {
		t.Errorf("expected translated read tool:\n%s", got)
	}
	// mcp__<server>__<tool> is documented on the same permissions page,
	// so it passes through untranslated.
	if !strings.Contains(got, "  - mcp__github__get_issue\n") {
		t.Errorf("expected the MCP tool name to pass through:\n%s", got)
	}
	if strings.Contains(got, "WebSearch") {
		t.Errorf("WebSearch has no documented Devin tool name and must drop:\n%s", got)
	}
}

// A scoped agent lands flat in the agents directory. Devin documents
// sub-directory discovery for rules only, so scoping the agents dir
// would put the file where nothing reads it.
func TestEmit_ScopedAgentStaysFlat(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{{
		Kind: spec.KindAgent, Name: "delta", Path: "agents/backend/delta.md",
		Scope: "backend", Body: "delta body",
	}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".devin/agents/delta.md")); err != nil {
		t.Errorf("expected flat .devin/agents/delta.md: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "backend/.devin/rules/agent-delta.md")); !os.IsNotExist(err) {
		t.Errorf("expected no scoped rule-form agent file, err=%v", err)
	}
}

func readAgentFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
