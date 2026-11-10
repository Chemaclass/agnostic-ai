//go:build !darwin && !linux && !windows

package conversations

import "os"

// tryLockFile takes no lock here: these systems, such as the browser
// build, run one process on files nothing else shares, and the
// scanner's own mutex guards that one.
func tryLockFile(*os.File) error { return nil }

func lockHeldElsewhere(error) bool { return false }
