//go:build unix

package cli

import (
	"os"
	"syscall"

	"golang.org/x/sys/unix"
)

const mailboxDeliveryReadFlags = os.O_RDONLY | unix.O_NONBLOCK

func mailboxDeliveryOwned(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Geteuid()
}
