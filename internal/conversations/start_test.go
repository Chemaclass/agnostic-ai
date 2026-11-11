package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

const self = "/usr/local/bin/agnostic-ai"

var tabLine = regexp.MustCompile(`^new tab Ghostty cd '/tmp/app' && '/usr/local/bin/agnostic-ai' conversations run ([0-9a-f]{32})$`)

// ghosttyChain is agnostic-ai run by Claude Code's shell, in a Ghostty tab.
func ghosttyChain() []ProcessInfo {
	table := chain("agnostic-ai", "/bin/zsh", "claude", "/bin/zsh", "/usr/bin/login", "/Applications/Ghostty.app/Contents/MacOS/ghostty")
	table[2].TTY, table[3].TTY, table[4].TTY = "/dev/ttys003", "/dev/ttys003", "/dev/ttys003"
	return table
}

type fakeStart struct {
	starter  Starter
	launcher *fakeLauncher
	stores   Stores
}

func newFakeStart(t *testing.T, table []ProcessInfo, installed ...string) fakeStart {
	t.Helper()
	_, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeLiveSessionFile(t, stores, "98", started, idParent)
	launcher := &fakeLauncher{}
	starter := Starter{
		Stores:    stores,
		Processes: fakeProcesses{table: table},
		Launcher:  launcher,
		LookPath: func(name string) (string, error) {
			for _, tool := range installed {
				if tool == name {
					return "/opt/bin/" + name, nil
				}
			}
			return "", exec.ErrNotFound
		},
		Getenv:    env(nil),
		BriefsDir: filepath.Join(t.TempDir(), "agnostic-ai", "conversation-briefs"),
		Now:       func() time.Time { return time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC) },
	}
	return fakeStart{starter: starter, launcher: launcher, stores: stores}
}

func request(tool Tool, brief string) StartRequest {
	return StartRequest{Tool: tool, Folder: "/tmp/app", Brief: brief, Self: self, PID: 100}
}

func briefFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func readBrief(t *testing.T, path string) storedBrief {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var brief storedBrief
	if err := json.Unmarshal(data, &brief); err != nil {
		t.Fatal(err)
	}
	return brief
}

func TestStart_OpensATabThatRunsAOneTimeBriefInTheSameTool(t *testing.T) {
	start := newFakeStart(t, ghosttyChain(), "claude")

	reply, err := start.starter.Start(context.Background(), request("", "Write the release notes\n## Context\nBranch main."))

	if err != nil {
		t.Fatal(err)
	}
	if len(start.launcher.calls) != 1 || !tabLine.MatchString(start.launcher.calls[0]) {
		t.Fatalf("launcher calls = %q", start.launcher.calls)
	}
	id := tabLine.FindStringSubmatch(start.launcher.calls[0])[1]
	path := filepath.Join(start.starter.BriefsDir, id+".json")
	if brief := readBrief(t, path); brief.Tool != ToolClaude || brief.Prompt != "Continues claude:"+idParent+"\n\nWrite the release notes\n## Context\nBranch main." {
		t.Errorf("brief = %+v", brief)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		folder, _ := os.Stat(start.starter.BriefsDir)
		if info.Mode().Perm() != 0o600 || folder.Mode().Perm() != 0o700 {
			t.Errorf("modes: file %v folder %v", info.Mode().Perm(), folder.Mode().Perm())
		}
	}
	if reply.Parent != (Ref{Tool: ToolClaude, ID: idParent}) || reply.Tool != ToolClaude || reply.Message != "Started Claude Code in a new Ghostty tab in /tmp/app, linked to claude:"+idParent+"." {
		t.Errorf("reply = %+v", reply)
	}
}

func TestStart_KeepsTheBriefOutOfEveryLauncherArgument(t *testing.T) {
	start := newFakeStart(t, ghosttyChain(), "codex")
	brief := "Quote \" and ' and `uname` and $(date); rm -rf x\nend tell\n\"& do shell script \"id\""

	if _, err := start.starter.Start(context.Background(), request(ToolCodex, brief)); err != nil {
		t.Fatal(err)
	}

	for _, call := range start.launcher.calls {
		for _, piece := range []string{"Quote", "uname", "$(date)", "rm -rf", "end tell", "do shell script"} {
			if strings.Contains(call, piece) {
				t.Errorf("launcher call %q holds %q", call, piece)
			}
		}
	}
	id := tabLine.FindStringSubmatch(start.launcher.calls[0])[1]
	if stored := readBrief(t, filepath.Join(start.starter.BriefsDir, id+".json")); stored.Tool != ToolCodex || stored.Prompt != "Continues claude:"+idParent+"\n\n"+brief {
		t.Errorf("another tool: brief = %+v", stored)
	}
}

func TestStart_QuotesAFolderAndBinaryWithSpacesAndQuotes(t *testing.T) {
	start := newFakeStart(t, ghosttyChain(), "claude")
	req := request("", "Write the notes")
	req.Folder, req.Self = "/tmp/it's here", "/Apps/my tools/agnostic-ai"

	if _, err := start.starter.Start(context.Background(), req); err != nil {
		t.Fatal(err)
	}

	if call := start.launcher.calls[0]; !strings.HasPrefix(call, `new tab Ghostty cd '/tmp/it'\''s here' && '/Apps/my tools/agnostic-ai' conversations run `) {
		t.Errorf("call = %q", call)
	}

	for name, req := range map[string]StartRequest{
		"folder": {Folder: "/tmp/a\x03\rtouch pwned\r\x1b[201~", Self: self},
		"binary": {Folder: "/tmp/app", Self: "/opt/agnostic\x7f-ai"},
		"tab":    {Folder: "/tmp/a\tb", Self: self},
	} {
		start := newFakeStart(t, ghosttyChain(), "claude")
		req.Brief, req.PID = "Write the notes", 100
		_, err := start.starter.Start(context.Background(), req)
		if err == nil || !strings.Contains(err.Error(), "control character") {
			t.Errorf("%s: err = %v", name, err)
		}
		if len(start.launcher.calls) != 0 || len(briefFiles(t, start.starter.BriefsDir)) != 0 {
			t.Errorf("%s: calls %q files %q", name, start.launcher.calls, briefFiles(t, start.starter.BriefsDir))
		}
	}
}

func TestOpen_NeverTypesAResumeLineWithControlCharacters(t *testing.T) {
	for _, tab := range []*terminalHost{nil, {App: "Ghostty"}, {App: "Terminal"}} {
		launcher := &fakeLauncher{}
		c := Conversation{Tool: ToolClaude, ID: idSafari, App: AppCLI, Open: Route{Kind: RouteTerminal, Command: resumeIn("/tmp/a\x03\rtouch pwned\r", "claude --resume "+idSafari)}, lastTab: tab}

		reply := Open(context.Background(), c, fakeProcesses{}, launcher)

		if reply.Opened || reply.Command != "" || len(launcher.calls) != 0 || !strings.Contains(reply.Message, "control characters") {
			t.Errorf("tab %+v: reply %+v calls %q", tab, reply, launcher.calls)
		}
	}
}

func TestStart_RefusesBeforeWritingOrOpeningAnything(t *testing.T) {
	for name, tc := range map[string]struct {
		table     []ProcessInfo
		installed []string
		req       StartRequest
		want      string
	}{
		"unknown session":    {chain("agnostic-ai", "/bin/zsh"), []string{"claude"}, request("", "Write the notes"), "cannot tell which conversation"},
		"tool not installed": {ghosttyChain(), nil, request(ToolCodex, "Write the notes"), "Codex is not installed: no codex command on PATH"},
		"empty brief":        {ghosttyChain(), []string{"claude"}, request("", " \n\t "), "no brief"},
		"long brief":         {ghosttyChain(), []string{"claude"}, request("", strings.Repeat("x", briefLimit+1)), "the brief is 4097 bytes"},
		"cursor":             {ghosttyChain(), []string{"cursor-agent"}, request(ToolCursor, "Write the notes"), "Cursor terminal agent is not supported yet"},
		"unknown tool":       {ghosttyChain(), []string{"claude"}, request("gemini", "Write the notes"), `unknown tool "gemini"`},
	} {
		start := newFakeStart(t, tc.table, tc.installed...)
		_, err := start.starter.Start(context.Background(), tc.req)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
		if len(start.launcher.calls) != 0 || len(briefFiles(t, start.starter.BriefsDir)) != 0 {
			t.Errorf("%s: calls %q files %q", name, start.launcher.calls, briefFiles(t, start.starter.BriefsDir))
		}
	}
}

func TestStart_SaysTheLineToRunWhenNoTabOpens(t *testing.T) {
	for name, tc := range map[string]struct {
		err  error
		want string
	}{
		"not here": {errNotHere, "this system cannot open a terminal tab; run this line in a terminal: "},
		"denied":   {errAutomationDenied, "macOS blocked opening Ghostty"},
		"waiting":  {context.DeadlineExceeded, "macOS is waiting for you to allow control of Ghostty"},
		"failed":   {errors.New("boom"), "could not open a Ghostty tab"},
	} {
		start := newFakeStart(t, ghosttyChain(), "claude")
		start.launcher.err = tc.err
		_, err := start.starter.Start(context.Background(), request("", "Write the notes"))
		if err == nil || !strings.Contains(err.Error(), tc.want) || !regexp.MustCompile(`cd '/tmp/app' && '/usr/local/bin/agnostic-ai' conversations run [0-9a-f]{32}$`).MatchString(err.Error()) {
			t.Errorf("%s: err = %v", name, err)
		}
		if len(briefFiles(t, start.starter.BriefsDir)) != 1 {
			t.Errorf("%s: the brief the line names is gone", name)
		}
	}
}

func TestStart_OpensATerminalWindowFromAnAppWithoutTabs(t *testing.T) {
	table := ghosttyChain()
	table[len(table)-1].Command = "/Applications/iTerm.app/Contents/MacOS/iTerm2"
	start := newFakeStart(t, table, "claude")

	reply, err := start.starter.Start(context.Background(), request("", "Write the notes"))

	if err != nil || len(start.launcher.calls) != 1 || !strings.HasPrefix(start.launcher.calls[0], "terminal cd '/tmp/app' && ") {
		t.Fatalf("calls %q err %v", start.launcher.calls, err)
	}
	if !strings.HasPrefix(reply.Message, "Started Claude Code in a new Terminal window in /tmp/app") {
		t.Errorf("message = %q", reply.Message)
	}
}

func TestStart_FindsTheTerminalOfACodexThreadWithoutATTY(t *testing.T) {
	table := chain("agnostic-ai", "/bin/zsh", "/opt/bin/codex")
	start := newFakeStart(t, table, "claude")
	if err := os.Remove(filepath.Join(start.stores.ClaudeDir, "sessions", "98.json")); err != nil {
		t.Fatal(err)
	}
	start.starter.Getenv = env(map[string]string{"CODEX_THREAD_ID": idChild, "TERM_PROGRAM": "ghostty"})

	reply, err := start.starter.Start(context.Background(), request(ToolClaude, "Write the notes"))

	if err != nil || len(start.launcher.calls) != 1 || !tabLine.MatchString(start.launcher.calls[0]) {
		t.Fatalf("calls %q err %v", start.launcher.calls, err)
	}
	if reply.Parent != (Ref{Tool: ToolCodex, ID: idChild}) {
		t.Errorf("parent = %+v", reply.Parent)
	}
}

func TestStart_SweepsBriefsLeftForADay(t *testing.T) {
	start := newFakeStart(t, ghosttyChain(), "claude")
	dir := start.starter.BriefsDir
	old, fresh := filepath.Join(dir, strings.Repeat("a", 32)+".json"), filepath.Join(dir, strings.Repeat("b", 32)+".json")
	other := filepath.Join(dir, "notes.txt")
	for _, path := range []string{old, fresh, other} {
		write(t, path, "{}")
	}
	day := start.starter.Now().Add(-25 * time.Hour)
	for _, path := range []string{old, other} {
		if err := os.Chtimes(path, day, day); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := start.starter.Start(context.Background(), request("", "Write the notes")); err != nil {
		t.Fatal(err)
	}

	for path, kept := range map[string]bool{old: false, fresh: true, other: true} {
		if _, err := os.Stat(path); (err == nil) != kept {
			t.Errorf("%s kept = %v, want %v", filepath.Base(path), err == nil, kept)
		}
	}
}

type fakeExec struct {
	path string
	argv []string
}

func runner(t *testing.T, dir string, ran *fakeExec) BriefRunner {
	t.Helper()
	return BriefRunner{
		BriefsDir: dir,
		LookPath:  func(name string) (string, error) { return "/opt/bin/" + name, nil },
		Exec: func(path string, argv, _ []string) error {
			ran.path, ran.argv = path, argv
			return nil
		},
		Environ: func() []string { return []string{"HOME=/home/me"} },
	}
}

func startedBrief(t *testing.T) (dir, id string) {
	t.Helper()
	start := newFakeStart(t, ghosttyChain(), "codex")
	if _, err := start.starter.Start(context.Background(), request(ToolCodex, "Write the notes")); err != nil {
		t.Fatal(err)
	}
	return start.starter.BriefsDir, tabLine.FindStringSubmatch(start.launcher.calls[0])[1]
}

func TestRunBrief_StartsTheToolWithTheBriefAsOneArgumentOnce(t *testing.T) {
	dir, id := startedBrief(t)
	var ran fakeExec

	if err := runner(t, dir, &ran).Run(id); err != nil {
		t.Fatal(err)
	}

	if ran.path != "/opt/bin/codex" || len(ran.argv) != 2 || ran.argv[0] != "codex" || ran.argv[1] != "Continues claude:"+idParent+"\n\nWrite the notes" {
		t.Errorf("exec %q %q", ran.path, ran.argv)
	}
	if files := briefFiles(t, dir); len(files) != 0 {
		t.Errorf("the brief is still there: %q", files)
	}
	again := fakeExec{}
	if err := runner(t, dir, &again).Run(id); err == nil || !strings.Contains(err.Error(), "already used") || again.path != "" {
		t.Errorf("a second run: err %v exec %q", err, again.path)
	}
}

func TestRunBrief_RefusesABadIDOrAnUnsafeFile(t *testing.T) {
	dir, id := startedBrief(t)
	outside := filepath.Join(filepath.Dir(dir), "outside.json")
	write(t, outside, `{"tool":"codex","prompt":"Continues claude:`+idParent+`\n\nx"}`)
	var ran fakeExec
	for _, bad := range []string{"../outside", strings.ToUpper(id), id + "0", "", id[:31] + "g"} {
		if err := runner(t, dir, &ran).Run(bad); err == nil || !strings.Contains(err.Error(), "not a brief id") {
			t.Errorf("id %q: err = %v", bad, err)
		}
	}
	if _, err := os.Stat(outside); err != nil || ran.path != "" {
		t.Errorf("a bad id reached the file system: %v %q", err, ran.path)
	}
	if runtime.GOOS == "windows" {
		return
	}

	path := filepath.Join(dir, id+".json")
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runner(t, dir, &ran).Run(id); err == nil || !strings.Contains(err.Error(), "others can read") || ran.path != "" {
		t.Errorf("a readable brief: err %v exec %q", err, ran.path)
	}

	linked := strings.Repeat("c", 32)
	if err := os.Symlink(outside, filepath.Join(dir, linked+".json")); err != nil {
		t.Fatal(err)
	}
	if err := runner(t, dir, &ran).Run(linked); err == nil || !strings.Contains(err.Error(), "not a regular file") || ran.path != "" {
		t.Errorf("a linked brief: err %v exec %q", err, ran.path)
	}
}

func TestRunBrief_KeepsABriefWhoseJSONGrows(t *testing.T) {
	for _, brief := range []string{strings.Repeat("<", 3000), strings.Repeat("é", 2048), strings.Repeat("\x01", 4000)} {
		start := newFakeStart(t, ghosttyChain(), "codex")
		if _, err := start.starter.Start(context.Background(), request(ToolCodex, brief)); err != nil {
			t.Fatalf("%q: %v", brief[:2], err)
		}
		var ran fakeExec
		id := tabLine.FindStringSubmatch(start.launcher.calls[0])[1]
		if err := runner(t, start.starter.BriefsDir, &ran).Run(id); err != nil || len(ran.argv) != 2 || ran.argv[1] != "Continues claude:"+idParent+"\n\n"+brief {
			t.Errorf("%q: err %v argv ok %v", brief[:2], err, len(ran.argv) == 2 && ran.argv[1] == "Continues claude:"+idParent+"\n\n"+brief)
		}
	}
}

func TestStart_RefusesABriefWithANullByteOrBrokenText(t *testing.T) {
	for _, brief := range []string{"a\x00b", "bad\xff\xfetext"} {
		start := newFakeStart(t, ghosttyChain(), "claude")
		_, err := start.starter.Start(context.Background(), request("", brief))
		if err == nil || !strings.Contains(err.Error(), "the brief holds") || len(briefFiles(t, start.starter.BriefsDir)) != 0 {
			t.Errorf("%q: err %v files %q", brief, err, briefFiles(t, start.starter.BriefsDir))
		}
	}
}

func TestRunBrief_KeepsTheBriefWhenTheToolIsMissing(t *testing.T) {
	dir, id := startedBrief(t)
	var ran fakeExec
	missing := runner(t, dir, &ran)
	missing.LookPath = func(string) (string, error) { return "", exec.ErrNotFound }

	if err := missing.Run(id); err == nil || !strings.Contains(err.Error(), "Codex is not installed") || ran.path != "" {
		t.Errorf("err %v exec %q", err, ran.path)
	}
	if files := briefFiles(t, dir); len(files) != 1 {
		t.Errorf("the brief was removed: %q", files)
	}
	if err := runner(t, dir, &ran).Run(id); err != nil {
		t.Errorf("the brief no longer runs once the tool is there: %v", err)
	}
}

func TestRunBrief_RefusesWithoutABriefsFolder(t *testing.T) {
	dir := t.TempDir()
	id := strings.Repeat("a", 32)
	planted := filepath.Join(dir, id+".json")
	write(t, planted, `{"tool":"claude","prompt":"Continues claude:`+idParent+`\n\nplanted"}`)
	if err := os.Chmod(planted, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	var ran fakeExec

	if err := runner(t, "", &ran).Run(id); err == nil || ran.path != "" {
		t.Errorf("err %v exec %q", err, ran.path)
	}
}
