package cli

import "github.com/chemaclass/agnostic-ai/internal/config"

// importAugmentIgnore restores the workspace indexing exclusions that sync
// can own safely. Other Augment surfaces do not yet have native importers.
func importAugmentIgnore(root string, src config.Sources) error {
	ignores, err := importIgnoreFile(root, "augment", src)
	if err != nil {
		return err
	}
	summaryf("imported %d ignores (other Augment configuration is not imported)\n", ignores)
	printImportNextSteps(root, "augment")
	return nil
}
