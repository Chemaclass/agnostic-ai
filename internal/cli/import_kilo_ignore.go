package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

func importKiloIgnore(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Settings); err != nil {
		return err
	}
	ignores, err := importIgnoreFile(root, "kilo", src)
	if err != nil {
		return err
	}
	settings, err := importPortableSettings(root, "kilo.jsonc", filepath.Join(root, src.Settings), false, false)
	if err != nil {
		return err
	}
	summaryf("imported %d ignores, %d settings (other Kilo configuration is not imported)\n", ignores, settings)
	printImportNextSteps(root, "kilo")
	return nil
}
