//go:build unix

package cli

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func runOwnedWorktreeSetup(_ context.Context, cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stop := func() error {
		_ = cmd.Process.Kill()
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("terminate setup process group: %w", err)
		}
		return nil
	}
	cmd.Cancel = stop
	if err := cmd.Start(); err != nil {
		return err
	}
	err := cmd.Wait()
	if err == nil {
		return nil
	}
	return errors.Join(err, stop(), awaitWorktreeSetupGroup(cmd.Process.Pid))
}

func awaitWorktreeSetupGroup(group int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		out, err := exec.CommandContext(ctx, "ps", "-axo", "pgid=,stat=").Output()
		if err != nil {
			return fmt.Errorf("check setup process group %d: %w", group, err)
		}
		alive := false
		for _, line := range strings.Split(string(out), "\n") {
			fields := strings.Fields(line)
			if len(fields) != 2 {
				continue
			}
			pid, err := strconv.Atoi(fields[0])
			if err == nil && pid == group && !strings.HasPrefix(fields[1], "Z") {
				alive = true
				break
			}
		}
		if !alive {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("setup process group %d did not stop: %w", group, ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}
