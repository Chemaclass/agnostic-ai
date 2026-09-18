package cli

import (
	"path/filepath"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// claudeLayout is where a project's Claude Code files actually live.
// `sync` honors outputs.claude.dir and the per-kind keys, so import and
// doctor have to read the tree sync wrote rather than the default one,
// or a project that moved the directory cannot round-trip (#852).
type claudeLayout struct {
	dir, rules, commands, agents, skills string
}

// defaultClaudeLayout is the layout with no overrides configured. It is
// what every caller gets when no config is available.
func defaultClaudeLayout() claudeLayout {
	return claudeLayout{
		dir:      claudeDir,
		rules:    filepath.Join(claudeDir, "rules"),
		commands: filepath.Join(claudeDir, "commands"),
		agents:   filepath.Join(claudeDir, "agents"),
		skills:   filepath.Join(claudeDir, "skills"),
	}
}

// claudeLayoutFor resolves the layout from cfg by asking the adapter
// where it writes, rather than re-deriving the override rules here.
// The adapter's own NativeArtifacts is the single source of truth, so
// a future per-kind key cannot move emission without moving import
// with it.
func claudeLayoutFor(cfg *config.Config) claudeLayout {
	layout := defaultClaudeLayout()
	if cfg == nil {
		return layout
	}
	if out, ok := cfg.Outputs["claude"]; ok && out.Dir != "" {
		layout.dir = filepath.Clean(filepath.FromSlash(out.Dir))
	}
	for _, artifact := range adapters.NativeArtifactsFor("claude", cfg) {
		dir := filepath.Clean(strings.TrimSuffix(artifact.Location, "/"))
		switch artifact.Label {
		case "Rules":
			layout.rules = dir
		case "Commands":
			layout.commands = dir
		case "Agents":
			layout.agents = dir
		case "Skills":
			layout.skills = dir
		}
	}
	return layout
}
