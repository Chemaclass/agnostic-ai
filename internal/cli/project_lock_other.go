//go:build !darwin && !linux && !windows

package cli

import (
	"fmt"
	"os"
	"runtime"
)

func tryLockProjectFile(_ *os.File) error {
	return fmt.Errorf("project locking is unsupported on %s", runtime.GOOS)
}

func projectLockContended(_ error) bool {
	return false
}
