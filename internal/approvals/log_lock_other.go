//go:build !darwin && !linux && !windows

package approvals

import "os"

// These systems, such as the browser build, run no permission hook, so
// nothing else writes the log at the same time.
func tryLockFile(*os.File) error { return nil }

func lockHeldElsewhere(error) bool { return false }
