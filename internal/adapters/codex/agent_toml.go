package codex

import (
	"fmt"
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
	writeTOMLString(&sb, "name", a.Name)
	writeTOMLString(&sb, "description", description)
	writeTOMLMultiline(&sb, "developer_instructions", instructions)

	if v := stringOr(meta, "model", ""); v != "" {
		writeTOMLString(&sb, "model", v)
	}
	if v := stringOr(meta, "model_reasoning_effort", ""); v != "" {
		writeTOMLString(&sb, "model_reasoning_effort", v)
	}
	if v := stringOr(meta, "sandbox_mode", ""); v != "" {
		writeTOMLString(&sb, "sandbox_mode", v)
	}
	if names := stringSlice(meta["nickname_candidates"]); len(names) > 0 {
		writeTOMLStringArray(&sb, "nickname_candidates", names)
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

// writeTOMLString writes `key = "value"` with backslashes and double
// quotes escaped per the TOML basic-string rules.
func writeTOMLString(sb *strings.Builder, key, value string) {
	fmt.Fprintf(sb, "%s = \"%s\"\n", key, escapeTOMLBasic(value))
}

// writeTOMLMultiline writes `key = """\n<value>\n"""`. Backslashes and
// double quotes inside value are escaped so the closing delimiter cannot
// be confused with content.
func writeTOMLMultiline(sb *strings.Builder, key, value string) {
	escaped := escapeTOMLBasic(value)
	sb.WriteString(key + " = \"\"\"\n")
	sb.WriteString(escaped)
	if !strings.HasSuffix(escaped, "\n") {
		sb.WriteByte('\n')
	}
	sb.WriteString("\"\"\"\n")
}

func writeTOMLStringArray(sb *strings.Builder, key string, values []string) {
	sb.WriteString(key + " = [")
	for i, v := range values {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString("\"" + escapeTOMLBasic(v) + "\"")
	}
	sb.WriteString("]\n")
}

// escapeTOMLBasic escapes the two characters that can break out of a
// basic (or basic-multiline) TOML string: backslash and double quote.
// Newlines pass through unchanged so multiline literals stay readable.
func escapeTOMLBasic(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
