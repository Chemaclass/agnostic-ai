package conversations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// SendReply is the answer to a send request. Sent is true only when the
// text was typed into the conversation's input; nothing is ever submitted.
// Message says what happened, or why not and what to do instead.
type SendReply struct {
	Sent    bool   `json:"sent"`
	Message string `json:"message"`
}

// MaxSendBytes bounds the text one send types.
const MaxSendBytes = 256 << 10

// errAccessibilityDenied means macOS refused to let the server press
// keys, which pasting into Terminal needs.
var errAccessibilityDenied = errors.New("macOS blocked pressing keys")

// errNotInFront means the tab was found but its window did not come to
// the front, so a paste would land somewhere else.
var errNotInFront = errors.New("the tab did not come to the front")

// queuesTypedInput are the tools known to keep text typed into their
// input while the agent works. Others may drop it or read it as an answer.
var queuesTypedInput = map[Tool]bool{ToolClaude: true}

// typedInto are the tools that report when they wait for an answer and ask
// for bracketed paste. cursor-agent reports neither, so it is never typed into.
var typedInto = map[Tool]bool{ToolClaude: true, ToolCodex: true}

// bracketedPasteMarkers start and end a paste a terminal wraps for a
// program that asks for it; text holding them could end the paste early.
var bracketedPasteMarkers = strings.NewReplacer("\x1b[200~", "", "\x1b[201~", "")

// CleanSendText keeps newlines and tabs, turns carriage returns into
// newlines, drops every other control character and any bracketed paste
// marker, and trims trailing white space, so nothing typed is submitted.
func CleanSendText(text string) string {
	text = strings.ToValidUTF8(text, "")
	text = bracketedPasteMarkers.Replace(text)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	text = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' {
			return r
		}
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return -1
		}
		return r
	}, text)
	return strings.TrimRightFunc(text, unicode.IsSpace)
}

// Send types text into a conversation's input without pressing Enter.
// The conversation comes from the server's own scan, never from the
// request. It types only into a live session whose process holds its
// terminal right before the typing, in a tab app that can be told which
// tab. Anywhere else it switches to the conversation and leaves the
// paste to the person, since the clipboard still holds the text.
func Send(ctx context.Context, c Conversation, text string, processes Processes, launcher Launcher) SendReply {
	if len(text) > MaxSendBytes {
		return SendReply{Message: fmt.Sprintf("The text is %s, over the %s a send takes, so nothing was typed.", sizeOf(len(text)), sizeOf(MaxSendBytes))}
	}
	text = CleanSendText(text)
	if strings.TrimSpace(text) == "" {
		return SendReply{Message: "Nothing to send: copy some text first."}
	}
	if c.App == AppCloud || c.Tool == ToolChatGPT || c.Open.Kind == RouteLink {
		return pasteByHand(ctx, c, processes, launcher, "")
	}
	if c.Open.Kind == RouteTerminal {
		held, ok := heldNow(c, processes)
		if !ok {
			return SendReply{Message: "It is not running, so nothing was typed. Open it to resume it, then paste with Cmd+V."}
		}
		c = held
	}
	if c.Open.Kind != RouteFocus {
		return pasteByHand(ctx, c, processes, launcher, "")
	}
	if c.process == nil || !processes.Alive(c.process.PID, c.process.ProcStart) {
		return SendReply{Message: "It ended since the list was read, so nothing was typed. Open it to resume it, then paste with Cmd+V."}
	}
	placed := c.placed
	if placed == nil || placed.claudeBackground || len(placed.ttys) > 0 || !tabApps[placed.host.App] || placed.host.TTY == "" {
		return pasteByHand(ctx, c, processes, launcher, "")
	}
	if !typedInto[c.Tool] {
		return pasteByHand(ctx, c, processes, launcher, fmt.Sprintf("%s does not say when it waits for an answer, so nothing was typed.", toolLabels[c.Tool]))
	}
	if waiting(c) {
		return pasteByHand(ctx, c, processes, launcher, "It waits for your answer, and typed text could answer its question, so nothing was typed.")
	}
	if working(c) && !queuesTypedInput[c.Tool] {
		return pasteByHand(ctx, c, processes, launcher, fmt.Sprintf("%s is working and may drop typed text, so nothing was typed.", toolLabels[c.Tool]))
	}
	app, tty := placed.host.App, placed.host.TTY
	// A Codex thread the daemon holds shows in a client: that client owns the tab.
	if !holdsTerminal(processes, placed.process, tty) {
		return SendReply{Message: fmt.Sprintf("Its terminal on %s does not show it right now, so nothing was typed. Switch to it, then paste with Cmd+V.", shortTTY(tty))}
	}
	ok, err := launcher.TypeInTab(ctx, app, tty, text)
	byHand := " The text is on the clipboard: paste it with Cmd+V."
	switch {
	case errors.Is(err, errAutomationDenied):
		return SendReply{Message: allowControl("typing into", app, "then send again")}
	case errors.Is(err, errAccessibilityDenied):
		return SendReply{Message: fmt.Sprintf("macOS blocked pasting into %s. Allow agnostic-ai companion, or the terminal that runs agnostic-ai serve, in System Settings › Privacy & Security › Accessibility, then send again.", app) + byHand}
	case errors.Is(err, context.DeadlineExceeded):
		return SendReply{Message: fmt.Sprintf("macOS is waiting for you to allow control of %s. Answer its prompt, then send again.", app)}
	case errors.Is(err, errNotInFront):
		return SendReply{Message: fmt.Sprintf("The %s tab that runs it did not come to the front, so nothing was pasted.", app) + byHand}
	case err != nil:
		return SendReply{Message: fmt.Sprintf("Could not type into %s: %v.", app, err) + byHand}
	case !ok:
		return SendReply{Message: fmt.Sprintf("It runs in %s on %s, but no tab shows that terminal, so nothing was typed.", app, shortTTY(tty)) + byHand}
	}
	verb := "Typed"
	if app == "Terminal" {
		verb = "Pasted"
	}
	return SendReply{Sent: true, Message: fmt.Sprintf("%s into the %s tab that runs it. Press Enter there to send it.", verb, app)}
}

// waiting is a session that asks the person something, such as a
// permission prompt, where a typed digit or letter may pick an answer.
func waiting(c Conversation) bool {
	return c.Activity == ActivityWaiting || c.State == StateBlocked
}

func working(c Conversation) bool {
	return c.Activity == ActivityWorking || c.State == StateWorking
}

// holdsTerminal reports whether process still runs on tty in its
// foreground group, so the keys reach it and not a shell it was
// suspended from.
func holdsTerminal(processes Processes, process liveProcess, tty string) bool {
	if !processes.Alive(process.PID, process.ProcStart) {
		return false
	}
	table, err := processes.Table()
	if err != nil {
		return false
	}
	for _, info := range table {
		if info.PID == process.PID {
			return info.TTY == tty && strings.Contains(info.Stat, "+") && !strings.HasPrefix(info.Stat, "T")
		}
	}
	return false
}

// pasteByHand switches to a conversation that cannot be typed into from
// here and tells the person to paste. why, when set, says first why
// nothing was typed.
func pasteByHand(ctx context.Context, c Conversation, processes Processes, launcher Launcher, why string) SendReply {
	opened := Open(ctx, c, processes, launcher)
	message := opened.Message + " The text is on the clipboard: paste it with Cmd+V."
	if opened.Opened {
		message = "Switched to it. Paste with Cmd+V."
	}
	if why != "" {
		message = why + " " + message
	}
	return SendReply{Message: message}
}

func sizeOf(bytes int) string {
	return fmt.Sprintf("%d KB", (bytes+1023)/1024)
}
