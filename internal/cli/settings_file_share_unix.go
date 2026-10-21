//go:build unix

package cli

import (
	"os"
	"syscall"
)

// settingsFileShared says why replacing the file by rename would change
// more than its text: another name would keep the old copy, or the new
// file would belong to the current user instead of the owner.
func settingsFileShared(info os.FileInfo) string {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return ""
	}
	if st.Nlink > 1 {
		return "has another hard link"
	}
	if int(st.Uid) != os.Geteuid() {
		return "belongs to another user"
	}
	return ""
}
