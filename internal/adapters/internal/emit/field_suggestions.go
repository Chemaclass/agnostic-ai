package emit

import (
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func fieldNoteSuggestion(kind spec.Kind, field, reason string, shared bool) string {
	if kind == spec.KindSkill {
		switch field {
		case "argument-hint":
			if reason != "the skill file has no argument-hint field" {
				return ""
			}
			if shared {
				return "put the argument hint in the skill description or body; shared global skill files omit target overrides"
			}
			return "move `argument-hint` under `x-claude` for Claude Code, and put argument guidance in the description or body for other tools"
		case "metadata":
			if reason != "the skill file has no metadata field" {
				return ""
			}
			if shared {
				return "keep descriptive metadata in the skill body; shared global skill files omit target overrides"
			}
			return "move `metadata` under `x-claude` and `x-cursor` for those tools; keep information other tools need in the skill body"
		case "disable-model-invocation":
			if reason == "the skill file has no disable-model-invocation field" || reason == "its global copy stays model-invocable" {
				if shared {
					return "if manual-only invocation is required, use a supported native skill invocation policy, or limit this skill to tools with manual-only enforcement using `targets-exclude:`; excluding a tool does not hide copies in shared global skill directories it reads"
				}
				return "if manual-only invocation is required, use a supported native skill invocation policy, or add tools without manual-only enforcement to this skill's `targets-exclude:`"
			}
		}
	}
	if kind == spec.KindAgent {
		switch field {
		case "readonly":
			if reason == "the agent file has no read-only mode" {
				return "if read-only access is required, configure and verify read-only access in the tool's native permissions or session settings, or add tools without enforcement to this agent's `targets-exclude:`"
			}
		case "sandbox_mode":
			if reason == "`readonly: true` also writes it; since openai/codex#39299 an agent keeps the parent session's sandbox, so set sandbox_mode in config.toml instead" {
				return "set the required `sandbox_mode` in the parent Codex session's config.toml (`sandbox_mode = \"read-only\"` if read-only access is required) and verify the session policy; then set `x-codex: {sandbox_mode: null}` to omit the inactive agent key, or exclude this agent from Codex with `targets-exclude: [codex]` if that policy cannot be enforced"
			}
		}
	}
	return ""
}
