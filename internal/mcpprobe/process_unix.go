//go:build unix

package mcpprobe

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sync"
	"syscall"
)

func headersCommand(ctx context.Context, helper string) *exec.Cmd {
	return exec.CommandContext(ctx, "sh", "-c", helper)
}

func startProcess(_ context.Context, cmd *exec.Cmd) (func() error, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var once sync.Once
	var stopErr error
	stop := func() error {
		once.Do(func() {
			err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
			_ = cmd.Process.Kill()
			if err != nil && !errors.Is(err, syscall.ESRCH) {
				stopErr = fmt.Errorf("terminate MCP process group: %w", err)
			}
		})
		return stopErr
	}
	cmd.Cancel = stop
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return stop, nil
}
