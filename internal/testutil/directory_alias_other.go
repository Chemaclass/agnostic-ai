//go:build !windows

package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func DirectoryAlias(t *testing.T, target, alias string) {
	t.Helper()
	abs, err := filepath.Abs(target)
	if err != nil {
		t.Fatalf("resolve directory alias target %s: %v", target, err)
	}
	if err := os.Symlink(abs, alias); err != nil {
		t.Fatalf("create directory alias %s: %v", alias, err)
	}
}
