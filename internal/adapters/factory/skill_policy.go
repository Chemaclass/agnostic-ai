package factory

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// manualOnlyField is the frontmatter key this vendor documents for
// keeping a skill out of automatic model invocation. Claude, Cursor,
// Crush, and Factory all spell it the same way and mean the same thing.
const manualOnlyField = "disable-model-invocation"

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
