package conversations

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// maxSessionAncestors bounds the walk up from a command to its tool. A
// tool runs commands through shells and wrappers, so the walk goes
// further than hostOf's.
const maxSessionAncestors = 32

// errNoSession says the command runs inside no session it can name.
var errNoSession = errors.New("cannot tell which conversation this runs in: no Claude Code or Codex session is among its parent processes; run it from inside a session")

// CurrentSession finds the conversation that the process pid runs in,
// and the pid of the tool process that holds it. The nearest tool among
// pid's ancestors wins:
//
//   - Claude Code writes sessions/<pid>.json while it runs; its procStart
//     must match the ancestor, since a pid is reused.
//   - Codex runs commands from one process that may hold several
//     threads. CODEX_THREAD_ID names the thread, and is trusted only
//     under a Codex ancestor, since a tool started inside another
//     inherits the outer one's variables. Without it, a Codex process
//     that holds exactly one thread names that one.
//   - Cursor's terminal agent is refused.
func CurrentSession(processes Processes, stores Stores, getenv func(string) string, pid int) (Ref, int, error) {
	table, err := processes.Table()
	if err != nil {
		return Ref{}, 0, fmt.Errorf("could not read the process table: %w", err)
	}
	return currentSession(table, processes, stores, getenv, pid)
}

func currentSession(table []ProcessInfo, processes Processes, stores Stores, getenv func(string) string, pid int) (Ref, int, error) {
	byPID := make(map[int]ProcessInfo, len(table))
	for _, process := range table {
		byPID[process.PID] = process
	}
	process, found := byPID[pid]
	for range maxSessionAncestors {
		if !found {
			break
		}
		if id, ok := claudeSessionOf(stores, process); ok {
			return Ref{Tool: ToolClaude, ID: id}, process.PID, nil
		}
		switch filepath.Base(process.Command) {
		case "codex":
			ref, err := codexThreadOf(processes, stores, getenv, process.PID)
			return ref, process.PID, err
		case "cursor-agent":
			return Ref{}, 0, errors.New("the Cursor terminal agent is not supported yet: starting a linked conversation from it has not been checked")
		}
		if process.PPID <= 1 {
			break
		}
		process, found = byPID[process.PPID]
	}
	return Ref{}, 0, errNoSession
}

// claudeSessionOf reads the session Claude Code process process runs,
// from its sessions/<pid>.json.
func claudeSessionOf(stores Stores, process ProcessInfo) (string, bool) {
	data, err := os.ReadFile(filepath.Join(stores.ClaudeDir, "sessions", strconv.Itoa(process.PID)+".json"))
	if err != nil {
		return "", false
	}
	var entry claudeLive
	if json.Unmarshal(data, &entry) != nil || entry.PID != process.PID || !sessionIDPattern.MatchString(entry.SessionID) {
		return "", false
	}
	if strings.Join(strings.Fields(entry.ProcStart), " ") != strings.Join(strings.Fields(process.Started), " ") {
		return "", false
	}
	return entry.SessionID, true
}

// codexThreadOf names the thread a Codex process runs the command for.
func codexThreadOf(processes Processes, stores Stores, getenv func(string) string, pid int) (Ref, error) {
	if id := getenv("CODEX_THREAD_ID"); sessionIDPattern.MatchString(id) {
		return Ref{Tool: ToolCodex, ID: id}, nil
	}
	locks, _ := filepath.Glob(filepath.Join(stores.CodexDir, "thread-writer-locks", "*.lock"))
	var held []string
	for path, holder := range processes.Holders(locks) {
		id := strings.TrimSuffix(filepath.Base(path), ".lock")
		if holder == pid && sessionIDPattern.MatchString(id) {
			held = append(held, id)
		}
	}
	switch len(held) {
	case 1:
		return Ref{Tool: ToolCodex, ID: held[0]}, nil
	case 0:
		return Ref{}, fmt.Errorf("this command runs under Codex, but its process %d holds no thread and CODEX_THREAD_ID is not set, so the conversation cannot be named", pid)
	default:
		return Ref{}, fmt.Errorf("this command runs under a Codex process that holds %d threads, and CODEX_THREAD_ID is not set, so it cannot tell which one this is; close the other Codex threads, or start the conversation by hand", len(held))
	}
}
