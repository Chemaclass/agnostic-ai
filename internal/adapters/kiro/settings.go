package kiro

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// permissionsNoOpReason: Kiro's workspace settings have no allow, deny,
// or ask list; tool trust lives in each agent file.
const permissionsNoOpReason = "Kiro's .kiro/settings/cli.json has no allow, deny, or ask list; set tool trust per agent"

// RendersSpecFile reports whether a spec produces output for Kiro. A
// settings spec does only when it gives Kiro a model it can load.
func (Adapter) RendersSpecFile(e spec.Entry, _ *config.Config, _ string) bool {
	if e.Kind != spec.KindSettings {
		return true
	}
	settings := emit.WithoutForeignClaudeModels(caps, spec.Bundle{Settings: []spec.Entry{e}}).Settings
	return emit.SettingsModel(settings, target) != ""
}

// emitSettings merges the settings model into chat.defaultModel in
// .kiro/settings/cli.json, a flat dotted-key file. Every other key stays.
func emitSettings(sess *emit.Session, settings []spec.Entry, dryRun bool) error {
	model := emit.SettingsModel(settings, target)
	if model == "" {
		return nil
	}
	return sess.MergeJSONFile(defaultSettingsFile, map[string]any{"chat.defaultModel": model}, dryRun)
}
