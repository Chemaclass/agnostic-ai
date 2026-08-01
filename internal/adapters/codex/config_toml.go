package codex

import (
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// renderConfigTOML builds the `.codex/config.toml` body from the captured
// overlay, the bundle's MCP entries, and any first-class config fields.
// The overlay (carrying user-authored keys outside hooks/mcp_servers) is
// written first; then first-class scalars from `outputs.codex.config`
// (skipping any key the overlay already defines); then MCP server tables.
// Returns "" when there is no valid output to write.
//
// Hooks no longer render here. They land in `.codex/hooks.json` (see
// emitHooksJSON) which natively supports per-hook `timeout` and
// `statusMessage` metadata that the TOML schema discarded.
func renderConfigTOML(_ []spec.Entry, mcps []spec.Entry, cfg *config.CodexConfig, overlayBody string, overlayKeys map[string]bool) string {
	hasContent := anyNamedMCP(mcps) || hasCodexConfig(cfg) || overlayBody != ""
	if !hasContent {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(emit.HeaderBlock(emit.FormatTOML))

	if overlayBody != "" {
		sb.WriteString(overlayBody)
		if !strings.HasSuffix(overlayBody, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}

	writeCodexConfigFields(&sb, cfg, overlayKeys)
	writeMCPServers(&sb, mcps)
	return sb.String()
}

func hasCodexConfig(cfg *config.CodexConfig) bool {
	if cfg == nil {
		return false
	}
	return cfg.Sandbox != "" || cfg.ApprovalPolicy != "" || cfg.Model != "" ||
		cfg.ModelReasoningEffort != "" || cfg.ModelReasoningSummary != "" ||
		cfg.HistoryPersistence != "" || len(cfg.Notify) > 0 ||
		len(cfg.Profiles) > 0 || len(cfg.ModelProviders) > 0
}

// writeCodexConfigFields emits the first-class `.codex/config.toml` scalars
// when set in `outputs.codex.config`. Keys present in overlayKeys are
// skipped so the overlay's value wins on a conflict; this matches the
// claude adapter's overlay-first layering and keeps TOML duplicate-key
// errors from appearing in the emitted file.
func writeCodexConfigFields(sb *strings.Builder, cfg *config.CodexConfig, overlayKeys map[string]bool) {
	if cfg == nil {
		return
	}
	wrote := false
	if cfg.Model != "" && !overlayKeys["model"] {
		emit.WriteTOMLString(sb, "model", cfg.Model)
		wrote = true
	}
	if cfg.Sandbox != "" && !overlayKeys["sandbox"] {
		emit.WriteTOMLString(sb, "sandbox", cfg.Sandbox)
		wrote = true
	}
	if cfg.ApprovalPolicy != "" && !overlayKeys["approval_policy"] {
		emit.WriteTOMLString(sb, "approval_policy", cfg.ApprovalPolicy)
		wrote = true
	}
	if cfg.ModelReasoningEffort != "" && !overlayKeys["model_reasoning_effort"] {
		emit.WriteTOMLString(sb, "model_reasoning_effort", cfg.ModelReasoningEffort)
		wrote = true
	}
	if cfg.ModelReasoningSummary != "" && !overlayKeys["model_reasoning_summary"] {
		emit.WriteTOMLString(sb, "model_reasoning_summary", cfg.ModelReasoningSummary)
		wrote = true
	}
	if len(cfg.Notify) > 0 && !overlayKeys["notify"] {
		emit.WriteTOMLStringArray(sb, "notify", cfg.Notify)
		wrote = true
	}
	if cfg.HistoryPersistence != "" && !overlayKeys["history"] {
		sb.WriteString("\n[history]\n")
		emit.WriteTOMLString(sb, "persistence", cfg.HistoryPersistence)
		wrote = true
	}
	if !overlayKeys["model_providers"] {
		writeCodexModelProviders(sb, cfg.ModelProviders)
		if len(cfg.ModelProviders) > 0 {
			wrote = true
		}
	}
	if !overlayKeys["profiles"] {
		writeCodexProfiles(sb, cfg.Profiles)
		if len(cfg.Profiles) > 0 {
			wrote = true
		}
	}
	if wrote {
		sb.WriteString("\n")
	}
}

// writeCodexModelProviders emits each `[model_providers.<id>]` table sorted
// by id for deterministic output. Empty fields are skipped so each provider
// only carries the keys the user actually set.
func writeCodexModelProviders(sb *strings.Builder, providers map[string]config.CodexModelProvider) {
	if len(providers) == 0 {
		return
	}
	for _, id := range slices.Sorted(maps.Keys(providers)) {
		p := providers[id]
		sb.WriteString("\n[model_providers." + id + "]\n")
		if p.Name != "" {
			emit.WriteTOMLString(sb, "name", p.Name)
		}
		if p.BaseURL != "" {
			emit.WriteTOMLString(sb, "base_url", p.BaseURL)
		}
		if p.WireAPI != "" {
			emit.WriteTOMLString(sb, "wire_api", p.WireAPI)
		}
		if p.APIKeyEnv != "" {
			emit.WriteTOMLString(sb, "api_key_env", p.APIKeyEnv)
		}
		if p.EnvKey != "" {
			emit.WriteTOMLString(sb, "env_key", p.EnvKey)
		}
	}
}

// writeCodexProfiles emits each `[profiles.<name>]` table sorted by name for
// deterministic output. Empty fields are skipped so the profile only carries
// the overrides the user actually set.
func writeCodexProfiles(sb *strings.Builder, profiles map[string]config.CodexProfile) {
	if len(profiles) == 0 {
		return
	}
	for _, name := range slices.Sorted(maps.Keys(profiles)) {
		p := profiles[name]
		sb.WriteString("\n[profiles." + name + "]\n")
		if p.Model != "" {
			emit.WriteTOMLString(sb, "model", p.Model)
		}
		if p.Sandbox != "" {
			emit.WriteTOMLString(sb, "sandbox", p.Sandbox)
		}
		if p.ApprovalPolicy != "" {
			emit.WriteTOMLString(sb, "approval_policy", p.ApprovalPolicy)
		}
		if p.ModelReasoningEffort != "" {
			emit.WriteTOMLString(sb, "model_reasoning_effort", p.ModelReasoningEffort)
		}
		if p.ModelReasoningSummary != "" {
			emit.WriteTOMLString(sb, "model_reasoning_summary", p.ModelReasoningSummary)
		}
		if p.ModelProvider != "" {
			emit.WriteTOMLString(sb, "model_provider", p.ModelProvider)
		}
	}
}

func anyNamedMCP(mcps []spec.Entry) bool {
	for _, m := range mcps {
		if m.Name != "" {
			return true
		}
	}
	return false
}

func writeMCPServers(sb *strings.Builder, mcps []spec.Entry) {
	// Sort by name for deterministic output.
	sorted := append([]spec.Entry(nil), mcps...)
	slices.SortFunc(sorted, func(a, b spec.Entry) int { return strings.Compare(a.Name, b.Name) })
	for _, m := range sorted {
		if m.Name == "" {
			continue
		}
		writeMCPServerTable(sb, m)
	}
}

// writeMCPServerTable emits one `[mcp_servers.<name>]` table with the
// transport-appropriate keys plus the shared description/enabled/roots
// fields that mirror the `.mcp.json` schema (the spec's `disabled` field
// maps to Codex's own `enabled` key; see writeMCPSharedFields).
func writeMCPServerTable(sb *strings.Builder, m spec.Entry) {
	sb.WriteString("[mcp_servers." + m.Name + "]\n")

	transport, _ := m.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	switch transport {
	case "stdio":
		emit.WriteTOMLMCPStdioFields(sb, m.Meta)
		// cwd is documented for codex (and gemini) stdio servers but is
		// not part of the shared WriteTOMLMCPStdioFields helper: that
		// helper is also used by OpenHands, which has no cwd support
		// confirmed. See #532.
		if cwd, _ := m.Meta["cwd"].(string); cwd != "" {
			emit.WriteTOMLString(sb, "cwd", cwd)
		}
	case "http", "sse":
		emit.WriteTOMLMCPURLField(sb, m.Meta)
		if t, _ := m.Meta["bearer_token_env_var"].(string); t != "" {
			emit.WriteTOMLString(sb, "bearer_token_env_var", t)
		}
		emit.WriteTOMLInlineStringTable(sb, "http_headers", emit.StringMap(m.Meta["headers"]))
		// auth (`oauth` | `chatgpt`) is documented for codex http
		// servers. Passed through verbatim; codex validates the enum
		// itself. See #532.
		if auth, _ := m.Meta["auth"].(string); auth != "" {
			emit.WriteTOMLString(sb, "auth", auth)
		}
	}
	writeMCPSharedFields(sb, m.Meta)
	sb.WriteString("\n")
}

// writeMCPSharedFields emits description, enabled, and roots into the
// current `[mcp_servers.<name>]` table. Roots renders as an array of
// inline tables.
//
// The spec's `disabled` field inverts to Codex's own `enabled` key:
// `learn.chatgpt.com/docs/config-file/config-reference` documents
// `mcp_servers.<id>.enabled: boolean`, not `disabled`. Codex parses no
// `disabled` key at all, so writing one here would silently fail to stop
// the server. `enabled` only emits when the spec asks for `disabled:
// true`; Codex's own default (enabled) needs no explicit key.
func writeMCPSharedFields(sb *strings.Builder, meta map[string]any) {
	if desc, _ := meta["description"].(string); desc != "" {
		emit.WriteTOMLString(sb, "description", desc)
	}
	if disabled, _ := meta["disabled"].(bool); disabled {
		sb.WriteString("enabled = false\n")
	}
	raw, _ := meta["roots"].([]any)
	if len(raw) == 0 {
		return
	}
	sb.WriteString("roots = [")
	first := true
	for _, r := range raw {
		root, _ := r.(map[string]any)
		uri, _ := root["uri"].(string)
		if uri == "" {
			continue
		}
		if !first {
			sb.WriteString(", ")
		}
		first = false
		sb.WriteString(`{ uri = "` + emit.EscapeTOMLBasic(uri) + `"`)
		if name, _ := root["name"].(string); name != "" {
			sb.WriteString(`, name = "` + emit.EscapeTOMLBasic(name) + `"`)
		}
		sb.WriteString(" }")
	}
	sb.WriteString("]\n")
}

// hookCommands normalizes a spec's `command:` value (string, []string,
// or []any) into a slice of non-empty strings. Mirrors the helper used
// by the claude + gemini adapters; kept private here to avoid a shared
// import cycle.
func hookCommands(raw any) []string {
	switch v := raw.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}
