package emit

// RefForms holds, per target, the phrase that target's docs give
// for invoking an agent or a skill by name, as a format with one %s for
// the name. A keyword is listed only where a vendor page documents it;
// the rest render a neutral phrase and raise a coverage note.
var RefForms = map[string]map[string]string{
	// https://code.claude.com/docs/en/sub-agents#invoke-subagents-explicitly
	// https://code.claude.com/docs/en/skills#control-who-invokes-a-skill
	"claude": {RefAgent: "the %s subagent", RefSkill: "/%s"},
	// Codex names no agent in a prompt, only "spawn" phrasing:
	// https://learn.chatgpt.com/docs/agent-configuration/subagents.md
	// https://learn.chatgpt.com/docs/build-skills.md#how-chatgpt-and-codex-use-skills
	"codex": {RefSkill: "$%s"},
	// https://cursor.com/docs/subagents.md#explicit-invocation
	// https://cursor.com/docs/skills.md
	"cursor": {RefAgent: "the %s subagent", RefSkill: "/%s"},
	// Copilot CLI phrasing; the cloud agent documents neither form.
	// https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/create-custom-agents-for-cli
	// https://docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/add-skills
	"copilot": {RefAgent: "the %s agent", RefSkill: "the /%s skill"},
}
