package claude

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
)

// OwnedPermissionsFile records the permission rules sync added to
// settings.json: portable, protected, config, and x-claude rules.
// settings.json merges into the file on disk when no overlay was
// captured, so without the record a removed rule would stay.
const OwnedPermissionsFile = ".agnostic-ai-permissions.json"

// LegacyProtectedRulesFile is the earlier record, which held only the
// rules generated from protected paths.
const LegacyProtectedRulesFile = ".agnostic-ai-protected.json"

func readOwnedPermissions(dir string) (emit.OwnedRules, error) {
	return emit.ReadOwnedRules(filepath.Join(dir, OwnedPermissionsFile), filepath.Join(dir, LegacyProtectedRulesFile))
}
