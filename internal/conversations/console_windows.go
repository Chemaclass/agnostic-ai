package conversations

import (
	"os/exec"
	"syscall"
)

// startInNewConsole starts a program in a console window of its own
// through cmd's start. A console the server makes itself would get the
// null device as input and output, so PowerShell would end at once.
func startInNewConsole(name string, args ...string) error {
	cmd := exec.Command("cmd.exe", append([]string{"/c", "start", "", name}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
