//go:build darwin || linux

package conversations

import (
	"os"
	"os/exec"
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

// detach starts cmd in a session of its own, apart from this process's
// terminal and its signals.
func detach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
