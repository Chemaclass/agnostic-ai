package codex

import (
	"maps"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// renderConfigTOML builds the `.codex/config.toml` body from the bundle's
// hook and MCP entries plus any first-class config fields. Each MCP entry
// emits as a `[mcp_servers.<name>]` table; each hook entry emits as an
// `[[hooks.<event>]]` array-of-tables element. Returns "" when there is no
// valid output to write.
func renderConfigTOML(hooks, mcps []spec.Entry, cfg *config.CodexConfig) string {
	byEvent := groupHooksByEvent(hooks)
	hasContent := len(byEvent) > 0 || anyNamedMCP(mcps) || hasCodexConfig(cfg)
	if !hasContent {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(emit.Header(emit.FormatTOML) + "\n")

	writeCodexConfigFields(&sb, cfg)
	writeMCPServers(&sb, mcps)
	writeHookSectionsFromMap(&sb, byEvent)
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
// when set in `outputs.codex.config`.
func writeCodexConfigFields(sb *strings.Builder, cfg *config.CodexConfig) {
	if cfg == nil {
		return
	}
	if cfg.Model != "" {
		emit.WriteTOMLString(sb, "model", cfg.Model)
	}
	if cfg.Sandbox != "" {
		emit.WriteTOMLString(sb, "sandbox", cfg.Sandbox)
	}
	if cfg.ApprovalPolicy != "" {
		emit.WriteTOMLString(sb, "approval_policy", cfg.ApprovalPolicy)
	}
	if cfg.ModelReasoningEffort != "" {
		emit.WriteTOMLString(sb, "model_reasoning_effort", cfg.ModelReasoningEffort)
	}
	if cfg.ModelReasoningSummary != "" {
		emit.WriteTOMLString(sb, "model_reasoning_summary", cfg.ModelReasoningSummary)
	}
	if len(cfg.Notify) > 0 {
		emit.WriteTOMLStringArray(sb, "notify", cfg.Notify)
	}
	if cfg.HistoryPersistence != "" {
		sb.WriteString("\n[history]\n")
		emit.WriteTOMLString(sb, "persistence", cfg.HistoryPersistence)
	}
	writeCodexModelProviders(sb, cfg.ModelProviders)
	writeCodexProfiles(sb, cfg.Profiles)
	if hasCodexConfig(cfg) {
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
// transport-appropriate keys plus the shared description/disabled/roots
// fields that mirror the `.mcp.json` schema.
func writeMCPServerTable(sb *strings.Builder, m spec.Entry) {
	sb.WriteString("[mcp_servers." + m.Name + "]\n")

	transport, _ := m.Meta["type"].(string)
	if transport == "" {
		transport = "stdio"
	}
	switch transport {
	case "stdio":
		if cmd, _ := m.Meta["command"].(string); cmd != "" {
			emit.WriteTOMLString(sb, "command", cmd)
		}
		if args := emit.StringSlice(m.Meta["args"]); len(args) > 0 {
			emit.WriteTOMLStringArray(sb, "args", args)
		}
		emit.WriteTOMLInlineStringTable(sb, "env", emit.StringMap(m.Meta["env"]))
	case "http", "sse":
		if url, _ := m.Meta["url"].(string); url != "" {
			emit.WriteTOMLString(sb, "url", url)
		}
		if t, _ := m.Meta["bearer_token_env_var"].(string); t != "" {
			emit.WriteTOMLString(sb, "bearer_token_env_var", t)
		}
		emit.WriteTOMLInlineStringTable(sb, "http_headers", emit.StringMap(m.Meta["headers"]))
	}
	writeMCPSharedFields(sb, m.Meta)
	sb.WriteString("\n")
}

// writeMCPSharedFields emits description, disabled, and roots — the keys
// .mcp.json supports across every transport — into the current
// `[mcp_servers.<name>]` table. Roots renders as an array of inline tables.
func writeMCPSharedFields(sb *strings.Builder, meta map[string]any) {
	if desc, _ := meta["description"].(string); desc != "" {
		emit.WriteTOMLString(sb, "description", desc)
	}
	if disabled, _ := meta["disabled"].(bool); disabled {
		sb.WriteString("disabled = true\n")
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

func writeHookSectionsFromMap(sb *strings.Builder, byEvent map[string][]spec.Entry) {
	for _, event := range slices.Sorted(maps.Keys(byEvent)) {
		for _, h := range byEvent[event] {
			writeHookSection(sb, event, h)
		}
	}
}

func groupHooksByEvent(hooks []spec.Entry) map[string][]spec.Entry {
	out := map[string][]spec.Entry{}
	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		if event == "" {
			continue
		}
		out[event] = append(out[event], h)
	}
	return out
}

func writeHookSection(sb *strings.Builder, event string, h spec.Entry) {
	sb.WriteString("[[hooks." + event + "]]\n")
	if matcher, _ := h.Meta["matcher"].(string); matcher != "" {
		emit.WriteTOMLString(sb, "matcher", matcher)
	}
	if cmd, _ := h.Meta["command"].(string); cmd != "" {
		emit.WriteTOMLString(sb, "command", cmd)
	}
	sb.WriteString("\n")
}
