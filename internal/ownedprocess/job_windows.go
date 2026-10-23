package ownedprocess

import (
	"context"
	"errors"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

func SetJobLimit(job windows.Handle, flags uint32) error {
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = flags
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		return fmt.Errorf("set owned job limits: %w", err)
	}
	return nil
}

func AssignJob(job windows.Handle, pid uint32) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, pid)
	if err != nil {
		return fmt.Errorf("open suspended owned process: %w", err)
	}
	defer func() { _ = windows.CloseHandle(process) }()
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return fmt.Errorf("assign suspended owned process: %w", err)
	}
	return nil
}

func ResumeProcess(pid uint32) error {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("find suspended owned thread: %w", err)
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()
	entry := windows.ThreadEntry32{Size: uint32(unsafe.Sizeof(windows.ThreadEntry32{}))}
	var threadID uint32
	for err = windows.Thread32First(snapshot, &entry); err == nil; err = windows.Thread32Next(snapshot, &entry) {
		if entry.OwnerProcessID != pid {
			continue
		}
		if threadID != 0 {
			return fmt.Errorf("suspended owned process %d has more than one thread", pid)
		}
		threadID = entry.ThreadID
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return fmt.Errorf("enumerate suspended owned threads: %w", err)
	}
	if threadID == 0 {
		return fmt.Errorf("suspended owned process %d has no thread", pid)
	}
	thread, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, threadID)
	if err != nil {
		return fmt.Errorf("open suspended owned thread: %w", err)
	}
	defer func() { _ = windows.CloseHandle(thread) }()
	count, err := windows.ResumeThread(thread)
	if err != nil {
		return fmt.Errorf("resume owned thread: %w", err)
	}
	if count != 1 {
		return fmt.Errorf("owned thread suspend count is %d, expected 1", count)
	}
	return nil
}

func AwaitJob(job windows.Handle) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		var accounting struct {
			UserTime, KernelTime, PeriodUserTime, PeriodKernelTime           int64
			PageFaults, TotalProcesses, ActiveProcesses, TerminatedProcesses uint32
		}
		if err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&accounting)), uint32(unsafe.Sizeof(accounting)), nil); err != nil {
			return fmt.Errorf("check owned job processes: %w", err)
		}
		if accounting.ActiveProcesses == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("owned job processes did not stop: %w", ctx.Err())
		case <-time.After(10 * time.Millisecond):
		}
	}
}
