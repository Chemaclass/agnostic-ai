//go:build !unix

package hookrun

import (
	"context"
	"os/exec"
)

// crushIsolate does nothing off Unix, as in Crush
// (shell/exec_windows.go:15-18).
func crushIsolate(*exec.Cmd) {}

// crushStart is never reached off Unix, where Crush keeps mvdan's
// default exec handler (shell/exec_windows.go:20-23).
func crushStart(_ context.Context, cmd *exec.Cmd) error {
	return cmd.Run()
}
