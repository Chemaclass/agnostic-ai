package codex

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// openaiYAMLKeys are the top-level fields the Codex CLI reads from
// `<skill>/agents/openai.yaml`. Only these pass through; anything else
// under `x-codex` is ignored to keep the emitted file faithful to the
// documented schema.
var openaiYAMLKeys = []string{"interface", "policy", "dependencies"}

// emitSkill writes the per-skill folder layout for the Codex CLI:
//
//	<skillsDir>/<name>/
//	  SKILL.md           (required, frontmatter + body)
//	  agents/openai.yaml (optional, when x-codex provides UI/policy/deps)
//	  <attached files>   (any sibling files / subdirs in the source skill)
//
// The SKILL.md frontmatter is reduced to the two fields Codex requires
// (`name`, `description`) so the file stays valid against the published
// schema regardless of what extra metadata the source spec carries.
//
// Any other files or subdirectories that live next to the source
// SKILL.md (helper scripts, fixtures, additional docs) are propagated
// byte-for-byte into the emitted skill folder. The optional Codex
// `agents/openai.yaml` overlay derived from `x-codex` is written last
// so a spec-authored value wins over any verbatim copy of the same
// path from the source tree.
func emitSkill(s spec.Entry, skillsDir string, dryRun bool) error {
	folder := filepath.Join(skillsDir, s.Name)

	if err := emit.WriteFile(filepath.Join(folder, "SKILL.md"), emit.WithHeader(skillMarkdown(s), emit.FormatMarkdown), dryRun); err != nil {
		return err
	}

	if err := propagateSkillAssets(s, folder, dryRun); err != nil {
		return err
	}

	if yamlBody := openaiYAML(s); yamlBody != "" {
		if err := emit.WriteFile(filepath.Join(folder, "agents", "openai.yaml"), emit.WithHeader(yamlBody, emit.FormatYAML), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// propagateSkillAssets mirrors every sibling file under the source
// skill directory into the emitted folder, skipping SKILL.md because
// the adapter re-renders it from the spec frontmatter. No-op when the
// source path is unknown (in-memory specs from the playground).
func propagateSkillAssets(s spec.Entry, dstDir string, dryRun bool) error {
	if s.Path == "" {
		return nil
	}
	srcDir := filepath.Dir(s.Path)
	return emit.CopyTree(srcDir, dstDir, func(rel string) bool {
		return rel == "SKILL.md"
	}, dryRun)
}

func skillMarkdown(s spec.Entry) string {
	// Resolve description through the per-target meta so a spec
	// carrying `x-codex.description` wins over the (claude-side) top-
	// level value. Without this, every divergent skill imported via
	// PR #310 still emitted the claude description (#312).
	resolved := emit.ResolveMeta(s.Meta, target)
	desc, _ := resolved["description"].(string)
	if desc == "" {
		desc = s.Name
	}
	front := emit.FrontmatterOrdered(map[string]any{
		"name":        s.Name,
		"description": desc,
	}, []string{"name", "description"})
	body := strings.TrimSpace(s.Body)
	if body == "" {
		return front + "\n"
	}
	return front + "\n" + body + "\n"
}

// openaiYAML returns the YAML body for the optional agents/openai.yaml
// file, or "" when the spec carries no Codex-specific UI, policy, or
// dependency overrides. The provenance header is applied by the caller
// via emit.WithHeader so the marker lives in one place.
func openaiYAML(s spec.Entry) string {
	x, ok := s.Meta["x-codex"].(map[string]any)
	if !ok || len(x) == 0 {
		return ""
	}
	out := map[string]any{}
	for _, k := range openaiYAMLKeys {
		if v, ok := x[k]; ok {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return ""
	}
	data, err := yaml.Marshal(out)
	if err != nil {
		return ""
	}
	return string(data)
}
