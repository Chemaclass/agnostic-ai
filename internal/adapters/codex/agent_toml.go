package codex

import (
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// agentTOML renders a Codex CLI custom-agent TOML document for a single
// agent spec. The schema follows
// https://developers.openai.com/codex/subagents:
//
//	name                   (required, from spec.Name)
//	description            (required, from frontmatter; falls back to name)
//	developer_instructions (required, from spec body; falls back to description)
//	model                  (optional, from frontmatter or x-codex.model)
//	model_reasoning_effort (optional, from x-codex)
//	sandbox_mode           (optional, from x-codex)
//	nickname_candidates    (optional, []string from x-codex)
//
// Other Codex agent fields (mcp_servers, skills.config) are not yet
// translated; pass them through `x-codex` raw if needed in a follow-up.
func agentTOML(a spec.Entry) string {
	meta := emit.ResolveMeta(a.Meta, target)

	description := stringOr(meta, "description", a.Name)
	instructions := strings.TrimSpace(a.Body)
	if instructions == "" {
		instructions = description
	}

	var sb strings.Builder
	emit.WriteTOMLString(&sb, "name", a.Name)
	emit.WriteTOMLString(&sb, "description", description)
	emit.WriteTOMLMultiline(&sb, "developer_instructions", instructions)

	if v := stringOr(meta, "model", ""); v != "" {
		emit.WriteTOMLString(&sb, "model", v)
	}
	if v := stringOr(meta, "model_reasoning_effort", ""); v != "" {
		emit.WriteTOMLString(&sb, "model_reasoning_effort", v)
	}
	if v := stringOr(meta, "sandbox_mode", ""); v != "" {
		emit.WriteTOMLString(&sb, "sandbox_mode", v)
	}
	if names := stringSlice(meta["nickname_candidates"]); len(names) > 0 {
		emit.WriteTOMLStringArray(&sb, "nickname_candidates", names)
	}
	return sb.String()
}

func stringOr(meta map[string]any, key, fallback string) string {
	if v, ok := meta[key].(string); ok && v != "" {
		return v
	}
	return fallback
}

// stringSlice converts an []any of strings (yaml's default) into []string.
// Returns nil for any other shape so the caller can skip the field.
func stringSlice(v any) []string {
	switch xs := v.(type) {
	case []string:
		return xs
	case []any:
		out := make([]string, 0, len(xs))
		for _, x := range xs {
			s, ok := x.(string)
			if !ok {
				return nil
			}
			out = append(out, s)
		}
		return out
	}
	return nil
}

