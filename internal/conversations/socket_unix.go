//go:build darwin || linux

package conversations

import (
	"os"
	"syscall"
)

// ownSocket is true for a socket at path that this user owns, since any
// user can make one in /tmp.
func ownSocket(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSocket == 0 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}
