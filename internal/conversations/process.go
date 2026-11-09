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
	// TTY is the terminal device pid writes to, such as /dev/ttys018.
	TTY(pid int) (string, error)
	// TerminalApp names the app whose window holds pid's terminal, such
	// as "Terminal" or "Cursor", or "" when no known app is an ancestor.
	TerminalApp(pid int) (string, error)
	// Holders maps each of paths a running process has open to that
	// process's pid.
	Holders(paths []string) map[string]int
	// AppRunning reports whether the app, such as "Cursor", runs.
	AppRunning(name string) bool
}

// ProcessRef names a process by its pid and, when known, by when it
// started, as ps lstart writes it in UTC and the C locale.
type ProcessRef struct {
	PID       int
	ProcStart string
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

func (SystemProcesses) TTY(pid int) (string, error) {
	tty, err := ps("-o", "tty=", "-p", strconv.Itoa(pid))
	if err != nil {
		return "", err
	}
	if tty == "" || tty == "??" {
		return "", fmt.Errorf("process %d has no terminal", pid)
	}
	return "/dev/" + strings.TrimPrefix(tty, "/dev/"), nil
}

// terminalApps maps a fragment of an ancestor's command path to the app
// that shows the terminal.
var terminalApps = []struct{ fragment, app string }{
	{"/Terminal.app/", "Terminal"},
	{"/iTerm.app/", "iTerm"},
	{"/Ghostty.app/", "Ghostty"},
	{"/Warp.app/", "Warp"},
	{"/Cursor.app/", "Cursor"},
	{"/Visual Studio Code.app/", "VS Code"},
	{"/Zed.app/", "Zed"},
}

const maxAncestors = 12

func (SystemProcesses) TerminalApp(pid int) (string, error) {
	current := pid
	for range maxAncestors {
		line, err := ps("-o", "ppid=,comm=", "-p", strconv.Itoa(current))
		if err != nil {
			return "", err
		}
		parentField, command, _ := strings.Cut(strings.TrimSpace(line), " ")
		for _, known := range terminalApps {
			if strings.Contains(command, known.fragment) {
				return known.app, nil
			}
		}
		parent, err := strconv.Atoi(strings.TrimSpace(parentField))
		if err != nil || parent <= 1 {
			return "", nil
		}
		current = parent
	}
	return "", nil
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
