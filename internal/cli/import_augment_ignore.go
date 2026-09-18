package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// importFromAugment restores the Augment surfaces sync can own safely:
// the workspace indexing exclusions and the committed tool permission
// policy. The rest of Augment's configuration has no native importer.
func importFromAugment(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Settings); err != nil {
		return err
	}
	ignores, err := importIgnoreFile(root, "augment", src)
	if err != nil {
		return err
	}
	settings, err := importAugmentPermissions(root, filepath.Join(root, src.Settings))
	if err != nil {
		return err
	}
	summaryf("imported %d ignores, %d settings (other Augment configuration is not imported)\n", ignores, settings)
	printImportNextSteps(root, "augment")
	return nil
}
