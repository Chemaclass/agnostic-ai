package trae

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitAgents writes one native `<dir>/<name>.md` per agent spec and
// sweeps the rule-form file a prior sync left behind for the same name
// (including the scoped copy, since rule-form agents nested under
// `<rules-dir>/<scope>/`).
func emitAgents(sess *emit.Session, agents []spec.Entry, dir, rulesDir string, dryRun bool) error {
	droppedModel := 0
	for _, a := range agents {
		md, dropped := agentMarkdown(a)
		if dropped {
			droppedModel++
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
	emit.NoteFieldNoOp(target, spec.KindAgent, "model", droppedModel,
		"Trae accepts only its own built-in model IDs; scope the value with model: {trae: <id>} or x-trae.model to reach the file")
	return nil
}

// legacyAgentPaths returns the rule-form file names a prior sync wrote
// for this agent: the flat one, plus the nested copy a scoped agent got
// before agents moved to their own directory.
func legacyAgentPaths(a spec.Entry, rulesDir string) []string {
	name := legacyAgentPrefix + a.Name + ".md"
	paths := []string{filepath.Join(rulesDir, name)}
	if s := a.EffectiveScope(); s != "" {
		paths = append(paths, filepath.Join(rulesDir, s, name))
	}
	return paths
}

// agentMarkdown renders one `.trae/agents/<name>.md` subagent file and
// reports whether a generic `model` was dropped for lack of a Trae
// value. Trae's documented frontmatter is `name` and `description`
// (both required), plus optional `model`, `tools`, `disallowedTools`,
// and `mcpServers` (docs.trae.ai/ide/subagents); the body after the
// closing delimiter is the system prompt.
//
// `tools` passes through unmapped, comma-joined: Trae's vocabulary is
// Claude-style and covers agnostic-ai's set exactly (Bash, Edit, Glob,
// Grep, Read, Write, WebFetch, WebSearch, plus Skill, LSP, TodoWrite
// and `mcp__<server>__<tool>`), and the vendor spells the value as a
// comma-separated string rather than a YAML list. `disallowedTools` and
// `mcpServers` have no generic spec field and reach the file through
// `x-trae`, like any key Trae adds next.
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
	model, _ := resolved["model"].(string)
	droppedModel := model != "" && !modelIsTraeScoped(a.Meta)
	if model != "" && !droppedModel {
		meta["model"] = model
		keys = append(keys, "model")
	}
	if tools := emit.StringSlice(resolved["tools"]); len(tools) > 0 {
		meta["tools"] = strings.Join(tools, ", ")
		keys = append(keys, "tools")
	}
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, "name", "description", "model", "tools")
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(a.Body)
	if body == "" {
		return front + "\n", droppedModel
	}
	return front + "\n" + body + "\n", droppedModel
}

// modelIsTraeScoped reports whether the spec named a model for Trae
// specifically, through `x-trae.model` or `model: {trae: <id>}`. Only
// those two channels carry a value from Trae's own built-in model
// table; a bare cross-target `model:` carries a name Trae does not
// accept ("Only built-in models provided by TraeCode are supported"),
// so it never reaches the file.
func modelIsTraeScoped(meta map[string]any) bool {
	if x, _ := emit.CustomTargetMeta(meta, target); x != nil {
		if _, ok := x["model"]; ok {
			return true
		}
	}
	perTarget, ok := meta["model"].(map[string]any)
	if !ok {
		return false
	}
	_, ok = perTarget[target]
	return ok
}
