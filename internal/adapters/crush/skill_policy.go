package crush

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// userOnly keeps a skill out of the model's list and puts it in the
// command palette: Crush needs both fields for that (README, "User-Invocable
// Skills", since v0.71.0).
var userOnly = emit.UserOnlySkill{
	Target: target,
	Keys:   []string{"disable-model-invocation", "user-invocable"},
	Values: map[string]any{"disable-model-invocation": true, "user-invocable": true},
}

// SkillMarkdown renders s with the fields that keep it user-run.
func (Adapter) SkillMarkdown(s spec.Entry) string { return userOnly.Markdown(s) }
