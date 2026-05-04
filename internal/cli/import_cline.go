package cli

// importFromCline scaffolds an agnostic-ai project by reading existing
// Cline config (.clinerules/) under root.
func importFromCline(root string) error {
	return importFromRulesDir(root, "cline", ".clinerules")
}
