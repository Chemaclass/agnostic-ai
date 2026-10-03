//go:build !unix

package hookrun

import (
	"context"
	"os/exec"
)

// crushGroups tracks nothing off Unix, which has no process groups to
// end; Crush isolates nothing there either (shell/exec_windows.go:15-18).
type crushGroups struct{}

func (*crushGroups) reap() {}

// crushStart runs cmd. The default exec handler, which Crush keeps on
// Windows (shell/exec_windows.go:20-23), never reaches it.
func crushStart(_ context.Context, cmd *exec.Cmd, _ *crushGroups, _ bool) error {
	return cmd.Run()
}
