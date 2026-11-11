package conversations

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Processes answers questions about running processes.
type Processes interface {
	// Alive reports whether pid runs and, when procStart is set, is the
	// process that started then rather than a later one with a reused pid.
	Alive(pid int, procStart string) bool
	// AliveAmong answers Alive for many processes at once and returns
	// the pids that passed. A scan asks about every open session, so
	// the system reader does it with one look at the process table.
	AliveAmong(processes []ProcessRef) map[int]bool
	// Table lists every process, with its parent, terminal and start.
	Table() ([]ProcessInfo, error)
	// Args maps each of pids that runs to its command line.
	Args(pids []int) map[int]string
	// Cwds maps each of pids that runs to its working folder.
	Cwds(pids []int) map[int]string
	// Holders maps each of paths a running process has open to that
	// process's pid.
	Holders(paths []string) map[string]int
	// AppRunning reports whether the app, such as "Cursor", runs.
	AppRunning(name string) bool
}

// ProcessRef names a process by its pid and, when known, by when it
// started, as ps lstart writes it in UTC and the C locale.
type ProcessRef struct {
	PID       int    `json:"pid"`
	ProcStart string `json:"procStart"`
}

// SystemProcesses reads the process table with ps.
type SystemProcesses struct{}

const psTimeout = 3 * time.Second

func ps(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ps", args...)
	// procStart in Claude Code's session files is ps lstart in UTC and
	// the C locale.
	cmd.Env = append(os.Environ(), "TZ=UTC", "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ps %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (SystemProcesses) Alive(pid int, procStart string) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false
	}
	if procStart == "" {
		return true
	}
	started, err := ps("-o", "lstart=", "-p", strconv.Itoa(pid))
	return err == nil && started == strings.TrimSpace(procStart)
}

// AliveAmong runs ps once for every pid. ps prints a line for each pid
// that runs and fails when none does, which leaves the result empty.
func (SystemProcesses) AliveAmong(processes []ProcessRef) map[int]bool {
	alive := map[int]bool{}
	var pids []string
	for _, process := range processes {
		if process.PID > 0 {
			pids = append(pids, strconv.Itoa(process.PID))
		}
	}
	if len(pids) == 0 {
		return alive
	}
	out, err := ps("-o", "pid=,lstart=", "-p", strings.Join(pids, ","))
	if err != nil {
		return alive
	}
	started := psStartTimes(out)
	for _, process := range processes {
		start, runs := started[process.PID]
		if runs && (process.ProcStart == "" || start == strings.TrimSpace(process.ProcStart)) {
			alive[process.PID] = true
		}
	}
	return alive
}

// psStartTimes reads `ps -o pid=,lstart=` output: a pid, then its start
// time, on each line.
func psStartTimes(out string) map[int]string {
	started := map[int]string{}
	for _, line := range strings.Split(out, "\n") {
		pidText, start, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		pid, err := strconv.Atoi(pidText)
		if err != nil {
			continue
		}
		started[pid] = strings.TrimSpace(start)
	}
	return started
}

func (SystemProcesses) Table() ([]ProcessInfo, error) {
	out, err := ps("-axo", "pid=,ppid=,stat=,tty=,lstart=,comm=")
	if err != nil {
		return nil, err
	}
	return parseProcessTable(out), nil
}

func pidList(pids []int) string {
	texts := make([]string, len(pids))
	for i, pid := range pids {
		texts[i] = strconv.Itoa(pid)
	}
	return strings.Join(texts, ",")
}

func (SystemProcesses) Args(pids []int) map[int]string {
	args := map[int]string{}
	if len(pids) == 0 {
		return args
	}
	out, err := ps("-o", "pid=,args=", "-p", pidList(pids))
	if err != nil {
		return args
	}
	for _, line := range strings.Split(out, "\n") {
		pidText, command, ok := strings.Cut(strings.TrimSpace(line), " ")
		if pid, err := strconv.Atoi(pidText); ok && err == nil {
			args[pid] = strings.TrimSpace(command)
		}
	}
	return args
}

// Cwds asks lsof once for every pid: a "p<pid>" line, then "n<folder>".
func (SystemProcesses) Cwds(pids []int) map[int]string {
	cwds := map[int]string{}
	if len(pids) == 0 {
		return cwds
	}
	ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
	defer cancel()
	out, _ := exec.CommandContext(ctx, "lsof", "-a", "-d", "cwd", "-Fn", "-p", pidList(pids)).Output()
	pid := 0
	for _, line := range strings.Split(string(out), "\n") {
		if field, ok := strings.CutPrefix(line, "p"); ok {
			pid, _ = strconv.Atoi(field)
		} else if folder, ok := strings.CutPrefix(line, "n"); ok && pid > 0 {
			cwds[pid] = folder
		}
	}
	return cwds
}

func (SystemProcesses) Holders(paths []string) map[string]int {
	held := map[string]int{}
	if len(paths) == 0 {
		return held
	}
	// lsof names each open file by its resolved path, so a path under a
	// symlinked folder, such as a linked ~/.codex, is looked up by its target.
	asked := make(map[string]string, len(paths))
	for _, path := range paths {
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			asked[resolved] = path
		} else {
			asked[path] = path
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
	defer cancel()
	// lsof exits 1 when none of the files is open; its output still lists
	// the ones that are: a "p<pid>" line, then one "n<path>" line for
	// each file that process has open.
	out, _ := exec.CommandContext(ctx, "lsof", append([]string{"-F", "pn", "--"}, paths...)...).Output()
	pid := 0
	for _, line := range strings.Split(string(out), "\n") {
		if field, ok := strings.CutPrefix(line, "p"); ok {
			pid, _ = strconv.Atoi(field)
		} else if open, ok := strings.CutPrefix(line, "n"); ok {
			if path, ok := asked[open]; ok && pid > 0 {
				held[path] = pid
			}
		}
	}
	return held
}

// AppRunning looks for the app's main executable in the process list:
// on macOS its command is the full path inside the app bundle, such as
// /Applications/Cursor.app/Contents/MacOS/Cursor; elsewhere, its name.
func (SystemProcesses) AppRunning(name string) bool {
	commands, err := ps("-axo", "comm=")
	if err != nil {
		return false
	}
	bundled := "/" + name + ".app/Contents/MacOS/" + name
	for _, command := range strings.Split(commands, "\n") {
		command = strings.TrimSpace(command)
		if strings.HasSuffix(command, bundled) || strings.EqualFold(filepath.Base(command), name) {
			return true
		}
	}
	return false
}
