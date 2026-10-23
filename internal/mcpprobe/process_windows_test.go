package mcpprobe

import (
	"errors"
	"testing"

	"golang.org/x/sys/windows"
)

func processRunning(t *testing.T, pid int) bool {
	t.Helper()
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return false
	}
	if err != nil {
		t.Fatalf("open owned descendant: %v", err)
	}
	defer func() { _ = windows.CloseHandle(process) }()
	state, err := windows.WaitForSingleObject(process, 0)
	if err != nil {
		t.Fatalf("check owned descendant: %v", err)
	}
	return state == uint32(windows.WAIT_TIMEOUT)
}
