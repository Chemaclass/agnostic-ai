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

// errClipboardChanged means the clipboard Terminal would paste no longer
// holds the text the send checked.
var errClipboardChanged = errors.New("the clipboard changed")

// idleActivity is how each tool typed into reports a session ready for
// text: Codex reports nothing between turns.
var idleActivity = map[Tool]Activity{ToolClaude: ActivityIdle, ToolCodex: ""}

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
	return send(ctx, c, text, copiedText, processes, launcher)
}

// SendMarkdown sends Markdown made from copied HTML, as Send does, and
// says so. Terminal pastes from the clipboard, so there the clipboard
// holds the Markdown once the tab is found. A plain send never changes
// the clipboard.
func SendMarkdown(ctx context.Context, c Conversation, markdown string, processes Processes, launcher Launcher) SendReply {
	return send(ctx, c, markdown, markdownText, processes, launcher)
}

// SendComposed types text written in the companion into a live session.
func SendComposed(ctx context.Context, c Conversation, text string, processes Processes, launcher Launcher) SendReply {
	return send(ctx, c, text, composedText, processes, launcher)
}

type sendSource uint8

const (
	copiedText sendSource = iota
	markdownText
	composedText
)

func send(ctx context.Context, c Conversation, text string, source sendSource, processes Processes, launcher Launcher) SendReply {
	const copyTip = " Use Copy message to paste it yourself."
	composed := source == composedText
	hand := func(why string) SendReply {
		if composed {
			return SendReply{Message: firstNonEmpty(why, "This session cannot receive typed text.") + copyTip}
		}
		return pasteByHand(ctx, c, processes, launcher, why)
	}
	if len(text) > MaxSendBytes {
		return SendReply{Message: fmt.Sprintf("The text is %s, over the %s a send takes, so nothing was typed.", sizeOf(len(text)), sizeOf(MaxSendBytes))}
	}
	copied := text
	text = CleanSendText(text)
	if strings.TrimSpace(text) == "" {
		if composed {
			return SendReply{Message: "The message is empty."}
		}
		return SendReply{Message: "Nothing to send: copy some text first."}
	}
	resumeTip := " Open it to resume it, then paste with " + pasteKey() + "."
	if composed {
		resumeTip = copyTip
	}
	if c.App == AppCloud || c.Tool == ToolChatGPT || c.Open.Kind == RouteLink {
		return hand("")
	}
	if c.Open.Kind == RouteTerminal {
		held, ok := heldNow(c, processes)
		if !ok {
			return SendReply{Message: "It is not running, so nothing was typed." + resumeTip}
		}
		c = held
	}
	if c.Open.Kind != RouteFocus {
		return hand("")
	}
	if c.process == nil || !processes.Alive(c.process.PID, c.process.ProcStart) {
		return SendReply{Message: "It ended since the list was read, so nothing was typed." + resumeTip}
	}
	placed := c.placed
	if placed == nil || placed.claudeBackground || len(placed.ttys) > 0 || !tabApps[placed.host.App] || placed.host.TTY == "" {
		return hand("")
	}
	// The client was matched by its command line or first thread, and may have switched thread since.
	if placed.process.PID != c.process.PID {
		return hand(fmt.Sprintf("It shows in a %s terminal that may show another thread now, so nothing was typed.", toolLabels[c.Tool]))
	}
	if !typedInto[c.Tool] {
		return hand(toolLabels[c.Tool] + " does not say when it waits for an answer, so nothing was typed.")
	}
	if waiting(c) {
		return hand("It waits for your answer, and typed text could answer its question, so nothing was typed.")
	}
	if working(c) && !queuesTypedInput[c.Tool] {
		return hand(toolLabels[c.Tool] + " is working and may drop typed text, so nothing was typed.")
	}
	if !working(c) && c.Activity != idleActivity[c.Tool] {
		return hand("It does not say whether it is ready for text, so nothing was typed.")
	}
	app, tty := placed.host.App, placed.host.TTY
	if !holdsTerminal(processes, placed.process, tty) {
		next := " Switch to it, then paste with " + pasteKey() + "."
		if composed {
			next = copyTip
		}
		return SendReply{Message: fmt.Sprintf("Its terminal on %s does not show it right now, so nothing was typed.", shortTTY(tty)) + next}
	}
	setsClipboard := source != copiedText && app == "Terminal"
	if app == "Terminal" && source == copiedText && copied != text {
		return hand("The copied text ends in white space or holds characters a send drops, and Terminal would paste them, so nothing was pasted.")
	}
	ok, err := launcher.TypeInTab(ctx, app, tty, text, setsClipboard)
	byHand := " The text is on the clipboard: paste it with " + pasteKey() + "."
	if composed {
		byHand = copyTip
	}
	pasteSet, maybeSet, unknownSet := byHand, "", byHand
	if setsClipboard {
		label := "the Markdown"
		if composed {
			label = "the message"
		}
		pasteSet = " The clipboard now holds " + label + ": paste it with " + pasteKey() + "."
		maybeSet = " The clipboard may now hold " + label + "."
		unknownSet = " The clipboard may now hold " + label + ": paste it with " + pasteKey() + "."
		if composed {
			unknownSet = maybeSet + copyTip
		}
	}
	switch {
	case errors.Is(err, errAutomationDenied):
		return SendReply{Message: allowControl("typing into", app, "then send again") + maybeSet}
	case errors.Is(err, errAccessibilityDenied):
		return SendReply{Message: fmt.Sprintf("macOS blocked pasting into %s. Allow agnostic-ai companion, or the terminal that runs agnostic-ai serve, in System Settings › Privacy & Security › Accessibility, then send again.", app) + pasteSet}
	case errors.Is(err, context.DeadlineExceeded):
		return SendReply{Message: fmt.Sprintf("macOS is waiting for you to allow control of %s. Answer its prompt, then send again.", app) + maybeSet}
	case errors.Is(err, errClipboardChanged):
		return SendReply{Message: "The copied text changed, so nothing was pasted. Check the new text, then send again."}
	case errors.Is(err, errNotInFront):
		return SendReply{Message: fmt.Sprintf("The %s tab that runs it did not come to the front, so nothing was pasted.", app) + byHand}
	case err != nil:
		return SendReply{Message: fmt.Sprintf("Could not type into %s: %v.", app, err) + unknownSet}
	case !ok:
		return SendReply{Message: fmt.Sprintf("It runs in %s on %s, but no tab shows that terminal, so nothing was typed.", app, shortTTY(tty)) + byHand}
	}
	verb := "Typed"
	if app == "Terminal" {
		verb = "Pasted"
	}
	what, clipboard := "", ""
	if source == markdownText {
		what = " the Markdown"
		if app == "Terminal" {
			clipboard = " The clipboard now holds the Markdown."
		}
	}
	if composed && setsClipboard {
		clipboard = " The clipboard now holds the message."
	}
	return SendReply{Sent: true, Message: fmt.Sprintf("%s%s into the %s tab that runs it.%s Press Enter there to send it.", verb, what, app, clipboard)}
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
	message := opened.Message + " The text is on the clipboard: paste it with " + pasteKey() + "."
	if opened.Opened {
		message = "Switched to it. Paste with " + pasteKey() + "."
	}
	if why != "" {
		message = why + " " + message
	}
	return SendReply{Message: message}
}

func sizeOf(bytes int) string {
	return fmt.Sprintf("%d KB", (bytes+1023)/1024)
}

// pasteKey is the shortcut that pastes in a terminal on this system.
func pasteKey() string { return pasteKeyOn(shellSystem) }

func pasteKeyOn(goos string) string {
	switch goos {
	case "darwin":
		return "Cmd+V"
	case "windows":
		return "Ctrl+V"
	default:
		return "Ctrl+Shift+V"
	}
}
