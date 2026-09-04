package windsurf

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// devinTool maps agnostic-ai's Claude-style tool identifiers onto the
// five names Devin publishes as its complete vocabulary: "**Available
// tool names:** `read`, `edit`, `grep`, `glob`, `exec`"
// (docs.devin.ai/cli/reference/permissions). `Write` and `Edit` both
// collapse onto `edit`, which is Devin's single file-mutation tool, so
// an agent declaring only `Write` also gains edit capability. Names
// outside this table are never guessed at: they drop and fold into one
// coverage note per sync.
var devinTool = map[string]string{
	"Read":  "read",
	"Grep":  "grep",
	"Glob":  "glob",
	"Bash":  "exec",
	"Write": "edit",
	"Edit":  "edit",
}

// mcpToolPrefix marks a tool name that addresses one MCP server tool.
// Devin documents the same `mcp__<server>__<tool>` spelling this repo
// uses, so those names pass through the translation untouched.
const mcpToolPrefix = "mcp__"

// emitAgents writes one native `<dir>/<name>.md` per agent spec and
// sweeps the rule-form file a prior sync left behind for the same name
// (both the flat and the scoped copy, since windsurf routes scoped
// entries to `<scope>/<rules-dir>`). A generic `tools` list translates
// onto Devin's own vocabulary; any name with no table entry drops and
// folds into one coverage note per sync.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir, rulesDir string, dryRun bool) error {
	unmappedTools := 0
	for _, a := range agents {
		md, hasUnmapped := agentMarkdown(a)
		if hasUnmapped {
			unmappedTools++
		}
		path := filepath.Join(dir, a.Name+".md")
		if err := sess.WriteFile(path, emit.WithHeader(md, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
		for _, legacy := range legacyAgentPaths(a, rulesDir) {
			if err := sess.RemoveGenerated(legacy, dryRun); err != nil {
				return err
			}
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", unmappedTools,
		"value(s) outside agnostic-ai's Read/Write/Edit/Bash/Grep/Glob set have no documented Devin tool name; set x-windsurf.allowed-tools directly for those")
	return nil
}

// legacyAgentPaths returns the rule-form file names a prior sync wrote
// for this agent. The scoped copy is listed too because agents used to
// follow the rules dir's ScopeAtRoot routing; native subagents are flat,
// since Devin documents sub-directory discovery for rules only.
func legacyAgentPaths(a spec.Entry, rulesDir string) []string {
	name := legacyAgentPrefix + a.Name + ".md"
	paths := []string{filepath.Join(rulesDir, name)}
	if s := a.EffectiveScope(); s != "" && !emit.ScopeEscapesRoot(s) {
		paths = append(paths, filepath.Join(s, rulesDir, name))
	}
	return paths
}

// agentMarkdown renders one `.devin/agents/<name>.md` subagent file and
// reports whether the spec declared a tool with no Devin equivalent.
// Devin's documented frontmatter is `name`, `description`, `model`,
// `allowed-tools`, and `max-nesting` (docs.devin.ai/cli/subagents); the
// body after the closing delimiter is the subagent's system prompt.
//
// `model` passes through verbatim: the vendor's own worked example pins
// `model: sonnet`, the exact vocabulary a cross-target spec already
// carries. `max-nesting` has no generic spec field and reaches the file
// through `x-windsurf`, like any other key Devin adds next.
func agentMarkdown(a spec.Entry) (string, bool) {
	resolved := emit.ResolveMeta(a.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = a.Name
	}
	meta := map[string]any{
		"name":        a.Name,
		"description": desc,
	}
	keys := []string{"name", "description"}
	if model, _ := resolved["model"].(string); model != "" {
		meta["model"] = model
		keys = append(keys, "model")
	}
	var hasUnmapped bool
	if !xWindsurfSetsTools(a.Meta) {
		if raw := emit.StringSlice(a.Meta["tools"]); len(raw) > 0 {
			mapped, unmapped := translateTools(raw)
			if len(mapped) > 0 {
				meta["allowed-tools"] = mapped
				keys = append(keys, "allowed-tools")
			}
			hasUnmapped = unmapped
		}
	}
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, "name", "description", "model", "tools")
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(a.Body)
	if body == "" {
		return front + "\n", hasUnmapped
	}
	return front + "\n" + body + "\n", hasUnmapped
}

// translateTools maps a spec's Claude-style tools list onto Devin's own
// vocabulary (devinTool), deduplicated in first-seen order since Write
// and Edit collapse onto the same name. An `mcp__server__tool` name
// passes through untranslated. Anything else is left out and reported
// via hasUnmapped rather than written verbatim or dropped with no trace.
func translateTools(names []string) (mapped []string, hasUnmapped bool) {
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		out := devinTool[n]
		if out == "" && strings.HasPrefix(n, mcpToolPrefix) {
			out = n
		}
		if out == "" {
			hasUnmapped = true
			continue
		}
		if seen[out] {
			continue
		}
		seen[out] = true
		mapped = append(mapped, out)
	}
	return mapped, hasUnmapped
}

// xWindsurfSetsTools reports whether the spec already carries an
// explicit x-windsurf.allowed-tools override: the one channel this
// adapter trusts to already hold Devin's own vocabulary rather than
// agnostic-ai's generic Claude-style names.
func xWindsurfSetsTools(meta map[string]any) bool {
	x, _ := emit.CustomTargetMeta(meta, target)
	if x == nil {
		return false
	}
	_, ok := x["allowed-tools"]
	return ok
}
