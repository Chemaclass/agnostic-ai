package codex

import (
	"fmt"
	"slices"
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
	writeXCodexExtras(&sb, a.Meta)
	return sb.String()
}

// codexAgentEmittedKeys are the TOML keys agentTOML writes above. Any
// extra key carried in `x-codex` that is not in this set passes through
// so future / unknown Codex fields round-trip without data loss.
var codexAgentEmittedKeys = map[string]bool{
	"name":                   true,
	"description":            true,
	"developer_instructions": true,
	"model":                  true,
	"model_reasoning_effort": true,
	"sandbox_mode":           true,
	"nickname_candidates":    true,
}

// writeXCodexExtras walks `meta["x-codex"]` (when present) and emits
// every key not already in codexAgentEmittedKeys. Supports the common
// TOML value shapes: strings, bools, numbers, string arrays, and inline
// string tables.
func writeXCodexExtras(sb *strings.Builder, raw map[string]any) {
	x, ok := raw["x-codex"].(map[string]any)
	if !ok {
		return
	}
	keys := make([]string, 0, len(x))
	for k := range x {
		if codexAgentEmittedKeys[k] {
			continue
		}
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		writeTOMLAny(sb, k, x[k])
	}
}

// writeTOMLAny emits `key = <value>` for the shapes commonly used in
// agent TOML pass-through. Unsupported shapes are silently skipped to
// keep output valid; the importer can capture them but the emitter
// declines to invent encodings.
func writeTOMLAny(sb *strings.Builder, key string, v any) {
	switch val := v.(type) {
	case string:
		emit.WriteTOMLString(sb, key, val)
	case bool:
		fmt.Fprintf(sb, "%s = %t\n", key, val)
	case int:
		fmt.Fprintf(sb, "%s = %d\n", key, val)
	case int64:
		fmt.Fprintf(sb, "%s = %d\n", key, val)
	case float64:
		fmt.Fprintf(sb, "%s = %v\n", key, val)
	case []string:
		emit.WriteTOMLStringArray(sb, key, val)
	case []any:
		if ss := stringSlice(val); ss != nil {
			emit.WriteTOMLStringArray(sb, key, ss)
		}
	case map[string]string:
		emit.WriteTOMLInlineStringTable(sb, key, val)
	case map[string]any:
		emit.WriteTOMLInlineStringTable(sb, key, toStringMap(val))
	}
}

func toStringMap(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
		}
	}
	return out
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
