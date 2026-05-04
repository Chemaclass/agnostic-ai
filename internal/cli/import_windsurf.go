package cli

import "path/filepath"

// importFromWindsurf scaffolds an agnostic-ai project by reading
// existing Windsurf config (.windsurf/rules/) under root.
func importFromWindsurf(root string) error {
	return importFromRulesDir(root, "windsurf", filepath.Join(".windsurf", "rules"))
}
