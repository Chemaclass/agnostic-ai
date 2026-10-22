package emit

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// UserOnlySkill is a target's own SKILL.md fields that keep a skill out
// of the model's reach while the user can still run it by name.
type UserOnlySkill struct {
	Target string
	Keys   []string
	Values map[string]any
}

// Markdown renders s as SkillMarkdown does and, when s sets
// disable-model-invocation: true, adds the target's own fields. A value
// the spec or x-<target> sets for one of those keys wins.
func (u UserOnlySkill) Markdown(s spec.Entry) string {
	manual, _ := s.Meta["disable-model-invocation"].(bool)
	if resolved, _ := ResolveMeta(s.Meta, u.Target)["disable-model-invocation"].(bool); !manual || !resolved {
		return SkillMarkdown(s, u.Target)
	}
	custom, _ := s.Meta[XPrefix+u.Target].(map[string]any)
	merged := make(map[string]any, len(custom)+len(u.Keys))
	for k, v := range custom {
		merged[k] = v
	}
	keys := make([]string, 0, len(u.Keys))
	for _, k := range u.Keys {
		_, explicit := custom[k]
		// A spec's own value, such as user-invocable: false, is the
		// author's choice too.
		_, own := s.Meta[k]
		if !explicit && (!own || k == "disable-model-invocation") {
			merged[k] = u.Values[k]
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return SkillMarkdown(s, u.Target)
	}
	out := s
	out.Meta = make(map[string]any, len(s.Meta))
	for k, v := range s.Meta {
		out.Meta[k] = v
	}
	out.Meta[XPrefix+u.Target] = merged
	return SkillMarkdown(out, u.Target)
}

// Shared reports whether another target writes dir, where every writer
// agrees on the SKILL.md SkillMarkdown renders.
func (u UserOnlySkill) Shared(sess *Session, dir string) bool {
	return sess.SkillsDirWrittenByOthers(dir, u.Target)
}

// Coverage is caps.SkillFields for a sync that writes skills into dir:
// the target's own fields handle disable-model-invocation, unless dir is
// shared and the field is noted as dropped.
func (u UserOnlySkill) Coverage(sess *Session, dir string, coverage SkillFieldCoverage) SkillFieldCoverage {
	if u.Shared(sess, dir) {
		coverage.Markdown = func(s spec.Entry) string { return SkillMarkdown(s, u.Target) }
		return coverage
	}
	coverage.Markdown = u.Markdown
	coverage.Handled = append(append([]string(nil), coverage.Handled...), "disable-model-invocation")
	return coverage
}

// WriteFolders writes skills into dir with the target's own fields, or
// with the common SKILL.md when another target writes dir too.
func (u UserOnlySkill) WriteFolders(sess *Session, skills []spec.Entry, dir string, dryRun bool) error {
	render := u.Markdown
	if u.Shared(sess, dir) {
		render = func(s spec.Entry) string { return SkillMarkdown(s, u.Target) }
	}
	for _, s := range skills {
		folder := filepath.Join(dir, s.Name)
		if err := sess.WriteFile(filepath.Join(folder, "SKILL.md"), WithHeader(render(s), FormatMarkdown), dryRun); err != nil {
			return err
		}
		if err := sess.PropagateSkillAssets(s, folder, SkipSKILLMd, dryRun); err != nil {
			return err
		}
	}
	return nil
}
