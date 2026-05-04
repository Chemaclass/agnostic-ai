package cli

import "path/filepath"

// importFromContinue scaffolds an agnostic-ai project by reading
// existing Continue config (.continue/rules/) under root.
func importFromContinue(root string) error {
	return importFromRulesDir(root, "continue", filepath.Join(".continue", "rules"))
}
