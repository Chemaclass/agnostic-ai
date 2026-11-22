package conversations

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/preview"
)

const forgottenCommand = "bash scripts/run-preview.bash 'hello'"

func forgottenShellScanner(t *testing.T) (*Scanner, *fakeProcesses, string, string) {
	t.Helper()
	started := time.Date(2026, 11, 14, 9, 0, 0, 0, time.UTC)
	processes := &fakeProcesses{
		alive: map[int]bool{4242: true},
		table: []ProcessInfo{
			{PID: 4242, PGID: 4000, Started: started.Add(-time.Hour).Format(lstartLayout)},
			{PID: 4243, PPID: 4242, PGID: 4243, Started: started.Format(lstartLayout)},
			{PID: 4244, PPID: 4243, PGID: 4243, Started: started.Format(lstartLayout)},
			{PID: 4245, PPID: 4242, PGID: 4245, Started: started.Format(lstartLayout)},
			{PID: 4246, PPID: 4243, PGID: 4246, Started: started.Format(lstartLayout)},
		},
		args: map[int]string{
			4243: "/bin/zsh -c source /tmp/claude/shell-snapshots/snapshot-zsh-123.sh 2>/dev/null || true && eval " + shellQuote(forgottenCommand) + " < /dev/null && pwd -P >| /tmp/claude-123-cwd",
			4245: "/bin/zsh -c unrelated",
		},
	}
	scanner, stores := testScanner(t, nil, processes)
	scanner.Now = func() time.Time { return started.Add(31 * time.Minute) }
	project := filepath.Join(stores.ClaudeDir, "projects", "proj", idSafari+".jsonl")
	write(t, project, lines(typedPrompt("Run the preview"), agentLine(textPart("The preview is ready."))))
	session := filepath.Join(stores.ClaudeDir, "sessions", "4242.json")
	write(t, session, lines(obj{"sessionId": idSafari, "pid": 4242, "procStart": processes.table[0].Started, "startedAt": started.Add(-time.Hour).UnixMilli(), "status": "shell"}))
	return scanner, processes, project, session
}

func TestStuckShell_TracksThresholdAndOtherWork(t *testing.T) {
	scanner, processes, path, _ := forgottenShellScanner(t)
	ctx := context.Background()
	scanner.MarksPath = filepath.Join(scanner.Stores.Home, "marks.json")
	if err := scanner.Mark(ToolClaude, idSafari, MarkSeen); err != nil {
		t.Fatal(err)
	}
	scan := func(opts ScanOptions) Conversation { return byID(t, scanner.Scan(ctx, opts), idSafari) }
	longer := 32 * time.Minute
	disabled := time.Duration(0)
	for _, threshold := range []*time.Duration{&longer, &disabled} {
		if c := scan(ScanOptions{StuckShellAfter: threshold}); len(c.StuckShells) != 0 || c.State != StateWorking {
			t.Errorf("row with threshold %v = %+v", *threshold, c)
		}
	}
	c := scan(ScanOptions{})
	if runtime.GOOS == "windows" {
		if len(c.StuckShells) != 0 {
			t.Errorf("Windows showed shells: %+v", c.StuckShells)
		}
		return
	}
	if len(c.StuckShells) != 1 || c.State != StateBlocked || c.Waiting == nil || c.Waiting.For != "shell" {
		t.Fatalf("blocked row = %+v", c)
	}
	if shell := c.StuckShells[0]; shell.PID != 4243 || shell.Command != forgottenCommand || shell.Since != workerTime || shell.ProcessCount != 2 || shell.HelperName != "" {
		t.Errorf("shell = %+v", shell)
	}
	dispatch := agentLine(obj{"type": "tool_use", "name": "Bash", "id": "preview", "input": obj{"command": forgottenCommand, "run_in_background": true}})
	appendWorkerEvent(t, path, dispatch)
	appendWorkerEvent(t, path, workerResult("preview", obj{"backgroundTaskId": "proof"}))
	appendWorkerEvent(t, path, workerDispatch("other-work"))
	appendWorkerEvent(t, path, workerResult("other-work", obj{"status": "async_launched", "agentId": "other-agent"}))
	processes.args[4245] = "/bin/zsh -c source /tmp/claude/shell-snapshots/snapshot-zsh-123.sh 2>/dev/null || true && eval 'another command' < /dev/null && pwd -P >| /tmp/claude-123-cwd"
	if c := scan(ScanOptions{}); c.State != StateWorking || len(c.StuckShells) != 2 || c.StuckShells[0].PID != 4243 || c.StuckShells[1].PID != 4245 {
		t.Errorf("row with other work = %+v", c)
	}
	processes.table = processes.table[:1]
	if c := scan(ScanOptions{}); len(c.StuckShells) != 0 {
		t.Errorf("exited shell remains: %+v", c.StuckShells)
	}
}

func TestStuckShell_PairsTheSuccessfulResultWithItsHelper(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not read shells")
	}
	scanner, _, path, _ := forgottenShellScanner(t)
	helper := filepath.Join(claudeSubagentsDir(path), "agent-abc.jsonl")
	write(t, helper, "")
	meta := filepath.Join(claudeSubagentsDir(path), "agent-abc.meta.json")
	write(t, meta, lines(obj{"description": "Run the preview", "agentType": "Faramir"}))
	dispatch := agentLine(obj{"type": "tool_use", "name": "Bash", "id": "preview", "input": obj{"command": forgottenCommand, "run_in_background": true}})
	dispatch["isSidechain"] = true
	appendWorkerEvent(t, helper, workerResult("other", obj{"backgroundTaskId": "bad"}))
	scan := func() StuckShell {
		t.Helper()
		c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)
		if len(c.StuckShells) != 1 {
			t.Fatalf("shells = %+v", c.StuckShells)
		}
		return c.StuckShells[0]
	}
	if shell := scan(); shell.HelperID != "" || shell.HelperName != "" {
		t.Errorf("unmatched shell = %+v", shell)
	}
	appendWorkerEvent(t, helper, dispatch)
	appendWorkerEvent(t, helper, workerResult("preview", obj{"backgroundTaskId": "good"}))
	if shell := scan(); shell.HelperID != "abc" || shell.HelperName != "Faramir" {
		t.Errorf("matched shell = %+v", shell)
	}
	write(t, meta, lines(obj{"description": "Run the preview"}))
	if shell := scan(); shell.HelperID != "abc" || shell.HelperName != "Run the preview" {
		t.Errorf("helper without agent type = %+v", shell)
	}
	delete(dispatch, "isSidechain")
	appendWorkerEvent(t, path, dispatch)
	appendWorkerEvent(t, path, workerResult("preview", obj{"backgroundTaskId": "second-match"}))
	if shell := scan(); shell.HelperID != "" || shell.HelperName != "" {
		t.Errorf("ambiguous shell = %+v", shell)
	}
}

func TestStuckShell_BusyTurnNeedsBackgroundEvidenceAndWithholdsSecrets(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires process groups")
	}
	scanner, processes, path, session := forgottenShellScanner(t)
	const command = "echo sk-ant-api03-AbCd1234EfGh5678"
	processes.args[4243] = "/bin/zsh -c source /tmp/claude/shell-snapshots/snapshot-zsh-123.sh 2>/dev/null || true && \\builtin setopt NO_EQUALS && eval '" + command + "' < /dev/null && pwd -P >| /tmp/claude-123-cwd"
	write(t, session, lines(obj{"sessionId": idSafari, "pid": 4242, "procStart": processes.table[0].Started, "startedAt": time.Date(2026, 11, 14, 8, 0, 0, 0, time.UTC).UnixMilli(), "status": "busy"}))
	dispatch := agentLine(obj{"type": "tool_use", "name": "Bash", "id": "preview", "input": obj{"command": command, "run_in_background": true}})
	appendWorkerEvent(t, path, dispatch)
	scan := func() Conversation { return byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari) }
	if c := scan(); len(c.StuckShells) != 0 || c.State != StateWorking {
		t.Errorf("pending shell = %+v", c)
	}
	appendWorkerEvent(t, path, workerResult("preview", obj{"backgroundTaskId": "good"}))
	if c := scan(); len(c.StuckShells) != 1 || c.StuckShells[0].Command != "" || c.State != StateWorking {
		t.Errorf("busy row shells = %+v, state = %q", c.StuckShells, c.State)
	}
}

func TestStuckShell_EachAgedGroupIsListedAndTheProcessTableIsShared(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires process groups")
	}
	scanner, processes, _, _ := forgottenShellScanner(t)
	processes.args[4245] = processes.args[4243]
	counted := &shellTableReads{fakeProcesses: processes}
	scanner.Processes = counted
	c := byID(t, scanner.Scan(context.Background(), ScanOptions{}), idSafari)
	if len(c.StuckShells) != 2 || c.StuckShells[0].PID != 4243 || c.StuckShells[1].PID != 4245 {
		t.Errorf("shells = %+v", c.StuckShells)
	}
	if counted.reads != 1 {
		t.Errorf("read the process table %d times", counted.reads)
	}
}

type shellTableReads struct {
	*fakeProcesses
	reads      int
	beforeRead func(int)
}

func (p *shellTableReads) Table() ([]ProcessInfo, error) {
	p.reads++
	if p.beforeRead != nil {
		p.beforeRead(p.reads)
	}
	return p.fakeProcesses.Table()
}

func TestStuckShell_DelayedForegroundCallCannotBeStopped(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires process groups")
	}
	scanner, processes, path, _ := forgottenShellScanner(t)
	helper := filepath.Join(claudeSubagentsDir(path), "agent-abc.jsonl")
	write(t, helper, "")
	dispatch := agentLine(obj{"type": "tool_use", "name": "Bash", "id": "foreground", "input": obj{"command": forgottenCommand}})
	dispatch["timestamp"] = "2026-11-14T08:59:00Z"
	dispatch["isSidechain"] = true
	appendWorkerEvent(t, helper, dispatch)
	// The helper may be missing from task lifecycle records. The matching
	// foreground call alone is enough to withhold the stop action.
	ctx := context.Background()
	if c := byID(t, scanner.Scan(ctx, ScanOptions{}), idSafari); len(c.StuckShells) != 0 || c.State != StateWorking {
		t.Errorf("foreground row shells = %+v, state = %q", c.StuckShells, c.State)
	}
	signals := recordShellSignals(scanner)
	scanner.shellWait = func(context.Context, time.Duration) error { return nil }
	target := ProcessRef{PID: 4243, ProcStart: processes.table[1].Started}
	if _, err := scanner.StopShell(ctx, ToolClaude, idSafari, target, ScanOptions{}); !errors.Is(err, ErrShellRefused) || len(*signals) != 0 {
		t.Errorf("err = %v, signals = %+v", err, *signals)
	}
}

func TestStuckShell_UnreadableWorkerLogKeepsTheShellIneligible(t *testing.T) {
	if !canStopShells() {
		t.Skip("requires process groups")
	}
	scanner, _, path, _ := forgottenShellScanner(t)
	helper := filepath.Join(claudeSubagentsDir(path), "agent-abc.jsonl")
	write(t, helper, "unreadable record\n")
	ctx := context.Background()
	if c := byID(t, scanner.Scan(ctx, ScanOptions{}), idSafari); len(c.StuckShells) != 0 || c.State != StateWorking || c.WorkingSubagents != nil {
		t.Errorf("uncertain row shells = %+v, state = %q, workers = %v", c.StuckShells, c.State, c.WorkingSubagents)
	}
	if err := os.Remove(helper); err != nil {
		t.Fatal(err)
	}
	if c := byID(t, scanner.Scan(ctx, ScanOptions{}), idSafari); len(c.StuckShells) != 1 || c.State != StateBlocked {
		t.Errorf("row with complete logs shells = %+v, state = %q", c.StuckShells, c.State)
	}
}

func TestStuckShell_MatchesDarwinMultilineCommandAndRefusesAmbiguousForeground(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("requires Darwin ps command formatting")
	}
	scanner, processes, path, _ := forgottenShellScanner(t)
	helper := filepath.Join(claudeSubagentsDir(path), "agent-multiline.jsonl")
	write(t, helper, "")
	write(t, filepath.Join(claudeSubagentsDir(path), "agent-multiline.meta.json"), lines(obj{"agentType": "Faramir"}))
	const rawCommand = "echo 'first'\n\techo literal\\012"
	const displayedCommand = `echo 'first'\012\011echo literal\012`
	processes.args[4243] = "/bin/zsh -c source /tmp/claude/shell-snapshots/snapshot-zsh-123.sh 2>/dev/null || true && eval " + shellQuote(displayedCommand) + " < /dev/null && pwd -P >| /tmp/claude-123-cwd"
	dispatch := func(id, command string, background bool) obj {
		event := agentLine(obj{"type": "tool_use", "name": "Bash", "id": id, "input": obj{"command": command, "run_in_background": background}})
		event["isSidechain"] = true
		return event
	}
	ctx := context.Background()
	scan := func() Conversation { return byID(t, scanner.Scan(ctx, ScanOptions{}), idSafari) }
	appendWorkerEvent(t, helper, dispatch("background", rawCommand, true))
	if c := scan(); len(c.StuckShells) != 0 {
		t.Errorf("unreturned multiline command was eligible: %+v", c.StuckShells)
	}
	appendWorkerEvent(t, helper, workerResult("background", obj{"backgroundTaskId": "launched"}))
	c := scan()
	if len(c.StuckShells) != 1 || c.StuckShells[0].HelperName != "Faramir" || c.StuckShells[0].Command != `echo 'first' echo literal\012` {
		t.Errorf("matched multiline shell = %+v", c.StuckShells)
	}
	appendWorkerEvent(t, helper, dispatch("collision", displayedCommand, false))
	if c := scan(); len(c.StuckShells) != 0 {
		t.Errorf("a pending command with literal escapes was eligible: %+v", c.StuckShells)
	}
	signals := recordShellSignals(scanner)
	scanner.shellWait = func(context.Context, time.Duration) error { return nil }
	target := ProcessRef{PID: 4243, ProcStart: processes.table[1].Started}
	if _, err := scanner.StopShell(ctx, ToolClaude, idSafari, target, ScanOptions{}); !errors.Is(err, ErrShellRefused) || len(*signals) != 0 {
		t.Errorf("err = %v, signals = %+v", err, *signals)
	}
	appendWorkerEvent(t, helper, workerResult("collision", obj{}))
	appendWorkerEvent(t, helper, dispatch("ambiguous", displayedCommand, true))
	appendWorkerEvent(t, helper, workerResult("ambiguous", obj{"backgroundTaskId": "second"}))
	if c := scan(); len(c.StuckShells) != 1 || c.StuckShells[0].Command != "" || c.StuckShells[0].HelperID != "" {
		t.Errorf("ambiguous shell = %+v", c.StuckShells)
	}
}

func TestShowCommand_WithholdsOrMasksCredentialsAndKeepsLongCommands(t *testing.T) {
	plain := "make test " + strings.Repeat("a", 1490)
	cases := []struct {
		name, command, want string
	}{
		{"bearer token", `curl -H "Authorization: Bearer abcDEF123456ghiJKL789mno" https://api.example.com/v1/items`, ""},
		{"key assignment", "API_KEY=sk-ant-api03-AbCd1234EfGh5678 node server.js", ""},
		{"plain long command", plain, plain},
		{"cut at the limit", strings.Repeat("ls ", 1000), strings.TrimSpace(strings.Repeat("ls ", 1000)[:shellCommandRunes-1]) + "…"},
		{"several lines", "make\n  test", "make test"},
		{"known miss: a password after -p", "mysql -u root -p hunter2pass", "mysql -u root -p hunter2pass"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := showCommand(tc.command); got != tc.want {
				t.Errorf("showCommand() = %q, want %q", got, tc.want)
			}
		})
	}
	t.Run("URL password", func(t *testing.T) {
		if got := showCommand("git clone https://alice:hunter2pass@git.example.com/team/repo.git"); strings.Contains(got, "hunter2pass") {
			t.Errorf("showCommand() = %q, want no password", got)
		}
	})
	t.Run("URL user masked", func(t *testing.T) {
		want := "git clone https://" + preview.Redacted + "@git.example.com/x"
		if got := showCommand("git clone https://alice@git.example.com/x"); got != want {
			t.Errorf("showCommand() = %q, want %q", got, want)
		}
	})
}
