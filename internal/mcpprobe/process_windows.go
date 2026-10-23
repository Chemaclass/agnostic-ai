package mcpprobe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/chemaclass/agnostic-ai/internal/ownedprocess"
)

func headersCommand(ctx context.Context, helper string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(cmd.Path) + ` /d /s /c "` + helper + `"`}
	return cmd
}

func startProcess(ctx context.Context, cmd *exec.Cmd) (func() error, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("create MCP process job: %w", err)
	}
	if err := ownedprocess.SetJobLimit(job, windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE); err != nil {
		_ = windows.CloseHandle(job)
		return nil, err
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	var mu sync.Mutex
	assigned, closed := false, false
	kill := func() error {
		if closed {
			return os.ErrProcessDone
		}
		jobErr := windows.TerminateJobObject(job, 1)
		if assigned && jobErr == nil {
			return nil
		}
		if cmd.Process == nil {
			return jobErr
		}
		processErr := cmd.Process.Kill()
		if errors.Is(processErr, os.ErrProcessDone) {
			processErr = nil
		}
		return errors.Join(jobErr, processErr)
	}
	cmd.Cancel = func() error { mu.Lock(); defer mu.Unlock(); return kill() }
	cleanup := func() error {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return nil
		}
		err := kill()
		err = errors.Join(err, ownedprocess.AwaitJob(job))
		err = errors.Join(err, windows.CloseHandle(job))
		closed = true
		return err
	}
	mu.Lock()
	if err := cmd.Start(); err != nil {
		mu.Unlock()
		_ = cleanup()
		return nil, err
	}
	err = ownedprocess.AssignJob(job, uint32(cmd.Process.Pid))
	if err == nil {
		assigned = true
		err = ctx.Err()
	}
	if err == nil {
		err = ownedprocess.ResumeProcess(uint32(cmd.Process.Pid))
	}
	mu.Unlock()
	if err != nil {
		return nil, errors.Join(err, cleanup(), cmd.Wait())
	}
	return cleanup, nil
}
