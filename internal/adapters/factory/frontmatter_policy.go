package factory

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// manualOnlyField is the frontmatter key this vendor documents for
// keeping a skill out of automatic model invocation. Claude, Cursor,
// Crush, and Factory all spell it the same way and mean the same thing.
const manualOnlyField = "disable-model-invocation"

// droidEffortLevels is Factory's own value space for reasoning effort:
// "`low`, `medium`, or `high` for models that support it". The portable
// `effort` field is wider, since Qoder and Claude Code also document
// `xhigh`, `max`, and (on Qoder) a positive integer budget, so a value
// outside this set is reported rather than written.
var droidEffortLevels = map[string]bool{"low": true, "medium": true, "high": true}

// droidReasoningEffort maps the portable `effort` onto Factory's
// `reasoningEffort`, and reports whether the value is one Factory
// accepts. A native `reasoningEffort` wins when the author wrote one.
func droidReasoningEffort(resolved map[string]any) (string, bool) {
	if native, _ := resolved["reasoningEffort"].(string); native != "" {
		return native, droidEffortLevels[native]
	}
	portable, _ := resolved["effort"].(string)
	if portable == "" {
		return "", false
	}
	return portable, droidEffortLevels[portable]
}

// noteUnsupportedEffort reports a portable `effort` Factory cannot
// honor. Its enum stops at `high`, so `xhigh`, `max`, and Qoder's
// integer budgets have no Factory spelling, and the vendor also ignores
// the field entirely under `model: inherit`. Writing an out-of-range
// value would fail Droid CLI's load-time validation the same way an
// unknown tool ID does (target-audit 2026-09-18, #824).
func noteUnsupportedEffort(agents []spec.Entry) {
	unsupported := 0
	for _, agent := range agents {
		value, ok := droidReasoningEffort(emit.ResolveMeta(agent.Meta, target))
		if value != "" && !ok {
			unsupported++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "effort", unsupported,
		"Factory documents `low`, `medium`, and `high` only, so `xhigh`, `max`, and integer budgets are dropped; set x-factory.reasoningEffort to one it accepts")
}

// noteDroppedManualOnly surfaces the portable `disable-model-invocation`
// this target documents but never receives. Skills are written into the
// shared `.agents/skills/` tree, byte-identical across every co-writer,
// so emitting the key here would also hand it to targets sharing that
// path whose frontmatter has no such field. Losing it in silence is the
// worse half of that trade: a skill the author marked manual-only
// becomes model-invocable, the same safety boundary #823 closed for
// Windsurf (target-audit 2026-09-18, #811).
func noteDroppedManualOnly(skills []spec.Entry) {
	dropped := 0
	for _, skill := range skills {
		if emit.SharedSkillFieldDropped(skill, target, manualOnlyField) {
			dropped++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindSkill, manualOnlyField, dropped,
		"`.agents/skills/<name>/SKILL.md` is shared byte-for-byte with targets whose frontmatter has no such key; set x-"+target+"."+manualOnlyField+" to emit it for this target only")
}
