package gemini

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// resolvedAgentKeys names the fields agentMarkdown reads off the
// resolved meta, excluded from the x-gemini passthrough merge only so
// no key emits twice. Excluding them is not a ban on overriding them:
// ResolveMeta has already flattened `x-gemini.model` and friends onto
// the values read below. `tools` is deliberately absent from this list,
// since it is read from the raw meta instead and reaches the
// frontmatter through the merge when set there (see xGeminiSetsTools).
// `mcpServers` is listed so the name Gemini's docs show never reaches
// the file: agentMCPServers writes it as `mcp_servers` instead.
var resolvedAgentKeys = []string{"name", "description", "kind", "model", "temperature", "max_turns", "timeout_mins", "mcp_servers", "mcpServers"}

// geminiToolName maps agnostic-ai's Claude-style tool identifiers onto
// Gemini CLI's own tool names (geminicli.com/docs/reference/tools, whose
// "Available tools" tables name each one). The vocabularies share no
// spelling at all: Gemini's are snake_case, the terminal tool is
// `run_shell_command`, edit is `replace`, and search is `grep_search`
// ("Legacy alias: `search_file_content`"). A `tools` list carried over
// from a Claude-shaped spec would therefore restrict the subagent to
// tools that do not exist, which is worse than the vendor's documented
// default for an absent list ("If omitted, it inherits all tools from
// the parent session"). See the package doc.
var geminiToolName = map[string]string{
	"Read":      "read_file",
	"Write":     "write_file",
	"Edit":      "replace",
	"Glob":      "glob",
	"Grep":      "grep_search",
	"Bash":      "run_shell_command",
	"WebFetch":  "web_fetch",
	"WebSearch": "google_web_search",
}

// EmitAgents writes one native subagent per agent at
// `<dir>/<name>.md`: "Custom agents are defined as Markdown files
// (`.md`) with YAML frontmatter ... Project-level: `.gemini/agents/*.md`"
// (geminicli.com/docs/core/subagents). The body is the agent's system
// prompt.
//
// Agents that declare a `tools` list Gemini's own vocabulary has no
// entry for fold into one coverage note per sync rather than emitting a
// restriction that silences the subagent.
func (Adapter) EmitAgents(sess *emit.Session, agents []spec.Entry, dir string, dryRun bool) error {
	unmappedTools, renamedMCP := 0, 0
	for _, a := range agents {
		md, hasUnmapped, renamed := agentMarkdown(a)
		if hasUnmapped {
			unmappedTools++
		}
		if renamed {
			renamedMCP++
		}
		path := filepath.Join(dir, a.Name+".md")
		if err := sess.WriteFile(path, emit.WithHeader(md, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", unmappedTools,
		"value(s) outside agnostic-ai's Read/Write/Edit/Bash/Grep/Glob/WebFetch/WebSearch set have no confirmed Gemini tool name; set x-gemini.tools directly for those, or drop the field to inherit every tool from the parent session")
	if renamedMCP > 0 {
		emit.NoteProject(fmt.Sprintf("gemini: x-gemini.mcpServers on %d agent spec(s) is written as mcp_servers, the only per-agent MCP key Gemini's agent loader accepts; rename it in the spec", renamedMCP))
	}
	return nil
}

// agentMarkdown renders one `.gemini/agents/<name>.md` file. `name` and
// `description` are the two required frontmatter fields, so both always
// emit (`description` falls back to the spec name). `kind`, `model`,
// `temperature`, `max_turns`, and `timeout_mins` pass through verbatim
// when declared, and `tools` translates onto Gemini's own vocabulary
// (see geminiToolName). `mcp_servers` (inline per-agent MCP servers)
// passes through too; see agentMCPServers. Arbitrary `x-gemini` keys
// merge on top.
//
// `tools` is read from the raw meta rather than the resolved map for
// the same reason kiro's agent renderer does: ResolveMeta would already
// have flattened an `x-gemini.tools` override onto it, and running that
// value back through the Claude-style table would misread Gemini's own
// vocabulary as unmapped. xGeminiSetsTools guards that case explicitly,
// so an explicit override always wins outright instead of merging
// alongside a translated value.
func agentMarkdown(a spec.Entry) (body string, hasUnmappedTools, renamedMCP bool) {
	resolved := emit.ResolveMeta(a.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = a.Name
	}
	meta := map[string]any{"name": a.Name, "description": desc}
	keys := []string{"name", "description"}
	for _, k := range []string{"kind", "model"} {
		if v, _ := resolved[k].(string); v != "" {
			meta[k] = v
			keys = append(keys, k)
		}
	}
	if !xGeminiSetsTools(a.Meta) {
		if raw := emit.StringSlice(a.Meta["tools"]); len(raw) > 0 {
			mapped, unmapped := translateTools(raw)
			if len(mapped) > 0 {
				meta["tools"] = mapped
				keys = append(keys, "tools")
			}
			hasUnmappedTools = unmapped
		}
	}
	if v, ok := resolved["temperature"]; ok && isNumber(v) {
		meta["temperature"] = v
		keys = append(keys, "temperature")
	}
	for _, k := range []string{"max_turns", "timeout_mins"} {
		if v, ok := emit.IntField(resolved, k); ok {
			meta[k] = v
			keys = append(keys, k)
		}
	}
	servers, renamedMCP := agentMCPServers(a.Meta, resolved)
	if servers != nil {
		meta["mcp_servers"] = servers
		keys = append(keys, "mcp_servers")
	}
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, resolvedAgentKeys...)
	front := emit.FrontmatterOrdered(meta, keys)
	trimmed := strings.TrimSpace(a.Body)
	if trimmed == "" {
		return front + "\n", hasUnmappedTools, renamedMCP
	}
	return front + "\n" + trimmed + "\n", hasUnmappedTools, renamedMCP
}

// agentMCPServers returns the agent's inline MCP servers. Gemini's
// agent loader validates frontmatter with a strict schema whose key is
// `mcp_servers` (packages/core/src/agents/agentLoader.ts), so any other
// key makes it reject the whole agent. Its subagents docs still show
// `mcpServers`, so an `x-gemini.mcpServers` written from them is
// renamed and reported through renamed. A top-level `mcpServers` is
// left alone: it is Claude's per-agent field, with a different shape.
func agentMCPServers(meta, resolved map[string]any) (servers any, renamed bool) {
	x, _ := emit.CustomTargetMeta(meta, target)
	if _, ok := x["mcp_servers"]; !ok {
		if v, ok := x["mcpServers"]; ok {
			return v, true
		}
	}
	return resolved["mcp_servers"], false
}

// translateTools maps a spec's generic Claude-style tools list onto
// Gemini's own tool names, deduplicated in first-seen order. A name with
// no table entry is left out and reported through hasUnmapped rather
// than written verbatim, since an unknown name in this list restricts
// the subagent to a tool that does not exist.
func translateTools(names []string) (mapped []string, hasUnmapped bool) {
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		tool, ok := geminiToolName[n]
		if !ok {
			hasUnmapped = true
			continue
		}
		if seen[tool] {
			continue
		}
		seen[tool] = true
		mapped = append(mapped, tool)
	}
	return mapped, hasUnmapped
}

// xGeminiSetsTools reports whether the spec carries an explicit
// x-gemini.tools override: the one channel this adapter trusts to
// already be Gemini's own tool vocabulary (including the documented
// `*`, `mcp_*`, and `mcp_<server>_*` wildcards) rather than
// agnostic-ai's generic Claude-style names.
func xGeminiSetsTools(meta map[string]any) bool {
	x, _ := emit.CustomTargetMeta(meta, target)
	if x == nil {
		return false
	}
	_, tools := x["tools"]
	return tools
}

// isNumber reports whether v is a YAML-decoded number. `temperature` is
// the one fractional field in Gemini's agent schema (0.0 to 2.0), so it
// cannot route through IntField like max_turns and timeout_mins.
func isNumber(v any) bool {
	switch v.(type) {
	case int, int64, float64:
		return true
	}
	return false
}
