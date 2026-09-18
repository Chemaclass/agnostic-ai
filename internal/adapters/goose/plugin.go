package goose

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

// pluginSkillsRoot returns the plugin root a skills directory belongs
// to, or "" when it lands outside one. Goose loads a plugin's skills
// from its own `skills/` component directory, so
// `.agents/plugins/<name>/skills` is the only layout that makes the
// folders plugin skills.
func pluginSkillsRoot(skillsDir string) string {
	return emit.PluginRootOf(skillsDir, map[string]bool{"skills": true})
}
