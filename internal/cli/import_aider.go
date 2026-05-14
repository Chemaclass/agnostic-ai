package cli

import (
	"path/filepath"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

const aiderMainFile = "CONVENTIONS.md"

// importFromAider reads an existing Aider project (CONVENTIONS.md plus
// optional .aider.conf.yml) under root and writes specs into the
// configured source directories.
func importFromAider(root string, src config.Sources) error {
	if err := mkdirAllSources(root, src.Rules); err != nil {
		return err
	}
	n, err := sliceMainFileByH2(root, aiderMainFile, filepath.Join(root, src.Rules))
	if err != nil {
		return err
	}
	if err := mirrorMainFile(root, aiderMainFile); err != nil {
		return err
	}
	summaryf("imported %d rules\n", n)
	return nil
}
