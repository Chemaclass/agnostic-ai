package claude

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// ProtectedRulesFile records the permission rules sync added to
// settings.json for protected paths. settings.json merges into what is
// on disk, so without it a rule would outlive the block that wrote it.
const ProtectedRulesFile = ".agnostic-ai-protected.json"

func readProtectedRules(dir string, settings []spec.Entry) (emit.OwnedRules, error) {
	rules := map[string][]string{}
	for list, raw := range protectedPermissions(settings) {
		rules[list] = emit.StringSlice(raw)
	}
	return emit.ReadOwnedRules(filepath.Join(dir, ProtectedRulesFile), rules)
}
