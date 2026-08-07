package codex

import (
	"fmt"
	"maps"
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
//	tools                  (optional config table from x-codex)
//
// Agent-scoped `mcp_servers` and other nested-table fields pass through
// `x-codex` verbatim: the importer captures them under `x-codex.<key>`,
// and the emitter renders nested-table maps as `[<key>.<inner>]` blocks
// so they round-trip byte-equivalently. See writeXCodexExtras for the
// scalar-vs-table emission order.
func agentTOML(a spec.Entry) string {
	meta := emit.ResolveMeta(a.Meta, target)

	description := stringOr(meta, "description", a.Name)
	instructions := strings.TrimSpace(a.Body)
	if instructions == "" {
		instructions = description
	}

	// Codex agent names use underscores by convention; the agnostic
	// spec stores the canonical dash-cased filename. `x-codex.name`
	// overrides the on-disk slug so the emitted TOML carries the
	// runtime identifier (e.g. `changelog_keeper`) Codex expects.
	runtimeName := a.Name
	if v := xCodexString(a.Meta, "name"); v != "" {
		runtimeName = v
	}

	// Key order matches the convention codex docs show and hand-authored
	// agents use: scalars first so a reader sees the configuration at a
	// glance, multi-line developer_instructions last. Codex CLI itself
	// is order-insensitive; the order is purely a readability + byte-
	// stable round-trip win.
	var sb strings.Builder
	emit.WriteTOMLString(&sb, "name", runtimeName)
	emit.WriteTOMLString(&sb, "description", description)
	if v := stringOr(meta, "model", ""); v != "" {
		emit.WriteTOMLString(&sb, "model", v)
	}
	if v := stringOr(meta, "model_reasoning_effort", ""); v != "" {
		emit.WriteTOMLString(&sb, "model_reasoning_effort", v)
	}
	if v := stringOr(meta, "sandbox_mode", ""); v != "" {
		emit.WriteTOMLString(&sb, "sandbox_mode", v)
	}
	emit.WriteTOMLMultiline(&sb, "developer_instructions", instructions)
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
	"tools":                  true,
	"nickname_candidates":    true,
}

// writeTOMLTable renders a map as a TOML table. Scalar values lead,
// followed by nested maps as child tables, so each emitted document is
// valid and deterministic.
func writeTOMLTable(sb *strings.Builder, path string, values map[string]any) {
	sb.WriteString("\n[" + path + "]\n")
	var scalarKeys, tableKeys []string
	for key, value := range values {
		if _, ok := value.(map[string]any); ok {
			tableKeys = append(tableKeys, key)
		} else {
			scalarKeys = append(scalarKeys, key)
		}
	}
	slices.Sort(scalarKeys)
	slices.Sort(tableKeys)
	for _, key := range scalarKeys {
		writeTOMLAny(sb, key, values[key])
	}
	for _, key := range tableKeys {
		writeTOMLTable(sb, path+"."+key, values[key].(map[string]any))
	}
}

// writeXCodexExtras walks `meta["x-codex"]` (when present) and emits
// every key not already in codexAgentEmittedKeys. Supports the common
// TOML value shapes: strings, bools, numbers, string arrays, inline
// string tables, and nested tables (e.g. `mcp_servers.<name>`).
//
// Nested-table values emit last so the file stays TOML-valid: bare
// key-value pairs must come before any `[section]` header inside the
// document.
func writeXCodexExtras(sb *strings.Builder, raw map[string]any) {
	x, ok := raw["x-codex"].(map[string]any)
	if !ok {
		return
	}
	var inlineKeys, tableKeys []string
	for k, v := range x {
		if k == "tools" {
			if tools, ok := v.(map[string]any); ok && len(tools) > 0 {
				tableKeys = append(tableKeys, k)
			}
			continue
		}
		if codexAgentEmittedKeys[k] {
			continue
		}
		if isNestedTableMap(v) {
			tableKeys = append(tableKeys, k)
		} else {
			inlineKeys = append(inlineKeys, k)
		}
	}
	slices.Sort(inlineKeys)
	slices.Sort(tableKeys)
	for _, k := range inlineKeys {
		writeTOMLAny(sb, k, x[k])
	}
	for _, k := range tableKeys {
		if k == "tools" {
			writeTOMLTable(sb, k, x[k].(map[string]any))
			continue
		}
		writeNestedTableMap(sb, k, x[k].(map[string]any))
	}
}

// isNestedTableMap reports whether v is a non-empty map[string]any whose
// values are themselves map[string]any. Such values must emit as
// `[<key>.<inner>]` tables rather than inline tables so codex-side
// schemas (`mcp_servers`, `skills.config`) decode correctly.
func isNestedTableMap(v any) bool {
	m, ok := v.(map[string]any)
	if !ok || len(m) == 0 {
		return false
	}
	for _, vv := range m {
		if _, ok := vv.(map[string]any); !ok {
			return false
		}
	}
	return true
}

// writeNestedTableMap renders one `[key.inner]` block per inner key,
// sorted by name. Each block's scalar fields go through writeTOMLAny so
// strings, arrays, and inline tables of strings round-trip without
// inventing new encodings.
func writeNestedTableMap(sb *strings.Builder, key string, m map[string]any) {
	for _, name := range slices.Sorted(maps.Keys(m)) {
		inner, _ := m[name].(map[string]any)
		sb.WriteString("\n[" + key + "." + name + "]\n")
		for _, ik := range slices.Sorted(maps.Keys(inner)) {
			writeTOMLAny(sb, ik, inner[ik])
		}
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

// xCodexString returns meta["x-codex"][key] as a string. Returns ""
// when x-codex is absent, not a map, or the value is not a string.
func xCodexString(meta map[string]any, key string) string {
	x, ok := meta["x-codex"].(map[string]any)
	if !ok {
		return ""
	}
	s, _ := x[key].(string)
	return s
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
