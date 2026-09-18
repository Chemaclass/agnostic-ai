package factory

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// mcpScopeField is the frontmatter key Factory documents for narrowing
// which MCP servers a droid may reach: "Use `mcpServers` to limit which
// MCP servers a droid can reach". Claude, Junie, and Qoder accept the
// same server-name list and receive it today, because each renders its
// own agent frontmatter.
const mcpScopeField = "mcpServers"

// noteDroppedAgentMCPScope surfaces the portable `mcpServers` list
// Factory documents but never receives. Agents are written into the
// shared `.agents/agents/` tree, byte-identical across every co-writer,
// and OpenHands and Antigravity take inline server objects there rather
// than names, so one file cannot satisfy both shapes. Losing the list
// in silence widens a droid's tool surface past what the author wrote
// (target-audit 2026-09-18, #812).
func noteDroppedAgentMCPScope(agents []spec.Entry) {
	dropped := 0
	for _, agent := range agents {
		if emit.SharedSkillFieldDropped(agent, target, mcpScopeField) {
			dropped++
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, mcpScopeField, dropped,
		"`.agents/agents/<name>.md` is shared byte-for-byte with targets that take inline server objects instead of names; set x-"+target+".mcpServers to emit the list for factory only")
}

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
