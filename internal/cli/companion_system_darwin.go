package cli

import (
	"runtime"

	"golang.org/x/sys/unix"
)

func macOSVersion() string {
	version, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return ""
	}
	return version
}

// machineArch is the Mac's own processor. An Intel agnostic-ai under
// Rosetta still installs the Apple silicon companion.
func machineArch() string {
	if translated, err := unix.SysctlUint32("sysctl.proc_translated"); err == nil && translated == 1 {
		return "arm64"
	}
	return runtime.GOARCH
}
