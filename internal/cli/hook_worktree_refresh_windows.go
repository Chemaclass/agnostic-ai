package cli

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

func runOwnedWorktreeSetup(ctx context.Context, cmd *exec.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create setup job: %w", err)
	}
	defer func() { _ = windows.CloseHandle(job) }()
	if err := ownedprocess.SetJobLimit(job, windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE); err != nil {
		return err
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_SUSPENDED}
	var ownership sync.Mutex
	assigned := false
	stop := func() error {
		ownership.Lock()
		defer ownership.Unlock()
		return terminateWorktreeSetupJob(job, cmd, assigned)
	}
	cmd.Cancel = stop
	ownership.Lock()
	if err := cmd.Start(); err != nil {
		ownership.Unlock()
		return err
	}
	err = ownedprocess.AssignJob(job, uint32(cmd.Process.Pid))
	if err == nil {
		assigned = true
		err = ctx.Err()
	}
	if err == nil {
		err = ownedprocess.ResumeProcess(uint32(cmd.Process.Pid))
	}
	ownership.Unlock()
	if err != nil {
		return errors.Join(err, stop(), cmd.Wait(), ownedprocess.AwaitJob(job))
	}
	if err := cmd.Wait(); err != nil {
		return errors.Join(err, stop(), ownedprocess.AwaitJob(job))
	}
	if err := ownedprocess.SetJobLimit(job, 0); err != nil {
		return errors.Join(err, stop(), ownedprocess.AwaitJob(job))
	}
	return nil
}

func terminateWorktreeSetupJob(job windows.Handle, cmd *exec.Cmd, assigned bool) error {
	jobErr := windows.TerminateJobObject(job, 1)
	if assigned && jobErr == nil {
		return nil
	}
	processErr := cmd.Process.Kill()
	if errors.Is(processErr, os.ErrProcessDone) {
		processErr = nil
	}
	return errors.Join(jobErr, processErr)
}
