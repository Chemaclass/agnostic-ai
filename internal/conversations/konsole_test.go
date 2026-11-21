package conversations

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

const konsoleDBus = "dbus-send --session --print-reply --dest=:1.0 "

// konsoleReplies are dbus-send's answers from Konsole 23.08: window 2
// holds sessions 2 and 3.
func konsoleReplies() map[string]string {
	return map[string]string{
		konsoleDBus + "/Windows org.freedesktop.DBus.Introspectable.Introspect": "method return\n   string \"<node>\n  <node name=\"1\"/>\n  <node name=\"2\"/>\n</node>\"",
		konsoleDBus + "/Windows/1 org.kde.konsole.Window.sessionList":           "method return\n   array [\n      string \"1\"\n   ]",
		konsoleDBus + "/Windows/2 org.kde.konsole.Window.sessionList":           "method return\n   array [\n      string \"2\"\n      string \"3\"\n   ]",
		konsoleDBus + "/Windows/1 org.kde.konsole.Window.newSession":            "method return\n   int32 5",
	}
}

func testKonsole(commands *fakeCommands) konsoleTab {
	return konsoleTab{
		run: commands.run,
		table: func() ([]ProcessInfo, error) {
			return []ProcessInfo{
				{PID: 28, PPID: 7, Command: "konsole"},
				{PID: 32, PPID: 28, TTY: "/dev/pts/0", Command: "bash"},
				{PID: 40, PPID: 32, TTY: "/dev/pts/0", Command: "claude"},
				{PID: 50, PPID: 28, TTY: "/dev/pts/1", Command: "bash"},
			}, nil
		},
		environ: func(pids []int) (map[int][]string, error) {
			all := map[int][]string{
				32: {"KONSOLE_DBUS_SERVICE=:1.0", "KONSOLE_DBUS_SESSION=/Sessions/2"},
				40: {"KONSOLE_DBUS_SERVICE=:1.0", "KONSOLE_DBUS_SESSION=/Sessions/2"},
				50: {"KONSOLE_DBUS_SERVICE=:1.0", "KONSOLE_DBUS_SESSION=/Sessions/1"},
			}
			found := map[int][]string{}
			for _, pid := range pids {
				if entries, ok := all[pid]; ok {
					found[pid] = entries
				}
			}
			return found, nil
		},
	}
}

func TestKonsole_SelectsTheTabOfTheSessionOnTheTTY(t *testing.T) {
	commands := &fakeCommands{out: konsoleReplies()}

	ok, err := testKonsole(commands).focus(context.Background(), "/dev/pts/0")

	if !ok || !errors.Is(err, errNotInFront) {
		t.Errorf("ok %v err %v", ok, err)
	}
	if last := commands.calls[len(commands.calls)-1]; last != konsoleDBus+"/Windows/2 org.kde.konsole.Window.setCurrentSession int32:2" {
		t.Errorf("last call %q", last)
	}
}

func TestKonsole_NamesTheTabAndPastesText(t *testing.T) {
	commands := &fakeCommands{out: konsoleReplies()}
	konsole := testKonsole(commands)

	if ok, err := konsole.title(context.Background(), "/dev/pts/0", "Fix, the bug"); !ok || err != nil {
		t.Errorf("title: ok %v err %v", ok, err)
	}
	if ok, err := konsole.typeIn(context.Background(), "/dev/pts/0", "line one\nline two"); !ok || err != nil {
		t.Errorf("typeIn: ok %v err %v", ok, err)
	}
	for _, want := range []string{
		konsoleDBus + "/Sessions/2 org.kde.konsole.Session.setTitle int32:1 string:Fix, the bug",
		konsoleDBus + "/Sessions/2 org.kde.konsole.Session.sendText string:\x1b[200~line one\nline two\x1b[201~",
	} {
		if !slices.Contains(commands.calls, want) {
			t.Errorf("no call %q in %q", want, commands.calls)
		}
	}
}

func TestKonsole_ARefusedSendNamesTheSetting(t *testing.T) {
	refused := errors.New("dbus-send: exit status 1: Error org.freedesktop.DBus.Error.AccessDenied: Security sensitive DBus API is disabled in the settings.")
	commands := &fakeCommands{out: konsoleReplies(), errs: map[string]error{konsoleDBus + "/Sessions/2 org.kde.konsole.Session.sendText": refused}}

	_, err := testKonsole(commands).typeIn(context.Background(), "/dev/pts/0", "hello")

	if !errors.Is(err, errKonsoleCommandsOff) || !strings.Contains(err.Error(), "Enable the security sensitive parts of the DBus API") {
		t.Errorf("err %v", err)
	}
}

func TestKonsole_ANewTabOpensInTheFolderAndRunsTheCommand(t *testing.T) {
	commands := &fakeCommands{out: konsoleReplies()}

	err := testKonsole(commands).newTab(context.Background(), `cd '/tmp/my app' && claude --resume x`)

	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		konsoleDBus + "/Windows/1 org.kde.konsole.Window.newSession string: string:/tmp/my app",
		konsoleDBus + "/Sessions/5 org.kde.konsole.Session.runCommand string:claude --resume x",
	} {
		if !slices.Contains(commands.calls, want) {
			t.Errorf("no call %q in %q", want, commands.calls)
		}
	}
}

func TestKonsole_ARefusedCommandLeavesTheTabAndSaysSo(t *testing.T) {
	refused := errors.New("dbus-send: exit status 1: Error org.freedesktop.DBus.Error.AccessDenied: Security sensitive DBus API is disabled in the settings.")
	commands := &fakeCommands{out: konsoleReplies(), errs: map[string]error{konsoleDBus + "/Sessions/5 org.kde.konsole.Session.runCommand": refused}}

	err := testKonsole(commands).newTab(context.Background(), `cd '/tmp/app' && claude --resume x`)

	if !errors.Is(err, errKonsoleCommandsOff) || errors.Is(err, errNoTabCommand) {
		t.Errorf("err %v; want errKonsoleCommandsOff, not a missing command", err)
	}
	reply := notOpenedInTab(tabOpening{app: "Konsole", err: err}, `cd '/tmp/app' && claude --resume x`)
	if !strings.Contains(reply.Message, "Enable the security sensitive parts of the DBus API") || reply.Command == "" {
		t.Errorf("reply %+v", reply)
	}
}

func TestKonsole_WithoutItsVariablesItCannotBeDriven(t *testing.T) {
	commands := &fakeCommands{out: konsoleReplies()}
	konsole := testKonsole(commands)
	konsole.environ = func([]int) (map[int][]string, error) { return map[int][]string{}, nil }

	ok, err := konsole.focus(context.Background(), "/dev/pts/0")

	if ok || !errors.Is(err, errNoTabCommand) || len(commands.calls) != 0 {
		t.Errorf("ok %v err %v calls %q", ok, err, commands.calls)
	}
}

func TestKeptEntries_KeepsOnlyTheNamedVariables(t *testing.T) {
	got := keptEntries([]string{"HOME=/home/dev", "KONSOLE_DBUS_SERVICE=:1.0", "KONSOLE_DBUS_SESSION=/Sessions/2", "KONSOLE_DBUS_ACTIVATION_COOKIE=secret", "CLAUDE_CODE_SESSION_ID=x", "ANTHROPIC_API_KEY=k"})
	want := []string{"KONSOLE_DBUS_SERVICE=:1.0", "KONSOLE_DBUS_SESSION=/Sessions/2", "CLAUDE_CODE_SESSION_ID=x"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestHostOf_NamesKonsole(t *testing.T) {
	table := []ProcessInfo{{PID: 28, PPID: 7, Command: "konsole"}, {PID: 32, PPID: 28, TTY: "/dev/pts/0", Command: "bash"}}
	if host, ok := hostOf(table, 32); !ok || host.App != "Konsole" || !tabApps["Konsole"] {
		t.Errorf("host %+v ok %v, tab app %v", host, ok, tabApps["Konsole"])
	}
}

func TestKonsole_ARefusalInAnotherLanguageIsStillTheSetting(t *testing.T) {
	refused := errors.New("dbus-send: exit status 1: Error org.freedesktop.DBus.Error.AccessDenied: Sicherheitsrelevante DBus-API ist in den Einstellungen deaktiviert.")
	commands := &fakeCommands{out: konsoleReplies(), errs: map[string]error{konsoleDBus + "/Sessions/2 org.kde.konsole.Session.sendText": refused}}

	if _, err := testKonsole(commands).typeIn(context.Background(), "/dev/pts/0", "hello"); !errors.Is(err, errKonsoleCommandsOff) {
		t.Errorf("err %v", err)
	}
}

func TestKonsole_SkipsAnEnvironmentItCannotRead(t *testing.T) {
	commands := &fakeCommands{out: konsoleReplies()}
	konsole := testKonsole(commands)
	read := konsole.environ
	konsole.environ = func(pids []int) (map[int][]string, error) {
		if slices.Contains(pids, 32) {
			return nil, errors.New("open /proc/32/environ: permission denied")
		}
		return read(pids)
	}

	if ok, err := konsole.focus(context.Background(), "/dev/pts/0"); !ok || !errors.Is(err, errNotInFront) {
		t.Errorf("ok %v err %v", ok, err)
	}
}

func TestKonsole_ATitleKeepsItsPercentSigns(t *testing.T) {
	commands := &fakeCommands{out: konsoleReplies()}

	if _, err := testKonsole(commands).title(context.Background(), "/dev/pts/0", "Fix 100%d"); err != nil {
		t.Fatal(err)
	}
	if last := commands.calls[len(commands.calls)-1]; strings.Contains(last, "%d") {
		t.Errorf("Konsole would read %%d as a marker: %q", last)
	}
}

func TestKonsole_LongTextGoesInPartsInsideOnePaste(t *testing.T) {
	commands := &fakeCommands{out: konsoleReplies()}
	text := strings.Repeat("é", 100_000)

	if ok, err := testKonsole(commands).typeIn(context.Background(), "/dev/pts/0", text); !ok || err != nil {
		t.Fatalf("ok %v err %v", ok, err)
	}
	var sent strings.Builder
	parts := 0
	for _, call := range commands.calls {
		if part, ok := strings.CutPrefix(call, konsoleDBus+"/Sessions/2 org.kde.konsole.Session.sendText string:"); ok {
			if len(part) > sendPart+len("\x1b[200~") {
				t.Errorf("a part of %d bytes", len(part))
			}
			sent.WriteString(part)
			parts++
		}
	}
	if parts < 2 || sent.String() != "\x1b[200~"+text+"\x1b[201~" {
		t.Errorf("%d parts, text kept %v", parts, sent.String() == "\x1b[200~"+text+"\x1b[201~")
	}
}

func TestMacLauncher_KonsoleFallsBackLikeAnAppWithoutItsCommand(t *testing.T) {
	if err := (macLauncher{}).RunInNewTab(context.Background(), "Konsole", "true"); !errors.Is(err, errNoTabCommand) {
		t.Errorf("new tab: %v", err)
	}
	if _, err := (macLauncher{}).FocusTab(context.Background(), "Konsole", "/dev/ttys001"); !errors.Is(err, errNoTabCommand) {
		t.Errorf("focus: %v", err)
	}
}

func TestNotOpened_KonsoleRefusingTheCommandSaysTheTabOpened(t *testing.T) {
	err := notOpened(tabOpening{app: "Konsole", err: errKonsoleCommandsOff}, "cd '/tmp/app' && x")
	if !strings.Contains(err.Error(), "opened a Konsole tab") || !strings.Contains(err.Error(), "run this line there: cd '/tmp/app' && x") {
		t.Errorf("err %v", err)
	}
}
