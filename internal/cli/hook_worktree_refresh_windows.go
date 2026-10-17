package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func runOwnedWorktreeSetup(ctx context.Context, cmd *exec.Cmd) error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("create setup job: %w", err)
	}
	defer func() { _ = windows.CloseHandle(job) }()
	if err := worktreeSetupJobLimit(job, windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE); err != nil {
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
	err = assignWorktreeSetupJob(job, uint32(cmd.Process.Pid))
	if err == nil {
		assigned = true
		err = ctx.Err()
	}
	if err == nil {
		err = resumeWorktreeSetup(uint32(cmd.Process.Pid))
	}
	ownership.Unlock()
	if err != nil {
		return errors.Join(err, stop(), cmd.Wait(), awaitWorktreeSetupJob(job))
	}
	if err := cmd.Wait(); err != nil {
		return errors.Join(err, stop(), awaitWorktreeSetupJob(job))
	}
	if err := worktreeSetupJobLimit(job, 0); err != nil {
		return errors.Join(err, stop(), awaitWorktreeSetupJob(job))
	}
	return nil
}

func worktreeSetupJobLimit(job windows.Handle, flags uint32) error {
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = flags
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return fmt.Errorf("set setup job limits: %w", err)
	}
	return nil
}

func assignWorktreeSetupJob(job windows.Handle, pid uint32) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return fmt.Errorf("open suspended setup process: %w", err)
	}
	defer func() { _ = windows.CloseHandle(process) }()
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return fmt.Errorf("assign suspended setup process: %w", err)
	}
	return nil
}

func resumeWorktreeSetup(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("find suspended setup thread: %w", err)
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	var threadID uint32
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		if threadID != 0 {
			return fmt.Errorf("suspended setup process %d has more than one thread", pid)
		}
		threadID = entry.ThreadID
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("enumerate suspended setup threads: %w", err)
	}
	if threadID == 0 {
		return fmt.Errorf("suspended setup process %d has no thread", pid)
	}
	thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, threadID)
	if err != nil {
		return fmt.Errorf("open suspended setup thread: %w", err)
	}
	defer func() { _ = windows.CloseHandle(thread) }()
	count, err := windows.ResumeThread(thread)
	if err != nil {
		return fmt.Errorf("resume setup thread: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("setup thread suspend count is %d, expected 1", count)
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

func awaitWorktreeSetupJob(job windows.Handle) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		var accounting struct {
			UserTime, KernelTime, PeriodUserTime, PeriodKernelTime           int64
			PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
		}
		if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
			return fmt.Errorf("check setup job processes: %w", err)
		}
		if accounting.ActiveProcesses == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("setup job processes did not stop: %w", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}
