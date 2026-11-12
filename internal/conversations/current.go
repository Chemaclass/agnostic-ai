package conversations

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// maxSessionAncestors bounds the walk up from a command to its tool. A
// tool runs commands through shells and wrappers, so the walk goes
// further than hostOf's.
const maxSessionAncestors = 32

// errNoSession says the command runs inside no session it can name.
var errNoSession = errors.New("cannot tell which conversation this runs in: no Claude Code, Codex or Cursor terminal agent session is among its parent processes; run it from inside a session")

// errCursorEditor says the command runs in the Cursor editor with no
// session it can name in between. A tool it cannot recognise, such as
// a Claude Code that runs as node, may sit in between.
var errCursorEditor = errors.New("cannot tell which conversation this runs in: it runs inside the Cursor editor, and no Claude Code, Codex or cursor-agent session it can name is in between; a Cursor editor chat cannot start a linked conversation, since it does not run in a terminal tab")

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
//   - Cursor's terminal agent sets CURSOR_CONVERSATION_ID for the
//     commands it runs, and holds the chat's store.db open; they are
//     checked against each other as Codex's are.
//   - The Cursor editor is refused.
//
// A tool ancestor that cannot be resolved ends the walk with an error.
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
		// A tool ancestor ends the walk even when it cannot be resolved:
		// going on would name an outer session this command does not run in.
		switch {
		case isCursorAgent(process.Command):
			ref, err := cursorChatOf(processes, stores, getenv, process.PID)
			return ref, process.PID, err
		case isCursorEditor(process.Command):
			return Ref{}, 0, errCursorEditor
		case filepath.Base(process.Command) == "claude":
			return Ref{}, 0, fmt.Errorf("this command runs under Claude Code, but process %d has no session file that matches it, so the conversation cannot be named", process.PID)
		case filepath.Base(process.Command) == "codex":
			ref, err := codexThreadOf(processes, stores, getenv, process.PID)
			return ref, process.PID, err
		}
		if process.PPID <= 1 {
			break
		}
		process, found = byPID[process.PPID]
	}
	return Ref{}, 0, errNoSession
}

// isCursorAgent reports whether command runs cursor-agent: its own
// binary, or the node bundled under its install folder.
func isCursorAgent(command string) bool {
	return filepath.Base(command) == "cursor-agent" || strings.Contains(filepath.ToSlash(command), "/cursor-agent/")
}

// isCursorEditor reports whether command runs from the Cursor app,
// whose helpers run the editor's terminals and its chats' commands.
func isCursorEditor(command string) bool {
	return strings.Contains(filepath.ToSlash(command), "/Cursor.app/")
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
	locks, _ := filepath.Glob(filepath.Join(stores.CodexDir, "thread-writer-locks", "*.lock"))
	var held []string
	for path, holder := range processes.Holders(locks) {
		id := strings.TrimSuffix(filepath.Base(path), ".lock")
		if holder == pid && sessionIDPattern.MatchString(id) {
			held = append(held, id)
		}
	}
	return heldBy(heldSession{tool: ToolCodex, noun: "thread", variable: "CODEX_THREAD_ID", pattern: sessionIDPattern}, getenv, pid, held)
}

// cursorChatVariable is what cursor-agent sets, for each command it
// runs, to the id of the chat that runs it.
const cursorChatVariable = "CURSOR_CONVERSATION_ID"

// cursorChatOf names the chat a cursor-agent process runs the command
// for. A running chat holds chats/<folder hash>/<chat id>/store.db.
func cursorChatOf(processes Processes, stores Stores, getenv func(string) string, pid int) (Ref, error) {
	dbs, _ := filepath.Glob(filepath.Join(stores.CursorDir, "chats", "*", "*", "store.db"))
	var held []string
	for path, holder := range processes.Holders(dbs) {
		id := filepath.Base(filepath.Dir(path))
		if holder == pid && cursorIDPattern.MatchString(id) {
			held = append(held, id)
		}
	}
	return heldBy(heldSession{tool: ToolCursor, noun: "chat", variable: cursorChatVariable, pattern: cursorIDPattern}, getenv, pid, held)
}

// heldSession is how a tool that runs several sessions in one process
// names the one a command runs for.
type heldSession struct {
	tool Tool
	// noun is what the tool calls a session, such as "thread".
	noun string
	// variable names the session in the environment of the commands
	// the tool runs.
	variable string
	pattern  *regexp.Regexp
}

// heldBy names the session among held, the ones the tool process pid
// holds. The variable is trusted only under that process, since a tool
// started inside another inherits the outer one's variables; one that
// names a session pid does not hold was inherited, so it is refused.
func heldBy(kind heldSession, getenv func(string) string, pid int, held []string) (Ref, error) {
	label := toolLabels[kind.tool]
	if id := getenv(kind.variable); kind.pattern.MatchString(id) {
		if len(held) > 0 && !slices.Contains(held, id) {
			return Ref{}, fmt.Errorf("%s names a %s that %s process %d does not hold; it may come from an outer %s, so the conversation cannot be named", kind.variable, kind.noun, label, pid, label)
		}
		return Ref{Tool: kind.tool, ID: id}, nil
	}
	switch len(held) {
	case 1:
		return Ref{Tool: kind.tool, ID: held[0]}, nil
	case 0:
		return Ref{}, fmt.Errorf("this command runs under %s, but its process %d holds no %s and %s is not set, so the conversation cannot be named", label, pid, kind.noun, kind.variable)
	default:
		return Ref{}, fmt.Errorf("this command runs under a %s process that holds %d %ss, and %s is not set, so it cannot tell which one this is; close the other %s %ss, or start the conversation by hand", label, len(held), kind.noun, kind.variable, label, kind.noun)
	}
}
