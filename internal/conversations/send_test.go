package conversations

import (
	"context"
	"strings"
	"testing"
)

func TestCleanSendText_KeepsLinesAndTabsOnly(t *testing.T) {
	for in, want := range map[string]string{
		"one\ntwo\tthree":               "one\ntwo\tthree",
		"one\r\ntwo\rthree":             "one\ntwo\nthree",
		"ls\x1b[201~; rm -rf ~\x1b[A":   "ls; rm -rf ~[A",
		"start\x1b[200~middle":          "startmiddle",
		"bell\x07 and csi \U0000009b2J": "bell and csi 2J",
		"trailing lines\n\n\n":          "trailing lines",
		"trailing spaces \t\n ":         "trailing spaces",
		"bad \xff utf-8":                "bad  utf-8",
		"\x1b\x07\n":                    "",
	} {
		if actual := CleanSendText(in); actual != want {
			t.Errorf("CleanSendText(%q) = %q, want %q", in, actual, want)
		}
	}
}

func TestSend_TypesOnlyIntoALiveSessionInATabApp(t *testing.T) {
	ghostty := terminalHost{App: "Ghostty", TTY: "/dev/ttys004"}
	at := func(tool Tool, host terminalHost) Conversation {
		return Conversation{Tool: tool, Folder: "/tmp/app", Open: Route{Kind: RouteFocus, Command: "claude --resume x"}, process: &liveProcess{PID: 42}, placed: &placement{process: liveProcess{PID: 42}, host: host}}
	}
	holding := func(stat, tty string) fakeProcesses {
		return fakeProcesses{alive: map[int]bool{42: true}, table: []ProcessInfo{{PID: 42, PPID: 41, Stat: stat, TTY: tty}}}
	}
	inFront := holding("S+", "/dev/ttys004")
	working := func(c Conversation) Conversation {
		c.State, c.Activity = StateWorking, ActivityWorking
		return c
	}
	for name, tc := range map[string]struct {
		conversation Conversation
		text         string
		processes    fakeProcesses
		launcher     fakeLauncher
		reply        SendReply
		calls        string
	}{
		"Ghostty tab": {
			conversation: at(ToolClaude, ghostty),
			text:         "one\ntwo\nthree\n",
			processes:    inFront,
			launcher:     fakeLauncher{focusOK: true},
			reply:        SendReply{Sent: true, Message: "Typed into the Ghostty tab that runs it. Press Enter there to send it."},
			calls:        "type Ghostty /dev/ttys004 one\ntwo\nthree",
		},
		"Terminal tab": {
			conversation: at(ToolClaude, terminalHost{App: "Terminal", TTY: "/dev/ttys018"}),
			text:         "hello",
			processes:    holding("S+", "/dev/ttys018"),
			launcher:     fakeLauncher{focusOK: true},
			reply:        SendReply{Sent: true, Message: "Pasted into the Terminal tab that runs it. Press Enter there to send it."},
			calls:        "type Terminal /dev/ttys018 hello",
		},
		"control characters removed before typing": {
			conversation: at(ToolClaude, ghostty),
			text:         "fix\x1b[201~\x03 it\r\n",
			processes:    inFront,
			launcher:     fakeLauncher{focusOK: true},
			reply:        SendReply{Sent: true, Message: "Typed into the Ghostty tab that runs it. Press Enter there to send it."},
			calls:        "type Ghostty /dev/ttys004 fix it",
		},
		"nothing left once cleaned": {
			conversation: at(ToolClaude, ghostty),
			text:         "\x1b[201~\x07\n\n",
			processes:    inFront,
			reply:        SendReply{Message: "Nothing to send: copy some text first."},
		},
		"over the limit": {
			conversation: at(ToolClaude, ghostty),
			text:         strings.Repeat("a", MaxSendBytes+1),
			processes:    inFront,
			reply:        SendReply{Message: "The text is 257 KB, over the 256 KB a send takes, so nothing was typed."},
		},
		"ended since the scan": {
			conversation: at(ToolClaude, ghostty),
			text:         "hello",
			processes:    fakeProcesses{},
			reply:        SendReply{Message: "It ended since the list was read, so nothing was typed. Open it to resume it, then paste with Cmd+V."},
		},
		"not running": {
			conversation: Conversation{Tool: ToolClaude, Open: Route{Kind: RouteTerminal, Command: "claude --resume x"}, lastTab: &ghostty},
			text:         "hello",
			reply:        SendReply{Message: "It is not running, so nothing was typed. Open it to resume it, then paste with Cmd+V."},
		},
		"suspended to its shell": {
			conversation: at(ToolClaude, ghostty),
			text:         "hello",
			processes:    holding("T", "/dev/ttys004"),
			reply:        SendReply{Message: "Its terminal on ttys004 does not show it right now, so nothing was typed. Switch to it, then paste with Cmd+V."},
		},
		"in the background of its terminal": {
			conversation: at(ToolClaude, ghostty),
			text:         "hello",
			processes:    holding("S", "/dev/ttys004"),
			reply:        SendReply{Message: "Its terminal on ttys004 does not show it right now, so nothing was typed. Switch to it, then paste with Cmd+V."},
		},
		"on another terminal than the scan saw": {
			conversation: at(ToolClaude, ghostty),
			text:         "hello",
			processes:    holding("S+", "/dev/ttys009"),
			reply:        SendReply{Message: "Its terminal on ttys004 does not show it right now, so nothing was typed. Switch to it, then paste with Cmd+V."},
		},
		"working Claude Code keeps typed text": {
			conversation: working(at(ToolClaude, ghostty)),
			text:         "and the tests",
			processes:    inFront,
			launcher:     fakeLauncher{focusOK: true},
			reply:        SendReply{Sent: true, Message: "Typed into the Ghostty tab that runs it. Press Enter there to send it."},
			calls:        "type Ghostty /dev/ttys004 and the tests",
		},
		"working Codex is not typed into": {
			conversation: working(at(ToolCodex, ghostty)),
			text:         "and the tests",
			processes:    inFront,
			launcher:     fakeLauncher{focusOK: true},
			reply:        SendReply{Message: "Codex is working and may drop typed text, so nothing was typed. Switched to it. Paste with Cmd+V."},
			calls:        "focus Ghostty /dev/ttys004",
		},
		"Cursor is not typed into": {
			conversation: at(ToolCursor, ghostty),
			text:         "and the tests",
			processes:    inFront,
			launcher:     fakeLauncher{focusOK: true},
			reply:        SendReply{Message: "Cursor does not say when it waits for an answer, so nothing was typed. Switched to it. Paste with Cmd+V."},
			calls:        "focus Ghostty /dev/ttys004",
		},
		"editor terminal": {
			conversation: at(ToolClaude, terminalHost{App: "Cursor", TTY: "/dev/ttys020"}),
			text:         "hello",
			processes:    holding("S+", "/dev/ttys020"),
			reply:        SendReply{Message: "Brought its Cursor window to the front, but the terminal inside it cannot be selected from outside. It runs on ttys020 (process 42). The text is on the clipboard: paste it with Cmd+V."},
			calls:        "folder Cursor /tmp/app",
		},
		"Codex daemon thread with no terminal": {
			conversation: Conversation{Tool: ToolCodex, Open: Route{Kind: RouteFocus, Command: "codex resume x"}, process: &liveProcess{PID: 42}},
			text:         "hello",
			processes:    inFront,
			reply:        SendReply{Message: "Codex keeps this thread open in the background (process 42), but no Codex terminal shows it. The text is on the clipboard: paste it with Cmd+V."},
		},
		"editor chat": {
			conversation: Conversation{Tool: ToolCursor, App: AppIDE, Open: Route{Kind: RouteLink, Label: "Open in Cursor", Link: "cursor://x"}},
			text:         "hello",
			reply:        SendReply{Message: "Switched to it. Paste with Cmd+V."},
			calls:        "link cursor://x",
		},
		"cloud session": {
			conversation: Conversation{Tool: ToolClaude, App: AppCloud, Open: Route{Kind: RouteLink, Link: "https://claude.ai/code/session_01ABCDEFGHJKMNPQRSTVWXYZ01"}},
			text:         "hello",
			reply:        SendReply{Message: "Switched to it. Paste with Cmd+V."},
			calls:        "link https://claude.ai/code/session_01ABCDEFGHJKMNPQRSTVWXYZ01",
		},
		"tab not found": {
			conversation: at(ToolClaude, ghostty),
			text:         "hello",
			processes:    inFront,
			reply:        SendReply{Message: "It runs in Ghostty on ttys004, but no tab shows that terminal, so nothing was typed. The text is on the clipboard: paste it with Cmd+V."},
			calls:        "type Ghostty /dev/ttys004 hello",
		},
		"Automation refused": {
			conversation: at(ToolClaude, ghostty),
			text:         "hello",
			processes:    inFront,
			launcher:     fakeLauncher{err: errAutomationDenied},
			reply:        SendReply{Message: "macOS blocked typing into Ghostty. Allow agnostic-ai companion, or the terminal that runs agnostic-ai serve, to control Ghostty in System Settings › Privacy & Security › Automation, then send again."},
			calls:        "type Ghostty /dev/ttys004 hello",
		},
		"Automation prompt unanswered": {
			conversation: at(ToolClaude, ghostty),
			text:         "hello",
			processes:    inFront,
			launcher:     fakeLauncher{err: context.DeadlineExceeded},
			reply:        SendReply{Message: "macOS is waiting for you to allow control of Ghostty. Answer its prompt, then send again."},
			calls:        "type Ghostty /dev/ttys004 hello",
		},
		"Accessibility refused": {
			conversation: at(ToolClaude, terminalHost{App: "Terminal", TTY: "/dev/ttys018"}),
			text:         "hello",
			processes:    holding("S+", "/dev/ttys018"),
			launcher:     fakeLauncher{err: errAccessibilityDenied},
			reply:        SendReply{Message: "macOS blocked pasting into Terminal. Allow agnostic-ai companion, or the terminal that runs agnostic-ai serve, in System Settings › Privacy & Security › Accessibility, then send again. The text is on the clipboard: paste it with Cmd+V."},
			calls:        "type Terminal /dev/ttys018 hello",
		},
		"Terminal stayed behind": {
			conversation: at(ToolClaude, terminalHost{App: "Terminal", TTY: "/dev/ttys018"}),
			text:         "hello",
			processes:    holding("S+", "/dev/ttys018"),
			launcher:     fakeLauncher{err: errNotInFront},
			reply:        SendReply{Message: "The Terminal tab that runs it did not come to the front, so nothing was pasted. The text is on the clipboard: paste it with Cmd+V."},
			calls:        "type Terminal /dev/ttys018 hello",
		},
		"waiting for an answer": {
			conversation: func() Conversation {
				c := at(ToolClaude, ghostty)
				c.State, c.Activity = StateBlocked, ActivityWaiting
				return c
			}(),
			text:      "2",
			processes: inFront,
			launcher:  fakeLauncher{focusOK: true},
			reply:     SendReply{Message: "It waits for your answer, and typed text could answer its question, so nothing was typed. Switched to it. Paste with Cmd+V."},
			calls:     "focus Ghostty /dev/ttys004",
		},
		"waiting, already seen": {
			conversation: func() Conversation {
				c := at(ToolCodex, ghostty)
				c.State, c.Activity = StateWorking, ActivityWaiting
				return c
			}(),
			text:      "y",
			processes: inFront,
			launcher:  fakeLauncher{focusOK: true},
			reply:     SendReply{Message: "It waits for your answer, and typed text could answer its question, so nothing was typed. Switched to it. Paste with Cmd+V."},
			calls:     "focus Ghostty /dev/ttys004",
		},
		"Codex thread shown by a client": {
			conversation: Conversation{Tool: ToolCodex, Open: Route{Kind: RouteFocus, Command: "codex resume x"}, process: &liveProcess{PID: 42}, placed: &placement{process: liveProcess{PID: 50}, host: ghostty}},
			text:         "hello",
			processes:    fakeProcesses{alive: map[int]bool{42: true, 50: true}, table: []ProcessInfo{{PID: 42, Stat: "Ss"}, {PID: 50, Stat: "S+", TTY: "/dev/ttys004"}}},
			launcher:     fakeLauncher{focusOK: true},
			reply:        SendReply{Sent: true, Message: "Typed into the Ghostty tab that runs it. Press Enter there to send it."},
			calls:        "type Ghostty /dev/ttys004 hello",
		},
		"Codex client gone, daemon still holds the thread": {
			conversation: Conversation{Tool: ToolCodex, Open: Route{Kind: RouteFocus, Command: "codex resume x"}, process: &liveProcess{PID: 42}, placed: &placement{process: liveProcess{PID: 50}, host: ghostty}},
			text:         "hello",
			processes:    fakeProcesses{alive: map[int]bool{42: true}, table: []ProcessInfo{{PID: 42, Stat: "Ss"}}},
			reply:        SendReply{Message: "Its terminal on ttys004 does not show it right now, so nothing was typed. Switch to it, then paste with Cmd+V."},
		},
	} {
		t.Run(name, func(t *testing.T) {
			launcher := tc.launcher
			reply := Send(context.Background(), tc.conversation, tc.text, tc.processes, &launcher)
			if reply != tc.reply {
				t.Errorf("reply = %+v, want %+v", reply, tc.reply)
			}
			if actual := strings.Join(launcher.calls, "; "); actual != tc.calls {
				t.Errorf("calls = %q, want %q", actual, tc.calls)
			}
		})
	}
}

func TestSend_NeverTypesWhereNoTabCanBeNamed(t *testing.T) {
	alive := fakeProcesses{alive: map[int]bool{42: true}, table: []ProcessInfo{{PID: 42, Stat: "S+", TTY: "/dev/ttys004"}}}
	for name, c := range map[string]Conversation{
		"Claude Code background session":    {Tool: ToolClaude, Open: Route{Kind: RouteFocus, Command: "claude attach x"}, process: &liveProcess{PID: 42}, placed: &placement{process: liveProcess{PID: 42}, host: terminalHost{TTY: "/dev/ttys004"}, claudeBackground: true}},
		"Codex thread two clients may show": {Tool: ToolCodex, Open: Route{Kind: RouteFocus, Command: "codex resume x"}, process: &liveProcess{PID: 42}, placed: &placement{ttys: []string{"/dev/ttys002", "/dev/ttys004"}}},
		"terminal app without tabs to name": {Tool: ToolClaude, Open: Route{Kind: RouteFocus}, process: &liveProcess{PID: 42}, placed: &placement{process: liveProcess{PID: 42}, host: terminalHost{App: "iTerm", TTY: "/dev/ttys004"}}},
	} {
		t.Run(name, func(t *testing.T) {
			launcher := fakeLauncher{focusOK: true}
			reply := Send(context.Background(), c, "hello", alive, &launcher)
			if reply.Sent || !strings.Contains(reply.Message, "Cmd+V") {
				t.Errorf("reply = %+v, want a paste by hand", reply)
			}
			for _, call := range launcher.calls {
				if strings.HasPrefix(call, "type ") {
					t.Errorf("typed: %q", call)
				}
			}
		})
	}
}

func TestTypeInTabArgs_PassTheTextAsAnArgument(t *testing.T) {
	text := "one\ntwo\"; do shell script \"echo hacked"
	for _, app := range []string{"Ghostty", "Terminal"} {
		args := typeInTabArgs(app, "/dev/ttys004", text)
		if args[len(args)-2] != "/dev/ttys004" || args[len(args)-1] != text {
			t.Errorf("%s: tty and text must be the last arguments, actual %q", app, args[len(args)-2:])
		}
		script := strings.Join(args[:len(args)-2], "\n")
		if strings.Contains(script, "hacked") {
			t.Errorf("%s: the text leaked into the script", app)
		}
		if strings.Contains(script, `send key "enter"`) || strings.Contains(script, "do script") {
			t.Errorf("%s: the script must never press Enter:\n%s", app, script)
		}
	}
}
