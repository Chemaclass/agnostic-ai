package conversations

import (
	"path/filepath"
	"strings"
	"testing"
)

const started = "Sat Nov  7 08:00:00 2026"

// chain is a process table where each process is the parent of the one
// before it: the first is the command, the last is under launchd.
func chain(commands ...string) []ProcessInfo {
	var table []ProcessInfo
	for i, command := range commands {
		table = append(table, ProcessInfo{PID: 100 - i, PPID: 99 - i, Started: started, Command: command})
	}
	table[len(table)-1].PPID = 1
	return table
}

func env(values map[string]string) func(string) string {
	return func(name string) string { return values[name] }
}

func writeLiveSessionFile(t *testing.T, stores Stores, pid, procStart, id string) {
	t.Helper()
	write(t, filepath.Join(stores.ClaudeDir, "sessions", pid+".json"), `{"pid":`+pid+`,"sessionId":"`+id+`","procStart":"`+procStart+`","status":"busy"}`)
}

func writeCodexLocks(t *testing.T, stores Stores, ids ...string) []string {
	t.Helper()
	var paths []string
	for _, id := range ids {
		path := filepath.Join(stores.CodexDir, "thread-writer-locks", id+".lock")
		write(t, path, "")
		paths = append(paths, path)
	}
	return paths
}

func TestCurrentSession_FindsTheNearestClaudeCodeSession(t *testing.T) {
	_, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeLiveSessionFile(t, stores, "98", started, idParent)
	table := chain("agnostic-ai", "/bin/zsh", "/Users/me/.local/bin/claude", "/bin/zsh", "/Applications/Ghostty.app/Contents/MacOS/ghostty")

	ref, pid, err := CurrentSession(fakeProcesses{table: table}, stores, env(nil), 100)

	if err != nil || ref != (Ref{Tool: ToolClaude, ID: idParent}) || pid != 98 {
		t.Errorf("ref = %+v pid %d err %v", ref, pid, err)
	}
}

func TestCurrentSession_SkipsASessionFileOfAnEarlierProcess(t *testing.T) {
	_, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeLiveSessionFile(t, stores, "98", "Fri Nov  6 08:00:00 2026", idParent)
	writeLiveSessionFile(t, stores, "96", started, idChild)
	table := chain("agnostic-ai", "/bin/zsh", "/bin/zsh", "/bin/zsh", "claude", "/bin/zsh")

	ref, pid, err := CurrentSession(fakeProcesses{table: table}, stores, env(nil), 100)

	if err != nil || ref.ID != idChild || pid != 96 {
		t.Errorf("ref = %+v pid %d err %v", ref, pid, err)
	}
}

func TestCurrentSession_TrustsCodexThreadIDOnlyUnderCodex(t *testing.T) {
	_, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeLiveSessionFile(t, stores, "96", started, idParent)
	table := chain("agnostic-ai", "/bin/zsh", "/opt/homebrew/bin/codex", "/bin/zsh", "claude", "/bin/zsh")
	both := env(map[string]string{"CODEX_THREAD_ID": idChild, "CLAUDE_CODE_SESSION_ID": idParent})

	ref, pid, err := CurrentSession(fakeProcesses{table: table}, stores, both, 100)
	if err != nil || ref != (Ref{Tool: ToolCodex, ID: idChild}) || pid != 98 {
		t.Errorf("under Codex inside Claude Code: ref = %+v pid %d err %v", ref, pid, err)
	}

	underClaude := chain("agnostic-ai", "/bin/zsh", "claude", "/bin/zsh")
	writeLiveSessionFile(t, stores, "98", started, idParent)
	ref, _, err = CurrentSession(fakeProcesses{table: underClaude}, stores, env(map[string]string{"CODEX_THREAD_ID": idChild}), 100)
	if err != nil || ref != (Ref{Tool: ToolClaude, ID: idParent}) {
		t.Errorf("a Codex variable leaked into Claude Code: ref = %+v err %v", ref, err)
	}

	_, _, err = CurrentSession(fakeProcesses{table: chain("agnostic-ai", "/bin/zsh")}, stores, env(map[string]string{"CODEX_THREAD_ID": idChild}), 100)
	if err == nil {
		t.Error("a Codex variable with no Codex ancestor names no session")
	}
}

func TestCurrentSession_ReadsTheOneThreadACodexProcessHolds(t *testing.T) {
	_, stores := testScanner(t, noSQLite, fakeProcesses{})
	locks := writeCodexLocks(t, stores, idParent, idChild, idSide)
	table := chain("agnostic-ai", "/bin/zsh", "codex")

	one := fakeProcesses{table: table, held: map[string]int{locks[0]: 98, locks[2]: 4242}}
	ref, pid, err := CurrentSession(one, stores, env(nil), 100)
	if err != nil || ref != (Ref{Tool: ToolCodex, ID: idParent}) || pid != 98 {
		t.Errorf("one thread: ref = %+v pid %d err %v", ref, pid, err)
	}

	several := fakeProcesses{table: table, held: map[string]int{locks[0]: 98, locks[1]: 98}}
	if _, _, err := CurrentSession(several, stores, env(nil), 100); err == nil || !strings.Contains(err.Error(), "holds 2 threads") || !strings.Contains(err.Error(), "CODEX_THREAD_ID") {
		t.Errorf("several threads: err = %v", err)
	}

	none := fakeProcesses{table: table}
	if _, _, err := CurrentSession(none, stores, env(nil), 100); err == nil || !strings.Contains(err.Error(), "holds no thread") {
		t.Errorf("no thread: err = %v", err)
	}
}

func TestCurrentSession_RefusesWithoutAKnownSession(t *testing.T) {
	_, stores := testScanner(t, noSQLite, fakeProcesses{})
	for name, tc := range map[string]struct {
		table []ProcessInfo
		want  string
	}{
		"no tool ancestor": {chain("agnostic-ai", "/bin/zsh", "/Applications/Ghostty.app/Contents/MacOS/ghostty"), "cannot tell which conversation"},
		"cursor agent":     {chain("agnostic-ai", "/bin/zsh", "cursor-agent", "/bin/zsh"), "holds no chat and CURSOR_CONVERSATION_ID is not set"},
		"pid not listed":   {chain("/bin/zsh"), "cannot tell which conversation"},
	} {
		_, _, err := CurrentSession(fakeProcesses{table: tc.table}, stores, env(nil), 100)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}

func TestCurrentSession_StopsAtAToolItCannotResolve(t *testing.T) {
	_, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeLiveSessionFile(t, stores, "96", started, idParent)
	locks := writeCodexLocks(t, stores, idChild, idSide)
	for name, tc := range map[string]struct {
		processes fakeProcesses
		getenv    func(string) string
		want      string
	}{
		"cursor agent in its bundled node, inside Claude Code": {
			fakeProcesses{table: chain("agnostic-ai", "/bin/zsh", "/Users/me/.local/share/cursor-agent/versions/1/node", "/bin/zsh", "claude", "/bin/zsh")},
			env(nil), "runs under Cursor, but its process 98 holds no chat",
		},
		"Claude Code with no session file, inside Codex": {
			fakeProcesses{table: chain("agnostic-ai", "/bin/zsh", "claude", "/bin/zsh", "codex")},
			env(map[string]string{"CODEX_THREAD_ID": idChild}), "runs under Claude Code, but process 98 has no session file",
		},
		"Codex inside Codex with the outer thread's variable": {
			fakeProcesses{table: chain("agnostic-ai", "/bin/zsh", "codex", "/bin/zsh", "codex"), held: map[string]int{locks[0]: 98, locks[1]: 96}},
			env(map[string]string{"CODEX_THREAD_ID": idSide}), "CODEX_THREAD_ID names another thread",
		},
	} {
		ref, _, err := CurrentSession(tc.processes, stores, tc.getenv, 100)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: ref %+v err %v, want %q", name, ref, err, tc.want)
		}
	}
}
