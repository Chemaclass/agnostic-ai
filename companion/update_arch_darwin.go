package main

import (
	"runtime"

	"golang.org/x/sys/unix"
)

func nativeUpdateArch() string {
	if translated, err := unix.SysctlUint32("sysctl.proc_translated"); err == nil && translated == 1 {
		return "arm64"
	}
	return runtime.GOARCH
}
