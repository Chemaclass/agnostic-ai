package conversations

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

// linuxTable is the process table ps -o comm= printed in Ubuntu 24.04
// with each terminal running sleep. Linux cuts a program's name to 15
// characters, and tmux renames its server.
func linuxTable() []ProcessInfo {
	return []ProcessInfo{
		{PID: 7, PPID: 1, Command: "openbox"},
		{PID: 22, PPID: 7, Command: "kitty"},
		{PID: 23, PPID: 7, Command: "wezterm-gui"},
		{PID: 25, PPID: 7, Command: "alacritty"},
		{PID: 26, PPID: 7, Command: "xfce4-terminal"},
		{PID: 31, PPID: 1, Command: "tmux: server"},
		{PID: 34, PPID: 31, TTY: "/dev/pts/0", Command: "sleep"},
		{PID: 46, PPID: 23, TTY: "/dev/pts/2", Command: "sleep"},
		{PID: 101, PPID: 1, Command: "gnome-terminal-"},
		{PID: 119, PPID: 26, TTY: "/dev/pts/4", Command: "sleep"},
		{PID: 158, PPID: 25, TTY: "/dev/pts/5", Command: "sleep"},
		{PID: 160, PPID: 101, TTY: "/dev/pts/6", Command: "sleep"},
		{PID: 194, PPID: 22, TTY: "/dev/pts/7", Command: "sleep"},
		{PID: 300, PPID: 1, Command: "cool-retro-term"},
		{PID: 301, PPID: 300, TTY: "/dev/pts/9", Command: "sleep"},
	}
}

func TestHostOf_NamesLinuxTerminalsByTheirProgram(t *testing.T) {
	cases := map[int]terminalHost{
		194: {App: "kitty", TTY: "/dev/pts/7"},
		46:  {App: "WezTerm", TTY: "/dev/pts/2"},
		160: {App: "GNOME Terminal", TTY: "/dev/pts/6"},
		158: {App: "Alacritty", TTY: "/dev/pts/5"},
		119: {App: "Xfce Terminal", TTY: "/dev/pts/4"},
		34:  {App: "tmux", TTY: "/dev/pts/0", tmuxServer: 31},
	}
	for pid, want := range cases {
		if host, ok := hostOf(linuxTable(), pid); !ok || host != want {
			t.Errorf("pid %d: got %+v %v, want %+v", pid, host, ok, want)
		}
	}
	if host, ok := hostOf(linuxTable(), 301); ok {
		t.Errorf("an unknown terminal was named %+v", host)
	}
}

func TestIsKitty_TheLinuxProgramToo(t *testing.T) {
	for command, want := range map[string]bool{
		"/Applications/kitty.app/Contents/MacOS/kitty": true,
		"kitty -e bash":                  true,
		"/usr/bin/kitty":                 true,
		"/usr/bin/kitten @ ls":           false,
		"/Applications/kitty.app/x/bash": false,
	} {
		if got := isKitty(command); got != want {
			t.Errorf("%q: got %v", command, got)
		}
	}
}

func TestWezTerm_WithoutAnAppToRaiseSaysTheWindowMayBeBehind(t *testing.T) {
	var ran []string
	tab := weztermTab{
		run: func(_ context.Context, name string, args ...string) (string, error) {
			ran = append(ran, name+" "+args[2])
			if args[2] == "list" {
				return `[{"pane_id": 4, "tty_name": "/dev/pts/2"}]`, nil
			}
			return "", nil
		},
		path:      func() (string, error) { return "wezterm", nil },
		noRaising: true,
	}

	ok, err := tab.focus(context.Background(), "pts/2")

	if !ok || !errors.Is(err, errNotInFront) {
		t.Errorf("ok %v, err %v; want the tab selected and errNotInFront", ok, err)
	}
	if want := []string{"wezterm list", "wezterm activate-pane"}; !slices.Equal(ran, want) {
		t.Errorf("ran %q, want %q", ran, want)
	}
}

func TestSwitchToHost_ATabSelectedBehindIsNotOpened(t *testing.T) {
	launcher := &fakeLauncher{focusOK: true, err: errNotInFront}
	placed := placement{process: liveProcess{PID: 46}, host: terminalHost{App: "WezTerm", TTY: "pts/2"}}

	reply := switchToHost(context.Background(), "", placed, fakeProcesses{}, launcher, "")

	if want := (OpenReply{Message: "Selected the WezTerm tab that runs it. Its window may be behind other windows."}); reply != want {
		t.Errorf("reply %+v, want %+v", reply, want)
	}
}

func TestSwitchToHost_NamesATerminalWithNoTabSupport(t *testing.T) {
	placed := placement{process: liveProcess{PID: 158}, host: terminalHost{App: "GNOME Terminal", TTY: "pts/5"}}

	reply := switchToHost(context.Background(), "", placed, fakeProcesses{}, linuxLauncher{}, "")

	if want := "It runs in GNOME Terminal on pts/5 (process 158). Switch to it there."; reply.Message != want || reply.Opened {
		t.Errorf("reply %+v, want %q", reply, want)
	}
}

func TestLinuxDrivers_TypeAndOpenTabsWithoutRaising(t *testing.T) {
	wezList := `[{"pane_id": 4, "tty_name": "/dev/pts/2"}]`
	weztermCommands := &fakeCommands{out: map[string]string{"wezterm cli --no-auto-start list": wezList, "wezterm cli --no-auto-start spawn": "5"}}
	wezterm := testWezTerm(weztermCommands)
	wezterm.noRaising = true
	kittyCommands := &fakeCommands{out: map[string]string{"kitty @ --to unix:/tmp/kitty-900 ls": kittyLs, "kitty @ --to unix:/tmp/kitty-900 launch": "7"}}
	kitty := testKitty(kittyCommands)
	kitty.noRaising = true

	for name, driver := range map[string]commandTab{"WezTerm": wezterm, "kitty": kitty} {
		tty := map[string]string{"WezTerm": "/dev/pts/2", "kitty": "/dev/ttys041"}[name]
		if ok, err := driver.typeIn(context.Background(), tty, "hello"); !ok || err != nil {
			t.Errorf("%s typeIn: ok %v err %v", name, ok, err)
		}
		if err := driver.newTab(context.Background(), "claude --resume x"); err != nil {
			t.Errorf("%s newTab: %v", name, err)
		}
	}
	for _, calls := range [][]string{weztermCommands.calls, kittyCommands.calls} {
		for _, call := range calls {
			if strings.HasPrefix(call, "open ") {
				t.Errorf("raised the window on Linux: %q", call)
			}
		}
		if !slices.ContainsFunc(calls, func(call string) bool { return strings.Contains(call, "send-text") && strings.Contains(call, "hello") }) {
			t.Errorf("text not sent: %q", calls)
		}
	}
}

func TestKitty_ANewTabGoesToAKittyThatListens(t *testing.T) {
	commands := &fakeCommands{out: map[string]string{"kitty @ --to unix:/tmp/kitty-950 launch": "7"}}
	kitty := testKitty(commands, "/tmp/kitty-950")
	kitty.table = func() ([]ProcessInfo, error) {
		return []ProcessInfo{
			{PID: 940, PPID: 1, Command: "kitty"},
			{PID: 950, PPID: 1, Command: "kitty"},
		}, nil
	}

	if err := kitty.newTab(context.Background(), "claude"); err != nil || !strings.HasPrefix(commands.calls[0], "kitty @ --to unix:/tmp/kitty-950 launch") {
		t.Errorf("err %v calls %q", err, commands.calls)
	}
}

func TestSwitchToHost_KeepsTheKittySetupStepsOnLinux(t *testing.T) {
	launcher := &fakeLauncher{focusErr: errKittyRemoteControlOff, err: errNotHere}
	placed := placement{process: liveProcess{PID: 195}, host: terminalHost{App: "kitty", TTY: "/dev/pts/7"}}

	reply := switchToHost(context.Background(), "", placed, fakeProcesses{}, launcher, "")

	if !strings.Contains(reply.Message, "allow_remote_control socket-only") || !strings.Contains(reply.Message, "pts/7") {
		t.Errorf("reply %q", reply.Message)
	}
}

func TestProgramOf_KeepsATmuxPathWithASpace(t *testing.T) {
	if got := programOf("/Users/a b/bin/tmux"); got != "tmux" {
		t.Errorf("got %q", got)
	}
}

func TestPasteKey_IsTheTerminalShortcutOfEachSystem(t *testing.T) {
	for goos, want := range map[string]string{"darwin": "Cmd+V", "linux": "Ctrl+Shift+V", "windows": "Ctrl+V"} {
		if got := pasteKeyOn(goos); got != want {
			t.Errorf("%s: got %q, want %q", goos, got, want)
		}
	}
}
