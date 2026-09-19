package factory

import (
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// permissionsNoKeyReason explains, in the flushed coverage note, why
// the portable permission lists stop here. Factory's settings reference
// names `commandAllowlist`, `commandDenylist`, and `commandBlocklist`,
// but publishes neither the value grammar nor how the two deny-shaped
// keys differ, so this adapter reports the gap rather than guessing a
// spelling Droid CLI may read as something else.
const permissionsNoKeyReason = "Factory documents commandAllowlist, commandDenylist, and commandBlocklist by name only, with no rule grammar and no stated difference between the two deny keys; set them by hand in .factory/settings.json"

// emitSettings merges the portable default model into the project-tier
// `<git-root>/.factory/settings.json` (override via
// outputs.factory.conf-file). Factory documents the project tier on its
// hierarchical-settings page, not on the CLI settings page whose "Where
// settings live" table lists the user tier alone: "Settings are
// authored in `.factory/` folders, using the same schema at every
// level", with the levels table rowing "**Project** | `<git-root>/.factory/`".
// The skills page corroborates it: "the **Project** tab writes to
// `<project>/.factory/settings.json`".
//
// MergeJSONFile, not a whole-document write: this file is shared with
// every other key Droid CLI or a maintainer puts there, `disabledSkills`
// among them. Only `model` is ever set.
func emitSettings(sess *emit.Session, settings []spec.Entry, path string, dryRun bool) error {
	emit.NoteFieldNoOp(target, spec.KindSettings, "permissions", emit.SpecsWithPermissions(settings), permissionsNoKeyReason)
	model := emit.LastSettingsModel(settings)
	if model == "" {
		return nil
	}
	return sess.MergeJSONFile(path, map[string]any{"model": model}, dryRun)
}
