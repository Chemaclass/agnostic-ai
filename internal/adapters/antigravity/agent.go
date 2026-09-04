package antigravity

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// modelTiers are the only three values Antigravity documents for a
// subagent's `model` key: "Model tier used when invoked (`inherit`,
// `flash`, or `pro`)" (antigravity.google/docs/subagents). It is a
// closed tier enum, not a model ID, so a cross-target `model: sonnet`
// names nothing here and is dropped rather than written dead.
var modelTiers = map[string]bool{"inherit": true, "flash": true, "pro": true}

// emitAgents writes one native `<dir>/<name>.md` per agent spec and
// sweeps the rule-form file a prior sync left behind for the same name,
// at both the current and the pre-plural rules directory.
func emitAgents(sess *emit.Session, agents []spec.Entry, dir string, rulesDirs []string, dryRun bool) error {
	droppedTools, droppedModel := 0, 0
	for _, a := range agents {
		md, drops := agentMarkdown(a)
		if drops.tools {
			droppedTools++
		}
		if drops.model {
			droppedModel++
		}
		path := filepath.Join(dir, a.Name+".md")
		if err := sess.WriteFile(path, emit.WithHeader(md, emit.FormatMarkdown), dryRun); err != nil {
			return err
		}
		for _, rulesDir := range rulesDirs {
			legacy := filepath.Join(rulesDir, legacyAgentPrefix+a.Name+".md")
			if err := sess.RemoveGenerated(legacy, dryRun); err != nil {
				return err
			}
		}
	}
	emit.NoteFieldNoOp(target, spec.KindAgent, "tools", droppedTools,
		"Antigravity's tool vocabulary (view_file, run_command, ...) shares no name with agnostic-ai's, and the vendor warns an unmapped name can hang the subagent; set x-antigravity.tools directly")
	emit.NoteFieldNoOp(target, spec.KindAgent, "model", droppedModel,
		"Antigravity's model field is the tier enum inherit/flash/pro, not a model ID")
	return nil
}

// agentDrops records which fields agentMarkdown left out of a file so
// the caller can fold each into one coverage note per sync.
type agentDrops struct {
	tools bool
	model bool
}

// agentMarkdown renders one `.agents/agents/<name>.md` subagent file.
// Antigravity's documented frontmatter is `name` and `description`
// (both required) plus `tools`, `mainAgent`, `subagent`, `model`,
// `commandExecutionPolicy`, `mcpServers`, and `skills`/`plugins`
// (antigravity.google/docs/subagents); "The content following the YAML
// `---` delimiter defines the subagent's system prompt."
//
// A generic `tools` list never reaches the file. Antigravity's own
// vocabulary is `view_file`, `replace_file_content`, `grep_search`,
// `run_command` and friends, sharing no name with agnostic-ai's
// Claude-style set, and the page carries a boxed warning: "Specifying
// an unmapped or misspelled tool name in the `tools` list may cause the
// subagent process to hang during execution." `x-antigravity.tools` is
// the one channel trusted to already hold Antigravity's vocabulary.
// Every other documented key reaches the file the same way.
func agentMarkdown(a spec.Entry) (string, agentDrops) {
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
	var drops agentDrops
	if model, _ := resolved["model"].(string); model != "" {
		if modelTiers[model] {
			meta["model"] = model
			keys = append(keys, "model")
		} else {
			drops.model = true
		}
	}
	drops.tools = len(emit.StringSlice(a.Meta["tools"])) > 0 && !xAntigravitySetsTools(a.Meta)
	emit.MergeCustomTargetMeta(meta, &keys, a.Meta, target, "name", "description", "model")
	front := emit.FrontmatterOrdered(meta, keys)
	body := strings.TrimSpace(a.Body)
	if body == "" {
		return front + "\n", drops
	}
	return front + "\n" + body + "\n", drops
}

// xAntigravitySetsTools reports whether the spec already carries an
// explicit x-antigravity.tools override, the one channel this adapter
// trusts to hold Antigravity's own tool names.
func xAntigravitySetsTools(meta map[string]any) bool {
	x, _ := emit.CustomTargetMeta(meta, target)
	if x == nil {
		return false
	}
	_, ok := x["tools"]
	return ok
}
