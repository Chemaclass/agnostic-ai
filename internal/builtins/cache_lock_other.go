//go:build !unix && !windows

package builtins

import (
	"errors"
	"os"
)

func lockCache(*os.File) error {
	return errors.New("builtin cache locking is unavailable on this platform")
}
