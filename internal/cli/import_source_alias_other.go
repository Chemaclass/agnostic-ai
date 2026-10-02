//go:build !windows

package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

func createImportSourceAlias(target, alias string) error {
	rel, err := filepath.Rel(resolveImportSourceExisting(filepath.Dir(alias)), target)
	if err != nil {
		return fmt.Errorf("resolve source preview alias %s: %w", alias, err)
	}
	if err := os.Symlink(rel, alias); err != nil {
		return fmt.Errorf("%s: %w", alias, err)
	}
	return nil
}
