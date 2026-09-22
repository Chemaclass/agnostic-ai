package codex

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Codex's own ReasoningEffort type (codex-rs protocol/src/openai_models.rs)
// deserializes any non-empty string: a name it recognizes becomes a
// named variant, anything else falls back to Custom(string) rather than
// erroring. So the portable `effort` field reaches `.codex/agents/*.toml`
// as `model_reasoning_effort` for every string value, including ones
// Factory's own stricter enum rejects.
func TestEmit_AgentTOML_PortableEffortWritesModelReasoningEffort(t *testing.T) {
	dir := testutil.TempCwd(t)

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "architect", Body: "Design it.",
			Meta: map[string]any{"description": "Design work", "effort": "xhigh"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, dir+"/.codex/agents/architect.toml")
	if !strings.Contains(got, `model_reasoning_effort = "xhigh"`) {
		t.Errorf("toml missing model_reasoning_effort from portable effort:\n%s", got)
	}
}

// An explicit x-codex.model_reasoning_effort is the author's own
// spelling and wins over the mapped portable value.
func TestCodexReasoningEffort_NativeKeyWins(t *testing.T) {
	got, ok := codexReasoningEffort(map[string]any{"effort": "max", "model_reasoning_effort": "medium"})
	if got != "medium" || !ok {
		t.Errorf("got (%q, %v), want (\"medium\", true)", got, ok)
	}
}

// Every string effort Codex might see round-trips verbatim: the named
// tiers the vendor documents for subagents (developers.openai.com/codex,
// "Reasoning effort (model_reasoning_effort)") and the wider config-level
// enum both parse, and an unrecognized string still parses as a custom
// effort label rather than erroring.
func TestCodexReasoningEffort_AcceptsAnyString(t *testing.T) {
	for _, value := range []string{"minimal", "low", "medium", "high", "xhigh", "max", "ultra", "custom-tier"} {
		got, ok := codexReasoningEffort(map[string]any{"effort": value})
		if got != value || !ok {
			t.Errorf("codexReasoningEffort(%q) = (%q, %v), want (%q, true)", value, got, ok, value)
		}
	}
}

// Qoder's integer budget has no string form Codex's field can carry, so
// it is dropped rather than stringified into a label the vendor never
// documented.
func TestCodexReasoningEffort_RejectsIntegerBudget(t *testing.T) {
	for _, value := range []any{8000, int64(8000), float64(8000)} {
		got, ok := codexReasoningEffort(map[string]any{"effort": value})
		if got != "8000" || ok {
			t.Errorf("codexReasoningEffort(%T %v) = (%q, %v), want (\"8000\", false)", value, value, got, ok)
		}
	}
}

// No effort anywhere means no key and no note.
func TestCodexReasoningEffort_AbsentIsQuiet(t *testing.T) {
	if got, ok := codexReasoningEffort(map[string]any{}); got != "" || ok {
		t.Errorf("got (%q, %v), want (\"\", false)", got, ok)
	}
}

// A per-target `effort` map collapses in emit before this adapter ever
// sees it, so the note path reads the same scalar it always did.
func TestCodexReasoningEffort_ReadsThroughThePerTargetMap(t *testing.T) {
	meta := map[string]any{
		"effort": map[string]any{"codex": "xhigh", "default": "high"},
	}
	if got, ok := codexReasoningEffort(emit.ResolveMeta(meta, target)); got != "xhigh" || !ok {
		t.Errorf("got (%q, %v), want (\"xhigh\", true)", got, ok)
	}
	fallback := map[string]any{
		"effort": map[string]any{"claude": "xhigh", "default": "high"},
	}
	if got, ok := codexReasoningEffort(emit.ResolveMeta(fallback, target)); got != "high" || !ok {
		t.Errorf("got (%q, %v), want (\"high\", true)", got, ok)
	}
}

// The integer-budget coverage note fires once per agent that carries
// one, and stays quiet for every agent whose effort is a plain string.
func TestEmit_NotesIntegerEffortBudgetAsUnsupported(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "alpha", Body: "b", Meta: map[string]any{"effort": 8000}},
		{Kind: spec.KindAgent, Name: "beta", Body: "b", Meta: map[string]any{"effort": "high"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"`effort`", "codex", "1 agent"} {
		if !strings.Contains(note, want) {
			t.Errorf("expected coverage note to mention %q, got: %s", want, note)
		}
	}
}
