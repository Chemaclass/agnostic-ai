package conversations

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
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
		{PID: 920, PPID: 900, TTY: "/dev/ttys041", Command: "/usr/bin/login -f -l -p you /bin/zsh"},
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
