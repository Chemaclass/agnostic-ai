package conversations

import (
	"os/exec"
	"syscall"
)

// createNewConsole gives a program a console window of its own; the
// server that starts it has none.
const createNewConsole = 0x00000010

func startInNewConsole(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	if err := cmd.Start(); err != nil {
		return err
	}
	go func() { _ = cmd.Wait() }()
	return nil
}
