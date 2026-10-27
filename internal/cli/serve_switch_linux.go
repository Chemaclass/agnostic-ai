package cli

import "syscall"

// peerSysProcAttr stops a started server when this one dies without
// running its cleanup, such as on SIGKILL.
func peerSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}
