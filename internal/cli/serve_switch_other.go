//go:build !linux

package cli

import "syscall"

func peerSysProcAttr() *syscall.SysProcAttr {
	return nil
}
