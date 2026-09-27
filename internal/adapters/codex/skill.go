package codex

import (
	"maps"
	"path/filepath"
	"slices"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// emitSkill writes the per-skill folder layout for the Codex CLI:
//
//	<skillsDir>/<name>/
//	  SKILL.md           (required, frontmatter + body)
//	  agents/openai.yaml (optional, from x-codex UI/policy/deps or a manual-only skill)
//	  <attached files>   (any sibling files / subdirs in the source skill)
//
// The SKILL.md frontmatter is reduced to the two fields Codex requires
// (`name`, `description`) so the file stays valid against the published
// schema regardless of what extra metadata the source spec carries.
//
// Any other files or subdirectories that live next to the source
// SKILL.md (helper scripts, fixtures, additional docs) are propagated
// byte-for-byte into the emitted skill folder. The optional Codex
// `agents/openai.yaml` is rendered instead of copied when it merges a
// bundled copy with x-codex or the manual-only policy.
func emitSkill(sess *emit.Session, s spec.Entry, skillsDir string, dryRun bool) error {
	folder := filepath.Join(skillsDir, s.Name)

	if err := sess.WriteFile(filepath.Join(folder, "SKILL.md"), emit.WithHeader(skillMarkdown(s), emit.FormatMarkdown), dryRun); err != nil {
		return err
	}

	sidecars, err := skillSidecars(s)
	if err != nil {
		return err
	}
	skip := func(rel string) bool {
		_, rendered := sidecars[filepath.FromSlash(rel)]
		return emit.SkipSKILLMd(rel) || rendered
	}
	if err := sess.PropagateSkillAssets(s, folder, skip, dryRun); err != nil {
		return err
	}

	for _, rel := range slices.Sorted(maps.Keys(sidecars)) {
		if err := sess.WriteFile(filepath.Join(folder, rel), sidecars[rel], dryRun); err != nil {
			return err
		}
	}
	return nil
}

// skillSidecars returns the files Codex reads beside SKILL.md, keyed by
// path relative to the skill folder. Project and global sync both write
// them, so a policy set once reaches Codex from either scope.
func skillSidecars(s spec.Entry) (map[string]string, error) {
	yamlBody, err := emit.OpenAIYAML(s)
	if err != nil || yamlBody == "" {
		return nil, err
	}
	return map[string]string{filepath.FromSlash(emit.OpenAIYAMLRel): emit.WithHeader(yamlBody, emit.FormatYAML)}, nil
}

// skillMarkdown renders SKILL.md through the shared renderer, excluding
// the keys routed to agents/openai.yaml so nothing is emitted twice.
func skillMarkdown(s spec.Entry) string {
	return emit.SkillMarkdown(s, target, emit.OpenAIYAMLKeys...)
}

func (Adapter) SkillMarkdown(skill spec.Entry) string {
	return skillMarkdown(skill)
}

func (Adapter) SkillSidecars(skill spec.Entry) (map[string]string, error) {
	return skillSidecars(skill)
}

// SkillInvocationPolicySet reports whether the skill's openai.yaml
// sets allow_implicit_invocation, the only invocation marker Codex reads.
func (Adapter) SkillInvocationPolicySet(skill spec.Entry) bool {
	return emit.SkillOpenAIYAMLPolicySet(skill)
}
