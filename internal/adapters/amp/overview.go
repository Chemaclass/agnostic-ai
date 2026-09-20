package amp

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
)

// NativeArtifacts describes where Amp reads each generated artifact,
// honoring the same outputs.amp.* overrides Emit resolves. Rules are
// absent: Amp reads them through the entry-point pointer to the source
// specs. Commands are absent too: Amp has no file-based command
// surface (see the package doc comment and #553), so a Command spec
// warns instead of appearing here. Agents are absent for the same
// reason since Amp removed custom commands (#727); the merged
// rules-file carries them when the user opts into it. Settings have no
// row of their own: they merge into the same `.amp/settings.json` the
// MCP row already names, so a second row would repeat the path.
func (Adapter) NativeArtifacts(cfg *config.Config) []emit.NativeArtifact {
	return []emit.NativeArtifact{
		{Label: "Skills", Location: emit.OutputSkillsDir(cfg, target, defaultSkillsDir) + "/"},
		{Label: "MCP servers", Location: emit.OutputMCPFile(cfg, target, defaultMCPFile)},
	}
}
