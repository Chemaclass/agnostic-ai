//go:build windows

package cli

import "golang.org/x/sys/windows"

// stillActive is the exit code Windows reports for a process that runs.
const stillActive = 259

func companionProcessRuns(pid int) bool {
	if pid <= 0 {
		return false
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(handle) }()
	var code uint32
	return windows.GetExitCodeProcess(handle, &code) == nil && code == stillActive
}
