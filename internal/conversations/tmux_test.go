package conversations

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// tmuxTable is a tmux server 600 with a pane on ttys012 that runs
// claude, a client on ttys004 inside Ghostty and one on ttys009 inside
// Warp.
func tmuxTable() []ProcessInfo {
	return []ProcessInfo{
		{PID: 600, PPID: 1, Command: "tmux"},
		{PID: 601, PPID: 600, TTY: "/dev/ttys012", Command: "-zsh"},
		{PID: 602, PPID: 601, TTY: "/dev/ttys012", Command: "claude"},
		{PID: 700, PPID: 1, Command: "/Applications/Ghostty.app/Contents/MacOS/ghostty"},
		{PID: 701, PPID: 700, TTY: "/dev/ttys004", Command: "/usr/bin/login"},
		{PID: 702, PPID: 701, TTY: "/dev/ttys004", Command: "-zsh"},
		{PID: 703, PPID: 702, TTY: "/dev/ttys004", Command: "tmux"},
		{PID: 800, PPID: 1, Command: "/Applications/Warp.app/Contents/MacOS/stable"},
		{PID: 801, PPID: 800, TTY: "/dev/ttys009", Command: "-zsh"},
		{PID: 802, PPID: 801, TTY: "/dev/ttys009", Command: "/opt/homebrew/bin/tmux"},
	}
}

func TestHostOf_AShellUnderATmuxServer(t *testing.T) {
	host, ok := hostOf(tmuxTable(), 602)

	if !ok || host != (terminalHost{App: "tmux", TTY: "/dev/ttys012", tmuxServer: 600}) {
		t.Errorf("host %+v, ok %v", host, ok)
	}
	if client, _ := hostOf(tmuxTable(), 703); client != (terminalHost{App: "Ghostty", TTY: "/dev/ttys004"}) {
		t.Errorf("a tmux client in Ghostty: %+v", client)
	}
}

func TestOpen_SelectsTheTmuxPaneThenTheTabThatShowsIt(t *testing.T) {
	inTmux := Conversation{Tool: ToolClaude, Folder: "/tmp/app", Open: Route{Kind: RouteFocus, Command: "claude --resume x"}, process: &liveProcess{PID: 602}, placed: &placement{process: liveProcess{PID: 602}, host: terminalHost{App: "tmux", TTY: "/dev/ttys012", tmuxServer: 600}}}
	processes := fakeProcesses{alive: map[int]bool{602: true}, table: tmuxTable()}
	today := OpenReply{Message: "It runs on ttys012 (process 602) in an app this version cannot switch to. Switch to it there. To resume it elsewhere, end that process first, then run this command.", Command: "claude --resume x"}
	for name, tc := range map[string]struct {
		launcher fakeLauncher
		reply    OpenReply
		calls    string
	}{
		"attached in Ghostty": {
			launcher: fakeLauncher{tmuxClient: 703, focusOK: true},
			reply:    OpenReply{Opened: true, Message: "Switched to the Ghostty tab that runs it."},
			calls:    "tmux 600 /dev/ttys012; focus Ghostty /dev/ttys004",
		},
		"attached in an app without tabs": {
			launcher: fakeLauncher{tmuxClient: 802},
			reply:    OpenReply{Message: "Brought Warp to the front, but its tab cannot be selected from outside. It runs on ttys009 (process 602)."},
			calls:    "tmux 600 /dev/ttys012; activate Warp",
		},
		"detached": {
			reply: today,
			calls: "tmux 600 /dev/ttys012",
		},
		"tmux not reachable": {
			launcher: fakeLauncher{tmuxErr: errNoTabCommand},
			reply:    today,
			calls:    "tmux 600 /dev/ttys012",
		},
	} {
		t.Run(name, func(t *testing.T) {
			launcher := tc.launcher
			reply := Open(context.Background(), inTmux, processes, &launcher)
			if reply != tc.reply {
				t.Errorf("reply = %+v, want %+v", reply, tc.reply)
			}
			if actual := strings.Join(launcher.calls, "; "); actual != tc.calls {
				t.Errorf("calls = %q, want %q", actual, tc.calls)
			}
		})
	}
}

func testTmux(commands *fakeCommands) tmuxServers {
	return tmuxServers{run: commands.run, path: func() (string, error) { return "tmux", nil }, sockets: func() []string { return []string{"/tmp/tmux-501/default", "/tmp/tmux-501/work"} }}
}

func TestTmux_SelectsThePaneInTheSessionOfTheLatestClient(t *testing.T) {
	panes := "/tmp/tmux-501/work list-panes -a -F #{pid}\t#{pane_tty}\t#{pane_id}\t#{session_id}\t#{window_id}"
	clients := "/tmp/tmux-501/work list-clients -F #{client_pid}\t#{session_id}\t#{client_activity}"
	commands := &fakeCommands{
		out: map[string]string{
			"tmux -S " + panes:   "600\t/dev/ttys011\t%0\t$0\t@0\n600\t/dev/ttys012\t%3\t$1\t@2\n600\t/dev/ttys012\t%3\t$4\t@2\n",
			"tmux -S " + clients: "703\t$1\t1795288600\n802\t$4\t1795288623\n900\t$0\t1795288699\n",
		},
		errs: map[string]error{"tmux -S /tmp/tmux-501/default": errors.New("no server running")},
	}

	client, err := testTmux(commands).show(context.Background(), 600, "/dev/ttys012")

	want := []string{
		"tmux -S /tmp/tmux-501/default list-panes -a -F #{pid}\t#{pane_tty}\t#{pane_id}\t#{session_id}\t#{window_id}",
		"tmux -S " + panes,
		"tmux -S " + clients,
		"tmux -S /tmp/tmux-501/work select-window -t $4:@2",
		"tmux -S /tmp/tmux-501/work select-pane -t %3",
	}
	if client != 802 || err != nil || strings.Join(commands.calls, "\n") != strings.Join(want, "\n") {
		t.Errorf("client %d err %v calls:\n%s", client, err, strings.Join(commands.calls, "\n"))
	}
}

func TestTmux_ADetachedSessionSelectsNothing(t *testing.T) {
	commands := &fakeCommands{out: map[string]string{
		"tmux -S /tmp/tmux-501/default list-panes":   "600\t/dev/ttys012\t%3\t$1\t@2\n",
		"tmux -S /tmp/tmux-501/default list-clients": "900\t$0\t1795288699\n",
	}}

	client, err := testTmux(commands).show(context.Background(), 600, "/dev/ttys012")

	if client != 0 || err != nil || len(commands.calls) != 2 {
		t.Errorf("client %d err %v calls %q", client, err, commands.calls)
	}
}

func TestTmux_AnotherServersPaneOnTheSameTTYIsNotIt(t *testing.T) {
	commands := &fakeCommands{out: map[string]string{
		"tmux -S /tmp/tmux-501/default list-panes": "555\t/dev/ttys012\t%3\t$1\t@2\n",
	}}

	client, err := testTmux(commands).show(context.Background(), 600, "/dev/ttys012")

	if client != 0 || err != nil {
		t.Errorf("client %d err %v calls %q", client, err, commands.calls)
	}
}

func TestAttachedViewer_SkipsAClaudeAttachInsideTmux(t *testing.T) {
	table := append(tmuxTable(), ProcessInfo{PID: 610, PPID: 601, TTY: "/dev/ttys012", Command: "claude"})
	processes := fakeProcesses{table: table, args: map[int]string{610: "claude attach e3469759"}}

	if viewer, ok := attachedViewer(table, processes, ClaudeJob{ShortID: "e3469759"}); ok {
		t.Errorf("viewer %+v", viewer)
	}
}
