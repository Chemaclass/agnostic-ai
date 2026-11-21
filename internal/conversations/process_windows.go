//go:build windows

package conversations

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var nativeProcesses nativeReader = windowsProcesses{}

// stillActive is the exit code of a process that runs: STILL_ACTIVE.
const stillActive = 259

// windowsProcesses reads processes with the Windows API: a process
// snapshot for the table, each process's own creation time and command
// line, and the Restart Manager for which process holds a file.
type windowsProcesses struct{}

func (windowsProcesses) processes() ([]ProcessInfo, error) {
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("process snapshot: %w", err)
	}
	defer windows.CloseHandle(snapshot)
	var table []ProcessInfo
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		started, runs := processStarted(entry.ProcessID)
		if !runs {
			continue
		}
		table = append(table, ProcessInfo{
			PID:     int(entry.ProcessID),
			PPID:    int(entry.ParentProcessID),
			Started: started,
			Command: windows.UTF16ToString(entry.ExeFile[:]),
		})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("process snapshot: %w", err)
	}
	return table, nil
}

// processStarted is the creation time of pid, empty for a process this
// user may not ask, such as a system service. runs is false for one that
// has exited but whose handle another process still keeps open.
func processStarted(pid uint32) (started string, runs bool) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", true
	}
	defer windows.CloseHandle(process)
	var code uint32
	if windows.GetExitCodeProcess(process, &code) == nil && code != stillActive {
		return "", false
	}
	var created, exited, kernel, user windows.Filetime
	if windows.GetProcessTimes(process, &created, &exited, &kernel, &user) != nil {
		return "", true
	}
	return filetimeText(uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)), true
}

func (windowsProcesses) args(pids []int) map[int]string {
	args := map[int]string{}
	for _, pid := range pids {
		if line, ok := commandLine(uint32(pid)); ok {
			args[pid] = line
		}
	}
	return args
}

func commandLine(pid uint32) (string, bool) {
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(process)
	var size uint32
	_ = windows.NtQueryInformationProcess(process, windows.ProcessCommandLineInformation, nil, 0, &size)
	if size < uint32(unsafe.Sizeof(windows.NTUnicodeString{})) {
		return "", false
	}
	buffer := make([]byte, size)
	if windows.NtQueryInformationProcess(process, windows.ProcessCommandLineInformation, unsafe.Pointer(&buffer[0]), size, &size) != nil {
		return "", false
	}
	return (*windows.NTUnicodeString)(unsafe.Pointer(&buffer[0])).String(), true
}

var (
	restartManager      = windows.NewLazySystemDLL("rstrtmgr.dll")
	rmStartSession      = restartManager.NewProc("RmStartSession")
	rmRegisterResources = restartManager.NewProc("RmRegisterResources")
	rmGetList           = restartManager.NewProc("RmGetList")
	rmEndSession        = restartManager.NewProc("RmEndSession")
)

// rmProcessInfo is RM_PROCESS_INFO.
type rmProcessInfo struct {
	ProcessID   uint32
	StartTime   windows.Filetime
	AppName     [256]uint16
	ServiceName [64]uint16
	AppType     uint32
	AppStatus   uint32
	TSSessionID uint32
	Restartable int32
}

func (windowsProcesses) holders(paths []string) (map[string]int, error) {
	return holdersByHalves(paths, restartManagerHolders)
}

// restartManagerHolders asks the Restart Manager which processes have
// any of paths open. A file no process holds names none.
func restartManagerHolders(paths []string) ([]int, error) {
	var session uint32
	key := make([]uint16, 33)
	if code, _, _ := rmStartSession.Call(uintptr(unsafe.Pointer(&session)), 0, uintptr(unsafe.Pointer(&key[0]))); code != 0 {
		return nil, fmt.Errorf("RmStartSession: %w", windows.Errno(code))
	}
	defer rmEndSession.Call(uintptr(session))
	names := make([]*uint16, 0, len(paths))
	for _, path := range paths {
		name, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return nil, err
		}
		names = append(names, name)
	}
	if code, _, _ := rmRegisterResources.Call(uintptr(session), uintptr(len(names)), uintptr(unsafe.Pointer(&names[0])), 0, 0, 0, 0); code != 0 {
		return nil, fmt.Errorf("RmRegisterResources: %w", windows.Errno(code))
	}
	infos := make([]rmProcessInfo, 8)
	for {
		var needed, count uint32 = 0, uint32(len(infos))
		var reasons uint32
		code, _, _ := rmGetList.Call(uintptr(session), uintptr(unsafe.Pointer(&needed)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&infos[0])), uintptr(unsafe.Pointer(&reasons)))
		switch windows.Errno(code) {
		case 0:
			pids := make([]int, 0, count)
			for _, info := range infos[:count] {
				pids = append(pids, int(info.ProcessID))
			}
			return pids, nil
		case windows.ERROR_MORE_DATA:
			infos = make([]rmProcessInfo, needed+4)
		default:
			return nil, fmt.Errorf("RmGetList: %w", windows.Errno(code))
		}
	}
}
