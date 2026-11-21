package conversations

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// startInNewConsole starts a program in a console window of its own
// through cmd's start. A console the server makes itself would get the
// null device as input and output, so PowerShell would end at once.
// Both programs are found before the start, so cmd never looks for one
// in the server's folder, and a missing one fails here.
func startInNewConsole(name string, args ...string) error {
	program, err := exec.LookPath(name)
	if err != nil {
		return err
	}
	shell := filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
	cmd := exec.Command(shell, append([]string{"/c", "start", "", program}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
