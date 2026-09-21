package codex

import (
	"strconv"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// codexReasoningEffort maps the portable `effort` field onto Codex's own
// `model_reasoning_effort`, and reports whether the resolved value is one
// Codex's on-disk agent schema can carry. A native `model_reasoning_effort`
// set through `x-codex` wins when the author wrote one; ResolveMeta has
// already flattened it onto the top level under that key by the time this
// runs.
//
// Codex's `ReasoningEffort` type (codex-rs
// protocol/src/openai_models.rs) deserializes any non-empty string: a
// name it recognizes (`none`, `minimal`, `low`, `medium`, `high`,
// `xhigh`, `max`, `ultra`, `persistent`) becomes that named variant,
// and anything else falls back to `Custom(string)` rather than
// erroring. So every string `effort` value reaches
// `model_reasoning_effort` unchanged, including `xhigh` and `max`,
// which Factory's own stricter enum rejects.
//
// Qoder's integer budget (`effort: 8000`) has no string form in that
// schema: the field type is a string, not a number, so writing the
// digits would misrepresent a token budget as a named effort tier the
// vendor never documented. It is reported instead of written, the same
// caution Factory's own numeric case takes.
func codexReasoningEffort(resolved map[string]any) (string, bool) {
	if native, _ := resolved["model_reasoning_effort"].(string); native != "" {
		return native, true
	}
	if portable, _ := resolved["effort"].(string); portable != "" {
		return portable, true
	}
	if budget, ok := emit.IntField(resolved, "effort"); ok {
		return strconv.Itoa(budget), false
	}
	return "", false
}

// noteUnsupportedCodexEffort reports a portable `effort` Codex cannot
// carry as `model_reasoning_effort`: only an integer budget, which has
// no string form in Codex's schema. See codexReasoningEffort.
func noteUnsupportedCodexEffort(agents []spec.Entry) {
	unsupported := 0
	for _, agent := range agents {
		value, ok := codexReasoningEffort(emit.ResolveMeta(agent.Meta, target))
		if value != "" && !ok {
			unsupported++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "effort", unsupported,
		"model_reasoning_effort takes a string; Qoder's integer budget has no string form, set x-codex.model_reasoning_effort to a string value")
}
