package conversations

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

// kittyLs is `kitty @ ls` cut to the fields the server reads: two OS
// windows, the second with a tab split in two windows.
const kittyLs = `[
  {"id": 1, "is_focused": false, "tabs": [{"id": 1, "title": "zsh", "windows": [{"id": 1, "pid": 901, "title": "zsh"}]}]},
  {"id": 2, "is_focused": true, "tabs": [{"id": 2, "title": "claude", "windows": [{"id": 2, "pid": 911, "title": "zsh"}, {"id": 3, "pid": 921, "title": "claude"}]}]}
]`

// kittyTable is kitty 900 with a shell on ttys041 under login, as kitty
// starts it on macOS, and another on ttys042.
func kittyTable() []ProcessInfo {
	return []ProcessInfo{
		{PID: 900, PPID: 1, Command: "/Applications/kitty.app/Contents/MacOS/kitty"},
		{PID: 920, PPID: 900, TTY: "/dev/ttys041", Command: "/usr/bin/login"},
		{PID: 921, PPID: 920, TTY: "/dev/ttys041", Command: "-zsh"},
		{PID: 922, PPID: 921, TTY: "/dev/ttys041", Command: "claude"},
		{PID: 911, PPID: 900, TTY: "/dev/ttys042", Command: "-zsh"},
	}
}

func testKitty(commands *fakeCommands, sockets ...string) kittyTab {
	return kittyTab{
		run:    commands.run,
		path:   func() (string, error) { return "kitty", nil },
		table:  func() ([]ProcessInfo, error) { return kittyTable(), nil },
		socket: func(path string) bool { return len(sockets) == 0 || strings.Contains(strings.Join(sockets, " "), path) },
	}
}

func TestMacLauncher_ReadsKittyTabNamesWithVerifiedLoginParents(t *testing.T) {
	const to = "kitty @ --to unix:/tmp/kitty-900 "
	commands := &fakeCommands{out: map[string]string{
		to + "ls": `[{"tabs":[
			{"title":"Shell reported through login","title_overridden":true,"windows":[{"pid":921}]},
			{"title":"Raw login PID","title_overridden":true,"windows":[{"pid":912}]},
			{"title":"Direct child","title_overridden":true,"windows":[{"pid":911}]},
			{"title":"Split tab","title_overridden":true,"windows":[{"pid":932},{"pid":931}]},
			{"title":"Foreign login","title_overridden":true,"windows":[{"pid":941}]},
			{"title":"Lookalike login","title_overridden":true,"windows":[{"pid":951}]},
			{"title":"Pane title","title_overridden":false,"windows":[{"pid":952}]}
		]}]`,
	}}
	driver := commandTabs["kitty"]
	kitty := driver.(kittyTab)
	kitty.run = commands.run
	kitty.path = func() (string, error) { return "kitty", nil }
	kitty.socket = func(path string) bool { return path == "/tmp/kitty-900" }
	kitty.table = func() ([]ProcessInfo, error) {
		return append(kittyTable(), []ProcessInfo{
			{PID: 901, PPID: 1, Command: "/Applications/kitty.app/Contents/MacOS/kitty"},
			{PID: 912, PPID: 900, TTY: "/dev/ttys043", Command: "/usr/bin/login"},
			{PID: 930, PPID: 900, TTY: "/dev/ttys044", Command: "/usr/bin/login"},
			{PID: 931, PPID: 930, TTY: "/dev/ttys045", Command: "-zsh"},
			{PID: 932, PPID: 900, TTY: "/dev/ttys046", Command: "-zsh"},
			{PID: 940, PPID: 901, TTY: "/dev/ttys047", Command: "/usr/bin/login"},
			{PID: 941, PPID: 940, TTY: "/dev/ttys047", Command: "-zsh"},
			{PID: 950, PPID: 900, TTY: "/dev/ttys048", Command: "/opt/bin/login"},
			{PID: 951, PPID: 950, TTY: "/dev/ttys048", Command: "-zsh"},
			{PID: 952, PPID: 900, TTY: "/dev/ttys049", Command: "-zsh"},
		}...), nil
	}
	commandTabs["kitty"] = kitty
	t.Cleanup(func() { commandTabs["kitty"] = driver })

	tabs, err := (macLauncher{}).ReadTabNames(t.Context(), "kitty")

	want := []TerminalTab{
		tab("Shell reported through login", "/dev/ttys041"),
		tab("Raw login PID", "/dev/ttys043"),
		tab("Direct child", "/dev/ttys042"),
		tab("Split tab", "/dev/ttys046", ""),
		tab("Foreign login", ""),
		tab("Lookalike login", ""),
		tab("", "/dev/ttys049"),
	}
	if err != nil || !reflect.DeepEqual(tabs, want) {
		t.Errorf("tabs = %+v, err = %v, want %+v", tabs, err, want)
	}
	if want := []string{to + "ls"}; !reflect.DeepEqual(commands.calls, want) {
		t.Errorf("calls = %q, want %q", commands.calls, want)
	}

	commands.out[to+"ls"] = kittyLs
	if tabs, err := (macLauncher{}).ReadTabNames(t.Context(), "kitty"); len(tabs) != 0 || !errors.Is(err, errNoTabCommand) {
		t.Errorf("reverse read = %+v, err = %v, want missing override metadata rejected", tabs, err)
	}
}

func TestLinuxLauncher_ReadsKittyTabNamesFromEachOwnedInstance(t *testing.T) {
	if !canReadTabNames("kitty", "linux") || !canReadTabNames("kitty", "darwin") || canReadTabNames("kitty", "windows") {
		t.Error("kitty tab name reads should be supported on Linux and macOS")
	}
	const prefix = "kitty @ --to unix:/tmp/kitty-"
	commands := &fakeCommands{
		errs: map[string]error{prefix + "900 ls": errors.New("connection refused")},
		out: map[string]string{
			prefix + "910 ls": `[{"tabs":[{"id":1,"title":"Old shell title","windows":[{"id":1,"pid":911}]}]}]`,
			prefix + "920 ls": `[{"tabs":[
			  {"id":1,"title":"Manual title","title_overridden":true,"windows":[{"id":1,"pid":921}]},
			  {"id":2,"title":"Split tab","title_overridden":true,"windows":[{"id":2,"pid":922},{"id":3,"pid":941},{"id":4,"pid":999},{"id":5,"pid":924}]}
			]}]`,
			prefix + "940 ls": `[{"tabs":[{"id":1,"title":"Shell title","title_overridden":false,"windows":[{"id":1,"pid":941}]}]}]`,
		},
	}
	kitty := testKitty(commands, "/tmp/kitty-900", "/tmp/kitty-910", "/tmp/kitty-920", "/tmp/kitty-940")
	tableReads := 0
	kitty.table = func() ([]ProcessInfo, error) {
		tableReads++
		return []ProcessInfo{
			{PID: 900, PPID: 1, Command: "/usr/bin/kitty"},
			{PID: 910, PPID: 1, Command: "/usr/bin/kitty"},
			{PID: 911, PPID: 910, TTY: "/dev/pts/1", Command: "sh"},
			{PID: 920, PPID: 1, Command: "/usr/bin/kitty"},
			{PID: 921, PPID: 920, TTY: "/dev/pts/2", Command: "sh"},
			{PID: 922, PPID: 920, TTY: "/dev/pts/3", Command: "sh"},
			{PID: 923, PPID: 920, TTY: "/dev/pts/5", Command: "/usr/bin/login"},
			{PID: 924, PPID: 923, TTY: "/dev/pts/5", Command: "sh"},
			{PID: 940, PPID: 1, Command: "/usr/bin/kitty"},
			{PID: 941, PPID: 940, TTY: "/dev/pts/4", Command: "sh"},
			{PID: 950, PPID: 1, Command: "/usr/bin/kitty"},
		}, nil
	}
	driver := linuxCommandTabs["kitty"]
	t.Cleanup(func() { linuxCommandTabs["kitty"] = driver })
	linuxCommandTabs["kitty"] = kitty

	tabs, err := (linuxLauncher{}).ReadTabNames(context.Background(), "kitty")

	want := []TerminalTab{tab("Manual title", "/dev/pts/2"), tab("Split tab", "/dev/pts/3", "", "", ""), tab("", "/dev/pts/4")}
	if err != nil || !reflect.DeepEqual(tabs, want) {
		t.Errorf("tabs = %+v, err = %v, want %+v", tabs, err, want)
	}
	if tableReads != 1 {
		t.Errorf("process table read %d times, want one", tableReads)
	}
	if want := []string{prefix + "900 ls", prefix + "910 ls", prefix + "920 ls", prefix + "940 ls"}; !reflect.DeepEqual(commands.calls, want) {
		t.Errorf("calls = %q, want %q", commands.calls, want)
	}
}

func TestLinuxKitty_AStalledSocketLeavesTimeForTheNextInstance(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), tabNameReadTimeout)
	defer cancel()
	appDeadline, _ := ctx.Deadline()
	var stalledDone <-chan struct{}
	var calls []string
	kitty := testKitty(&fakeCommands{}, "/tmp/kitty-900", "/tmp/kitty-910")
	kitty.table = func() ([]ProcessInfo, error) {
		return []ProcessInfo{
			{PID: 900, PPID: 1, Command: "/usr/bin/kitty"},
			{PID: 910, PPID: 1, Command: "/usr/bin/kitty"},
			{PID: 911, PPID: 910, TTY: "/dev/pts/1", Command: "sh"},
		}, nil
	}
	kitty.run = func(readCtx context.Context, _ string, args ...string) (string, error) {
		socket := args[2]
		calls = append(calls, socket)
		deadline, bounded := readCtx.Deadline()
		if socket == "unix:/tmp/kitty-900" {
			stalledDone = readCtx.Done()
			if !bounded || !deadline.Before(appDeadline) {
				t.Error("the stalled socket can use the whole app deadline")
				cancel()
			}
			return "", context.DeadlineExceeded
		}
		select {
		case <-stalledDone:
		default:
			t.Error("the previous socket context was not canceled before starting this read")
		}
		if !bounded || !deadline.Equal(appDeadline) {
			t.Errorf("last socket deadline = %v, want %v", deadline, appDeadline)
		}
		return `[{"tabs":[{"title":"Healthy tab","title_overridden":true,"windows":[{"id":1,"pid":911}]}]}]`, nil
	}

	tabs, err := kitty.readTabNames(ctx)

	if want := []TerminalTab{tab("Healthy tab", "/dev/pts/1")}; err != nil || !reflect.DeepEqual(tabs, want) {
		t.Errorf("tabs = %+v, err = %v, want %+v", tabs, err, want)
	}
	if want := []string{"unix:/tmp/kitty-900", "unix:/tmp/kitty-910"}; !reflect.DeepEqual(calls, want) {
		t.Errorf("sockets = %q, want %q", calls, want)
	}
}

func TestKitty_FocusesTheWindowWhoseProcessHoldsTheTTY(t *testing.T) {
	commands := &fakeCommands{out: map[string]string{"kitty @ --to unix:/tmp/kitty-900 ls": kittyLs}}

	ok, err := testKitty(commands).focus(context.Background(), "/dev/ttys041")

	want := "kitty @ --to unix:/tmp/kitty-900 ls; kitty @ --to unix:/tmp/kitty-900 focus-window --match id:3; open -a kitty"
	if !ok || err != nil || strings.Join(commands.calls, "; ") != want {
		t.Errorf("ok %v err %v calls %q", ok, err, commands.calls)
	}
}

func TestKitty_NoWindowOnTheTTY(t *testing.T) {
	commands := &fakeCommands{out: map[string]string{"kitty @ --to unix:/tmp/kitty-900 ls": kittyLs}}

	ok, err := testKitty(commands).focus(context.Background(), "/dev/ttys099")

	if ok || err != nil || len(commands.calls) != 0 {
		t.Errorf("ok %v err %v calls %q", ok, err, commands.calls)
	}
}

func TestKitty_RemoteControlOffNamesTheTwoSettings(t *testing.T) {
	commands := &fakeCommands{}
	kitty := testKitty(commands, "/tmp/other")

	_, err := kitty.focus(context.Background(), "/dev/ttys041")

	if !errors.Is(err, errNoTabCommand) || !strings.Contains(err.Error(), "allow_remote_control socket-only") || !strings.Contains(err.Error(), "listen_on unix:/tmp/kitty-{kitty_pid}") || len(commands.calls) != 0 {
		t.Errorf("err %v calls %q", err, commands.calls)
	}
	if err := kitty.newTab(context.Background(), "claude --resume x"); !errors.Is(err, errNoTabCommand) {
		t.Errorf("new tab: err %v", err)
	}
}

func TestKitty_TitlesTypesAndOpensWithBackslashesKept(t *testing.T) {
	text := `C:\new "x" --match id:1` + "\nsecond line"
	escaped := `C:\\new "x" --match id:1` + "\nsecond line"
	commands := &fakeCommands{out: map[string]string{"kitty @ --to unix:/tmp/kitty-900 ls": kittyLs, "kitty @ --to unix:/tmp/kitty-900 launch": "7\n"}}
	kitty := testKitty(commands)

	if ok, err := kitty.title(context.Background(), "/dev/ttys041", text); !ok || err != nil {
		t.Fatalf("title: ok %v err %v", ok, err)
	}
	if ok, err := kitty.typeIn(context.Background(), "/dev/ttys041", text); !ok || err != nil {
		t.Fatalf("type: ok %v err %v", ok, err)
	}
	if err := kitty.newTab(context.Background(), text); err != nil {
		t.Fatalf("new tab: %v", err)
	}

	to := "kitty @ --to unix:/tmp/kitty-900 "
	want := []string{
		to + "ls",
		to + "set-tab-title --match window_id:3 -- " + escaped,
		to + "ls",
		to + "focus-window --match id:3",
		"open -a kitty",
		to + `send-text --match id:3 -- \e[200~` + escaped + `\e[201~`,
		to + "launch --type tab",
		to + `send-text --match id:7 -- ` + escaped + `\r`,
		"open -a kitty",
	}
	if strings.Join(commands.calls, "\n---\n") != strings.Join(want, "\n---\n") {
		t.Errorf("calls:\n%q\nwant:\n%q", commands.calls, want)
	}
}

func TestKitty_TitlesATabWithUnusableReverseMetadata(t *testing.T) {
	const to = "kitty @ --to unix:/tmp/kitty-900 "
	commands := &fakeCommands{out: map[string]string{
		to + "ls": `[{"tabs":[{"title":42,"title_overridden":"wrong type","windows":[{"id":3,"pid":921}]}]}]`,
	}}
	kitty := testKitty(commands)

	ok, err := kitty.title(t.Context(), "/dev/ttys041", "Named from the list")

	if !ok || err != nil {
		t.Errorf("title: ok %v, err %v", ok, err)
	}
	if want := []string{to + "ls", to + "set-tab-title --match window_id:3 -- Named from the list"}; !reflect.DeepEqual(commands.calls, want) {
		t.Errorf("calls = %q, want %q", commands.calls, want)
	}
	if tabs, err := kitty.readTabNames(t.Context()); len(tabs) != 0 || !errors.Is(err, errNoTabCommand) {
		t.Errorf("reverse read = %+v, err = %v, want malformed metadata rejected", tabs, err)
	}
}

func TestKitty_ACommandThatCannotReachTheAppIsNoTabCommand(t *testing.T) {
	commands := &fakeCommands{errs: map[string]error{"kitty @ --to unix:/tmp/kitty-900 ls": errors.New("connection refused")}}

	if _, err := testKitty(commands).focus(context.Background(), "/dev/ttys041"); !errors.Is(err, errNoTabCommand) {
		t.Errorf("err = %v", err)
	}
}

func TestOwnSocket_OnlyASocketThisUserOwns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("kitty is driven on macOS")
	}
	dir, err := os.MkdirTemp("", "sock") //nolint:usetesting // a unix socket path must stay under 104 bytes on macOS
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	socket := filepath.Join(dir, "kitty-1")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Skipf("unix sockets: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	file := filepath.Join(dir, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if !ownSocket(socket) || ownSocket(file) || ownSocket(filepath.Join(dir, "missing")) {
		t.Errorf("socket %v, file %v", ownSocket(socket), ownSocket(file))
	}
}

func TestKitty_TypesALargeTextInPartsThatFitOneArgument(t *testing.T) {
	text := strings.Repeat("aé\\", MaxSendBytes/4)
	commands := &fakeCommands{out: map[string]string{"kitty @ --to unix:/tmp/kitty-900 ls": kittyLs}}

	if ok, err := testKitty(commands).typeIn(context.Background(), "/dev/ttys041", text); !ok || err != nil {
		t.Fatalf("ok %v err %v", ok, err)
	}

	parts := sentParts(commands.calls)
	if len(parts) < 2 {
		t.Fatalf("parts = %d, want the text cut", len(parts))
	}
	for _, part := range parts {
		if len(part) >= linuxArgMax || !utf8.ValidString(part) {
			t.Errorf("part of %d bytes, valid UTF-8 %v", len(part), utf8.ValidString(part))
		}
	}
	if got := strings.Join(parts, ""); got != `\e[200~`+kittyText(text)+`\e[201~` {
		t.Errorf("joined parts differ from the pasted text")
	}
}

func TestKitty_ClosesThePasteWhenALaterPartFails(t *testing.T) {
	to := "kitty @ --to unix:/tmp/kitty-900 "
	text := strings.Repeat("a", sendPart/2) + "second part"
	commands := &fakeCommands{
		out:  map[string]string{to + "ls": kittyLs},
		errs: map[string]error{to + "send-text --match id:3 -- second part": errors.New("broken pipe")},
	}

	if ok, err := testKitty(commands).typeIn(context.Background(), "/dev/ttys041", text); ok || err == nil {
		t.Fatalf("ok %v err %v", ok, err)
	}
	if last := commands.calls[len(commands.calls)-1]; last != to+`send-text --match id:3 -- \e[201~` {
		t.Errorf("last call = %q, want the paste closed", last)
	}
}

func TestKitty_ClosesThePasteWhenTheFirstOfSeveralPartsFails(t *testing.T) {
	to := "kitty @ --to unix:/tmp/kitty-900 "
	text := strings.Repeat("a", sendPart/2) + "second part"
	commands := &fakeCommands{
		out:  map[string]string{to + "ls": kittyLs},
		errs: map[string]error{to + `send-text --match id:3 -- \e[200~`: errors.New("broken pipe")},
	}

	if ok, err := testKitty(commands).typeIn(context.Background(), "/dev/ttys041", text); ok || err == nil {
		t.Fatalf("ok %v err %v", ok, err)
	}
	if last := commands.calls[len(commands.calls)-1]; last != to+`send-text --match id:3 -- \e[201~` {
		t.Errorf("last call = %q, want the paste closed", last)
	}
}
