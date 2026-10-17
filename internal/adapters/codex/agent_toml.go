package codex

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// agentTOML renders a Codex CLI custom-agent TOML document for a single
// agent spec. The schema follows
// https://learn.chatgpt.com/docs/agent-configuration/subagents:
//
//	name                   (required, from spec.Name)
//	description            (required, from frontmatter; falls back to name)
//	developer_instructions (required, from spec body; falls back to description)
//	model                  (optional, from frontmatter or x-codex.model)
//	model_reasoning_effort (optional, from x-codex.model_reasoning_effort or
//	                        the portable `effort` field; see effort.go)
//	sandbox_mode           (optional, from readonly or x-codex; ignored
//	                        since openai/codex#39299)
//	nickname_candidates    (optional, []string from x-codex)
//	tools                  (optional config table from x-codex)
//
// Agent-scoped `mcp_servers` and other nested-table fields pass through
// `x-codex` verbatim: the importer captures them under `x-codex.<key>`,
// and the emitter renders nested-table maps as `[<key>.<inner>]` blocks
// so they round-trip. A table of plain values (`[features]`) comes back
// as an inline table with the same values. See writeXCodexExtras for the
// scalar-vs-table emission order.
func agentTOML(a spec.Entry) string {
	body, _ := agentTOMLWithSkips(a)
	return body
}

// agentTOMLWithSkips is agentTOML plus the `x-codex` key paths it could
// not encode as TOML.
func agentTOMLWithSkips(a spec.Entry) (string, tomlSkips) {
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
	if v, ok := codexReasoningEffort(meta); ok {
		emit.WriteTOMLString(&sb, "model_reasoning_effort", v)
	}
	if v := agentSandboxMode(a); v != "" {
		emit.WriteTOMLString(&sb, "sandbox_mode", v)
	}
	emit.WriteTOMLMultiline(&sb, "developer_instructions", instructions)
	if names := stringSlice(meta["nickname_candidates"]); len(names) > 0 {
		emit.WriteTOMLStringArray(&sb, "nickname_candidates", names)
	}
	var skips tomlSkips
	writeXCodexExtras(&sb, &skips, a.Meta)
	return sb.String(), skips
}

// agentSandboxMode is the sandbox_mode an agent file carries: an
// explicit x-codex value, or read-only from `readonly: true`.
func agentSandboxMode(a spec.Entry) string {
	meta := emit.ResolveMeta(a.Meta, target)
	sandboxMode := stringOr(meta, "sandbox_mode", "")
	custom, _ := a.Meta["x-codex"].(map[string]any)
	if _, explicit := custom["sandbox_mode"]; !explicit && sandboxMode == "" && meta["readonly"] == true {
		return "read-only"
	}
	return sandboxMode
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

// tomlSkips collects the x-codex key paths the encoder could not write,
// so sync can name them instead of dropping them silently.
type tomlSkips []string

// tomlPath is a table path as raw keys. toml() quotes it for headers;
// the raw form names a key in a note.
type tomlPath []string

func (p tomlPath) child(key string) tomlPath {
	return append(slices.Clone(p), key)
}

func (p tomlPath) toml() string {
	quoted := make([]string, len(p))
	for i, k := range p {
		quoted[i] = emit.TOMLKey(k)
	}
	return strings.Join(quoted, ".")
}

func (p tomlPath) note(key string) string {
	return strings.Join(p.child(key), ".")
}

// writeTOMLTable renders a map as a TOML table. Scalar values lead,
// followed by nested maps as child tables and arrays of maps as
// `[[path.key]]` tables, so each emitted document is valid and
// deterministic.
func writeTOMLTable(sb *strings.Builder, skips *tomlSkips, path tomlPath, values map[string]any) {
	sb.WriteString("\n[" + path.toml() + "]\n")
	var scalarKeys, tableKeys, arrayKeys []string
	for key, value := range values {
		_, isMap := value.(map[string]any)
		switch {
		case isMap:
			tableKeys = append(tableKeys, key)
		case isTableArray(value):
			arrayKeys = append(arrayKeys, key)
		default:
			scalarKeys = append(scalarKeys, key)
		}
	}
	slices.Sort(scalarKeys)
	slices.Sort(tableKeys)
	slices.Sort(arrayKeys)
	for _, key := range scalarKeys {
		writeTOMLAny(sb, skips, path, key, values[key])
	}
	for _, key := range tableKeys {
		writeTOMLTable(sb, skips, path.child(key), values[key].(map[string]any))
	}
	for _, key := range arrayKeys {
		itemPath := path.child(key)
		for _, item := range values[key].([]any) {
			sb.WriteString("\n[[" + itemPath.toml() + "]]\n")
			inner := item.(map[string]any)
			for _, ik := range slices.Sorted(maps.Keys(inner)) {
				writeTOMLAny(sb, skips, itemPath, ik, inner[ik])
			}
		}
	}
}

// isTableMap reports whether v is a map that holds a nested map or an
// array of maps, so it needs `[table]` headers; a map of plain values
// stays an inline table.
func isTableMap(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	for _, vv := range m {
		if _, ok := vv.(map[string]any); ok || isTableArray(vv) {
			return true
		}
	}
	return false
}

// isTableArray reports whether v is a non-empty []any of maps, which
// TOML spells as repeated `[[array.of.tables]]` blocks.
func isTableArray(v any) bool {
	xs, ok := v.([]any)
	if !ok || len(xs) == 0 {
		return false
	}
	for _, x := range xs {
		if _, ok := x.(map[string]any); !ok {
			return false
		}
	}
	return true
}

// writeXCodexExtras walks `meta["x-codex"]` (when present) and emits
// every key not already in codexAgentEmittedKeys. Supports the common
// TOML value shapes: strings, bools, numbers, string arrays, inline
// string tables, and nested tables (e.g. `mcp_servers.<name>`).
//
// Nested-table values emit last so the file stays TOML-valid: bare
// key-value pairs must come before any `[section]` header inside the
// document.
func writeXCodexExtras(sb *strings.Builder, skips *tomlSkips, raw map[string]any) {
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
		if isNestedTableMap(v) || isTableMap(v) {
			tableKeys = append(tableKeys, k)
		} else {
			inlineKeys = append(inlineKeys, k)
		}
	}
	slices.Sort(inlineKeys)
	slices.Sort(tableKeys)
	for _, k := range inlineKeys {
		writeTOMLAny(sb, skips, nil, k, x[k])
	}
	for _, k := range tableKeys {
		if k == "tools" {
			writeTOMLTable(sb, skips, tomlPath{k}, x[k].(map[string]any))
			continue
		}
		if isNestedTableMap(x[k]) {
			writeNestedTableMap(sb, skips, tomlPath{k}, x[k].(map[string]any))
			continue
		}
		writeTOMLTable(sb, skips, tomlPath{k}, x[k].(map[string]any))
	}
}

// isNestedTableMap reports whether v is a non-empty map[string]any whose
// values are themselves map[string]any. Such values must emit as
// `[<key>.<inner>]` tables rather than inline tables so codex-side
// schemas such as `mcp_servers` decode correctly.
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
// sorted by name. Each block's fields go through writeTOMLAny so
// strings, arrays, and inline tables round-trip without inventing new
// encodings.
func writeNestedTableMap(sb *strings.Builder, skips *tomlSkips, key tomlPath, m map[string]any) {
	for _, name := range slices.Sorted(maps.Keys(m)) {
		inner, _ := m[name].(map[string]any)
		path := key.child(name)
		sb.WriteString("\n[" + path.toml() + "]\n")
		for _, ik := range slices.Sorted(maps.Keys(inner)) {
			writeTOMLAny(sb, skips, path, ik, inner[ik])
		}
	}
}

// writeTOMLAny emits `key = <value>` for the shapes commonly used in
// agent TOML pass-through. A nil value (a YAML null deletion marker) and
// a map with nothing to write are left out; a value TOML cannot hold,
// such as a non-finite float, is left out and recorded in skips under
// its raw dotted path. Keys that are not bare-safe are quoted.
func writeTOMLAny(sb *strings.Builder, skips *tomlSkips, path tomlPath, key string, v any) {
	if v == nil {
		return
	}
	if m, ok := v.(map[string]any); ok {
		for _, skipped := range emit.WriteTOMLInlineTable(sb, emit.TOMLKey(key), m) {
			*skips = append(*skips, path.child(key).note(skipped))
		}
		return
	}
	lit, ok := emit.TOMLLiteral(v)
	if !ok {
		*skips = append(*skips, path.note(key))
		return
	}
	sb.WriteString(emit.TOMLKey(key) + " = " + lit + "\n")
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

var inertAgentSessionKeys = []string{"approval_policy", "model_provider", "mcp_servers", "service_tier"}

// EmitAgents writes native Codex agent definitions to dir.
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	noteUnsupportedCodexEffort(agents)
	var droppedAgentTools []spec.Entry
	inertSandboxModes := 0
	inertSessionSettings := map[string]int{}
	for _, a := range agents {
		path := filepath.Join(dir, a.Name+".toml")
		body, skips := agentTOMLWithSkips(a)
		if err := sess.WriteFile(path, emit.WithHeader(body, emit.FormatTOML), dryRun); err != nil {
			return err
		}
		if len(skips) > 0 {
			emit.NoteProject(fmt.Sprintf("codex agent %s: left out x-codex values TOML cannot hold (%s)", a.Name, strings.Join(skips, ", ")))
		}
		if len(emit.StringSlice(a.Meta["tools"])) > 0 {
			droppedAgentTools = append(droppedAgentTools, a)
		}
		if agentSandboxMode(a) != "" {
			inertSandboxModes++
		}
		custom, _ := a.Meta["x-codex"].(map[string]any)
		for _, key := range inertAgentSessionKeys {
			if custom[key] != nil {
				inertSessionSettings[key]++
			}
		}
	}
	// The key stays for Codex releases before rust-v0.155.0, which still honor it.
	emit.NoteFieldNoOp(target, spec.KindAgent, "sandbox_mode", inertSandboxModes,
		"`readonly: true` also writes it; since openai/codex#39299 an agent keeps the parent session's sandbox, so set sandbox_mode in config.toml instead")
	for _, key := range inertAgentSessionKeys {
		emit.NoteFieldNoOp(target, spec.KindAgent, key, inertSessionSettings[key],
			"an agent keeps the parent session's setting; configure it in the parent session instead")
	}
	emit.NoteAgentToolsNoOp(target, droppedAgentTools,
		"Codex uses tools as a configuration table, not a Claude-style allowlist; set x-codex.tools for Codex-native tool settings")
	return nil
}
