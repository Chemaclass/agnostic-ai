package conversations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
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
	// the system reader does it with one look at the process table. The
	// error says the table could not be read, which differs from none running.
	AliveAmong(processes []ProcessRef) (map[int]bool, error)
	// Table lists every process, with its parent, terminal and start.
	Table() ([]ProcessInfo, error)
	// Args maps each of pids that runs to its command line.
	Args(pids []int) map[int]string
	// Cwds maps each of pids that runs to its working folder.
	Cwds(pids []int) map[int]string
	// Holders maps each of paths a running process has open to that
	// process's pid. The error says the open files could not be read.
	Holders(paths []string) (map[string]int, error)
	// HoldersUnder maps each file under dir, at any depth, that a running
	// process has open to that process's pid, named under dir as given.
	// The error says the open files could not be read.
	HoldersUnder(dir string) (map[string]int, error)
	// AppRunning reports whether the app, such as "Cursor", runs.
	AppRunning(name string) bool
	// ClaudeJobs lists the Claude Code background sessions that run.
	ClaudeJobs(ctx context.Context) ([]ClaudeJob, error)
	// ClaudeParents maps each of pids that a Claude Code session started,
	// as a command of its Bash tool, to that session's id. The error is an
	// *environmentError: the environments could not be read.
	ClaudeParents(pids []int) (map[int]string, error)
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
	// The output stays with an error: procps ps exits 1 when one of
	// several pids does not run, and still lists those that do.
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("ps %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func (s SystemProcesses) Alive(pid int, procStart string) bool {
	if pid <= 0 {
		return false
	}
	if nativeProcesses != nil {
		alive, _ := s.AliveAmong([]ProcessRef{{PID: pid, ProcStart: procStart}})
		return alive[pid]
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
// that runs, and exits 1 when some or all of them do not.
func (SystemProcesses) AliveAmong(processes []ProcessRef) (map[int]bool, error) {
	alive := map[int]bool{}
	if nativeProcesses != nil {
		table, err := nativeProcesses.processes()
		if err != nil {
			return alive, err
		}
		started := map[int]ProcessInfo{}
		for _, process := range table {
			started[process.PID] = process
		}
		for _, process := range processes {
			listed, runs := started[process.PID]
			if runs && process.PID > 0 && (process.ProcStart == "" || hidesClaudeStart(listed) || listed.Started == strings.TrimSpace(process.ProcStart)) {
				alive[process.PID] = true
			}
		}
		return alive, nil
	}
	// Asked about no pid, it still checks ps exists, so a system without it does not pass for one where nothing runs.
	if err := canRead("ps"); err != nil {
		return alive, err
	}
	var pids []string
	for _, process := range processes {
		if process.PID > 0 {
			pids = append(pids, strconv.Itoa(process.PID))
		}
	}
	if len(pids) == 0 {
		return alive, nil
	}
	out, err := ps("-o", "pid=,lstart=", "-p", strings.Join(pids, ","))
	if !answered(err) {
		return alive, err
	}
	started := psStartTimes(out)
	for _, process := range processes {
		start, runs := started[process.PID]
		if runs && (process.ProcStart == "" || start == strings.TrimSpace(process.ProcStart)) {
			alive[process.PID] = true
		}
	}
	return alive, nil
}

// canRead says whether the command can answer which processes run.
func canRead(command string) error {
	_, err := exec.LookPath(command)
	return err
}

// answered reports whether ps or lsof answered: it exited 0, or exited 1
// without a word on stderr, which both print for "none of these runs" or
// "none of these is open". Any other exit, or exit 1 with a message,
// such as an option this ps does not know, is a failed read.
func answered(err error) bool {
	if err == nil {
		return true
	}
	var exit *exec.ExitError
	return errors.As(err, &exit) && exit.ExitCode() == 1 && len(strings.TrimSpace(string(exit.Stderr))) == 0
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
	if nativeProcesses != nil {
		return nativeProcesses.processes()
	}
	out, err := ps("-axo", "pid=,ppid=,pgid=,stat=,tty=,lstart=,comm=")
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
	if nativeProcesses != nil {
		return nativeProcesses.args(pids)
	}
	out, err := ps("-ww", "-o", "pid=,args=", "-p", pidList(pids))
	if !answered(err) {
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

// Argv preserves argument boundaries where the system exposes them.
func (SystemProcesses) Argv(pids []int) map[int][]string {
	args := make(map[int][]string, len(pids))
	for _, pid := range pids {
		if argv := processArgv(pid); len(argv) > 0 {
			args[pid] = argv
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

func (SystemProcesses) Holders(paths []string) (map[string]int, error) {
	if nativeProcesses != nil {
		return nativeProcesses.holders(paths)
	}
	if err := canRead("lsof"); err != nil {
		return map[string]int{}, err
	}
	if len(paths) == 0 {
		return map[string]int{}, nil
	}
	held := map[string]int{}
	var readErr error
	for len(paths) > 0 {
		count, bytes := 0, 0
		// Keep lsof arguments below OS limits even with many old sessions.
		for count < len(paths) && count < 128 && (count == 0 || bytes+len(paths[count])+1 <= 32<<10) {
			bytes += len(paths[count]) + 1
			count++
		}
		open, err := readHolders(paths[:count])
		maps.Copy(held, open)
		readErr = errors.Join(readErr, err)
		paths = paths[count:]
	}
	return held, readErr
}

func readHolders(paths []string) (map[string]int, error) {
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
	// the ones that are.
	// -w keeps warnings, such as a file system lsof cannot stat, off stderr.
	out, err := exec.CommandContext(ctx, "lsof", append([]string{"-w", "-F", "pcn", "--"}, paths...)...).Output()
	if !answered(err) {
		return parseHolders(string(out), asked), fmt.Errorf("lsof: %w", err)
	}
	return parseHolders(string(out), asked), nil
}

// HoldersUnder asks lsof once for the open files under dir, so its cost
// follows the running processes, not how many files the folder keeps.
func (SystemProcesses) HoldersUnder(dir string) (map[string]int, error) {
	if nativeProcesses != nil {
		return nativeHoldersUnder(dir)
	}
	if err := canRead("lsof"); err != nil {
		return map[string]int{}, err
	}
	// lsof names each open file by its resolved path, so a linked
	// ~/.codex is searched at its target and named back under dir.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return map[string]int{}, err
	}
	named := func(open string) (string, bool) {
		rel, err := filepath.Rel(resolved, open)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", false
		}
		return filepath.Join(dir, rel), true
	}
	ctx, cancel := context.WithTimeout(context.Background(), psTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "lsof", "-w", "-F", "pcn", "+D", resolved).Output()
	if !answered(err) {
		return parseHoldersNamed(string(out), named), fmt.Errorf("lsof: %w", err)
	}
	return parseHoldersNamed(string(out), named), nil
}

// parseHolders reads `lsof -F pcn` output: a "p<pid>" line and a
// "c<command>" line for each process, then one "n<path>" line for each
// file it has open. A sqlite3 reader, such as the scan's own query on a
// cursor-agent chat, holds the file only for a moment and is not the
// process that runs the chat. When several processes hold a file, an
// agent's own runtime wins over a tool that only has it open.
func parseHolders(out string, asked map[string]string) map[string]int {
	return parseHoldersNamed(out, func(open string) (string, bool) {
		path, ok := asked[open]
		return path, ok
	})
}

// parseHoldersNamed is parseHolders with name giving the path each open
// file is reported as, or false for a file nobody asked about.
func parseHoldersNamed(out string, name func(open string) (string, bool)) map[string]int {
	held := map[string]int{}
	byAgent := map[string]bool{}
	pid, command := 0, ""
	for _, line := range strings.Split(out, "\n") {
		if field, ok := strings.CutPrefix(line, "p"); ok {
			pid, _ = strconv.Atoi(field)
			command = ""
		} else if field, ok := strings.CutPrefix(line, "c"); ok {
			command = field
		} else if open, ok := strings.CutPrefix(line, "n"); ok {
			path, ok := name(open)
			if !ok || pid <= 0 || command == "sqlite3" || (byAgent[path] && !agentCommands[command]) {
				continue
			}
			held[path] = pid
			byAgent[path] = agentCommands[command]
		}
	}
	return held
}

// agentCommands are the process names an agent runs as: cursor-agent
// runs in its bundled node.
var agentCommands = map[string]bool{"node": true, "cursor-agent": true}

// AppRunning looks for the app's main executable in the process list:
// on macOS its command is the full path inside the app bundle, such as
// /Applications/Cursor.app/Contents/MacOS/Cursor; elsewhere, its name.
func (SystemProcesses) AppRunning(name string) bool {
	if nativeProcesses != nil {
		table, err := nativeProcesses.processes()
		return err == nil && nativeAppRunning(table, name)
	}
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

// Claude Code puts these in the environment of every command its Bash
// tool runs, and a program such as codex inherits them. Only these two
// are read; every other variable, credentials among them, is dropped
// as the line is read and never kept or logged.
const (
	claudeSessionVar = "CLAUDE_CODE_SESSION_ID"
	claudeChildVar   = "CLAUDE_CODE_CHILD_SESSION"
)

// environmentReason is why process environments could not be read.
type environmentReason string

const (
	// environmentUnsupported: this system has no way to read them here.
	environmentUnsupported environmentReason = "this system cannot read another process's environment"
	// environmentUnreadable: the reader failed, such as ps exiting with an error.
	environmentUnreadable environmentReason = "the environment of running processes could not be read"
)

// environmentError is the one error ClaudeParents returns. cause holds
// the reader's own error for logs; it never holds an environment value.
type environmentError struct {
	reason environmentReason
	cause  error
}

func (e *environmentError) Error() string {
	if e.cause == nil {
		return string(e.reason)
	}
	return string(e.reason) + ": " + e.cause.Error()
}

func (e *environmentError) Unwrap() error { return e.cause }

// environmentRead reads the Claude Code entries of the environments of
// pids, one list of NAME=value entries per process that runs. Each
// system that can read another process's environment sets
// environmentReader in its own file; on any other it is nil.
//
// A reader keeps only the entries keptEntries keeps, and only entries
// whose boundaries it read from the environment itself: never words
// split out of a command line, whose arguments or values could hold
// such a name.
type environmentRead func(pids []int) (map[int][]string, error)

func (SystemProcesses) ClaudeParents(pids []int) (map[int]string, error) {
	parents := map[int]string{}
	if environmentReader == nil {
		return parents, &environmentError{reason: environmentUnsupported}
	}
	if len(pids) == 0 {
		return parents, nil
	}
	environments, err := environmentReader(pids)
	if err != nil {
		return parents, &environmentError{reason: environmentUnreadable, cause: err}
	}
	for pid, entries := range environments {
		if parent, ok := claudeParentIn(entries); ok {
			parents[pid] = parent
		}
	}
	return parents, nil
}

// keptEntries keeps the entries of an environment that name the Claude
// Code session or the Konsole tab, and drops every other, credentials
// among them.
func keptEntries(entries []string) []string {
	var kept []string
	for _, entry := range entries {
		name, _, _ := strings.Cut(entry, "=")
		if name == claudeSessionVar || name == claudeChildVar || name == konsoleServiceVar || name == konsoleSessionVar {
			kept = append(kept, entry)
		}
	}
	return kept
}

// claudeParentIn finds the session id in a process's NAME=value entries:
// set only when CLAUDE_CODE_CHILD_SESSION is 1 and the id is a session
// id. A name set twice is ambiguous and links nothing.
func claudeParentIn(entries []string) (string, bool) {
	values := map[string][]string{}
	for _, entry := range entries {
		name, value, found := strings.Cut(entry, "=")
		if found {
			values[name] = append(values[name], value)
		}
	}
	sessions, children := values[claudeSessionVar], values[claudeChildVar]
	if len(sessions) != 1 || len(children) != 1 {
		return "", false
	}
	if children[0] != "1" || !sessionIDPattern.MatchString(sessions[0]) {
		return "", false
	}
	return sessions[0], true
}

func (SystemProcesses) ClaudeJobs(ctx context.Context) ([]ClaudeJob, error) {
	locate := func() (string, error) { return findClaude(claudeFolders()) }
	return listClaudeJobs(ctx, locate, runClaudeCommand)
}

// outOfSight names the paths, of those held does not list, that one
// lsof +D run over dir cannot have seen. lsof skips a folder it cannot
// read and does not follow a link, and says nothing about either, so
// such a path proves nothing about closure. A missing path is in sight:
// nothing holds it. It costs one lstat per path and one open per folder.
func outOfSight(dir string, paths []string, held map[string]int) map[string]bool {
	unseen := map[string]bool{}
	listable := map[string]bool{}
	readable := func(folder string) bool {
		ok, known := listable[folder]
		if !known {
			ok = folder == dir || !isLink(folder)
			if ok {
				f, err := os.Open(folder)
				ok = err == nil
				if ok {
					_ = f.Close()
				}
			}
			listable[folder] = ok
		}
		return ok
	}
	for _, path := range paths {
		if _, isHeld := held[path]; isHeld {
			continue
		}
		info, err := os.Lstat(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil || info.Mode()&fs.ModeSymlink != 0 {
			unseen[path] = true
			continue
		}
		for folder := filepath.Dir(path); ; folder = filepath.Dir(folder) {
			if !readable(folder) {
				unseen[path] = true
				break
			}
			if folder == dir || folder == filepath.Dir(folder) {
				break
			}
		}
	}
	return unseen
}

func isLink(path string) bool {
	info, err := os.Lstat(path)
	return err != nil || info.Mode()&fs.ModeSymlink != 0
}
