//go:build !darwin && !linux && !windows

package gitexclude

import (
	"fmt"
	"os"
	"runtime"
)

func tryLock(_ *os.File) error {
	return fmt.Errorf("file locking is unsupported on %s", runtime.GOOS)
}

func contended(_ error) bool {
	return false
}
