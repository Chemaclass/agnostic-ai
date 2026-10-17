//go:build !unix && !windows

package cli

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
)

func runOwnedWorktreeSetup(context.Context, *exec.Cmd) error {
	return fmt.Errorf("owned Claude worktree setup is unsupported on %s", runtime.GOOS)
}
