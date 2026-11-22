//go:build !windows

package mailbox

import "os"

func openShared(path string) (*os.File, error) {
	return os.Open(path)
}
