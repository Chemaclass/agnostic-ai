package cli

import "github.com/chemaclass/agnostic-ai/internal/spec"

// lintSkillScopeKey flags a `scope:` key in a skill's frontmatter
// (LINT018, warning). A skill's scope comes from its folder, so the key
// changes nothing and reaches Claude Code's SKILL.md as an unknown field.
func lintSkillScopeKey(skills []spec.Entry) []lintFinding {
	var out []lintFinding
	for _, s := range skills {
		if _, ok := s.Meta["scope"]; ok {
			out = append(out, lintFinding{
				Code: "LINT018", Severity: lintWarn, Path: s.Path,
				Message: "`scope` has no effect on a skill: move the folder under skills/<scope>/ to scope it, or list extra Cursor workspaces under `workspaces`",
			})
		}
	}
	return out
}
