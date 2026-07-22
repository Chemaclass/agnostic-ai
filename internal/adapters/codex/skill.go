package codex

import (
	"path/filepath"

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
func emitSkill(sess *emit.Session, s spec.Entry, skillsDir string, dryRun bool) error {
	folder := filepath.Join(skillsDir, s.Name)

	if err := sess.WriteFile(filepath.Join(folder, "SKILL.md"), emit.WithHeader(skillMarkdown(s), emit.FormatMarkdown), dryRun); err != nil {
		return err
	}

	if err := sess.PropagateSkillAssets(s, folder, emit.SkipSKILLMd, dryRun); err != nil {
		return err
	}

	if yamlBody := openaiYAML(s); yamlBody != "" {
		if err := sess.WriteFile(filepath.Join(folder, "agents", "openai.yaml"), emit.WithHeader(yamlBody, emit.FormatYAML), dryRun); err != nil {
			return err
		}
	}
	return nil
}

// skillMarkdown renders SKILL.md through the shared renderer, excluding
// the keys routed to agents/openai.yaml so nothing is emitted twice.
func skillMarkdown(s spec.Entry) string {
	return emit.SkillMarkdown(s, target, openaiYAMLKeys...)
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
