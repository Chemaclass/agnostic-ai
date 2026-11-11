//go:build darwin || linux

package conversations

import (
	"os"
	"syscall"
)

// ownedByMe reports whether the current user owns the file.
func ownedByMe(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}

// ExecTool replaces this process with path, so the tool owns the tab.
func ExecTool(path string, argv, env []string) error {
	return syscall.Exec(path, argv, env)
}
