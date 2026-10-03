package hookrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Source: cursor.com/docs/hooks (rechecked 2026-10-03). Project hooks run
// from the project root; exit 2 blocks; another failure fails open unless
// failClosed is set; a permission hook blocks on invalid JSON at exit 0.
// The shell that runs a command string and the default timeout ("platform
// default") are not documented, so hook run assumes them.

// ContractDocs is the page a target's assumptions cite, "" for a target
// hook run assumes nothing for.
func ContractDocs(target string) string {
	if target == "cursor" {
		return "https://cursor.com/docs/hooks"
	}
	return ""
}

// cursorAssumedTimeout is the default hook run uses when a Cursor hook
// sets no timeout: 30 seconds, the shortest default among the targets
// that document one.
const cursorAssumedTimeout = 30 * time.Second

// cursorPermissionEvents return a permission decision; invalid JSON at
// exit 0 blocks them.
var cursorPermissionEvents = []string{"beforeShellExecution", "beforeMCPExecution", "beforeReadFile", "beforeTabFileRead", "subagentStart", "preToolUse"}

// cursorMatchValue is what a Cursor matcher is tested against, for the
// events hook run builds: the command for the shell events, the tool
// type for the tool events, and a fixed name for the rest.
func cursorMatchValue(event, command string) string {
	switch event {
	case "beforeShellExecution", "afterShellExecution":
		return command
	case "preToolUse", "postToolUse", "postToolUseFailure":
		return "Shell"
	case "afterFileEdit":
		return "Write"
	case "beforeSubmitPrompt":
		return "UserPromptSubmit"
	}
	return ""
}

// cursorMatches treats the matcher as an unanchored regular expression,
// and empty or `*` as everything.
func cursorMatches(matcher, value string) (bool, error) {
	if matcher == "" || matcher == "*" {
		return true, nil
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return false, errors.Join(fmt.Errorf("matcher %q", matcher), err)
	}
	return re.MatchString(value), nil
}

// buildCursor writes Cursor's payload: the common fields every hook
// gets, plus the event's own. The Write tool's input is undocumented, so
// --edit builds only afterFileEdit.
func buildCursor(event, matcher, root string, in Input) (Payload, error) {
	doc := map[string]any{
		"conversation_id": SessionID, "generation_id": turnID, "model": "", "hook_event_name": event,
		"cursor_version": "", "workspace_roots": []string{root}, "user_email": nil, "transcript_path": nil,
	}
	p := Payload{}
	switch event {
	case "beforeShellExecution", "afterShellExecution":
		if in.Bash == "" {
			return Payload{}, fmt.Errorf("%s needs --bash <command> or --payload <file>", event)
		}
		doc["command"], doc["sandbox"] = in.Bash, false
		if event == "beforeShellExecution" {
			doc["cwd"] = root
		} else {
			doc["output"], doc["duration"] = "", 0
		}
		p.Trigger = in.Bash
	case "preToolUse", "postToolUse":
		if in.Edit != "" {
			return Payload{}, errors.New("--edit: Cursor documents no tool_input for its Write tool; pass --payload <file>")
		}
		if in.Bash == "" {
			return Payload{}, fmt.Errorf("%s needs --bash <command> or --payload <file>", event)
		}
		doc["tool_name"], doc["tool_use_id"], doc["cwd"] = "Shell", toolUseID, root
		doc["tool_input"] = map[string]any{"command": in.Bash, "working_directory": root}
		if event == "postToolUse" {
			doc["tool_output"], doc["duration"] = "", 0
		}
		p.Trigger = "Shell"
	case "afterFileEdit":
		if in.Edit == "" {
			return Payload{}, fmt.Errorf("%s needs --edit <path> or --payload <file>", event)
		}
		doc["file_path"] = absPath(root, in.Edit)
		doc["edits"] = []any{map[string]any{"old_string": "", "new_string": ""}}
		p.Trigger = "Write"
	case "beforeSubmitPrompt":
		if in.Prompt == "" {
			return Payload{}, fmt.Errorf("%s needs --prompt <text> or --payload <file>", event)
		}
		doc["prompt"], doc["attachments"] = in.Prompt, []any{}
		p.Trigger = "prompt"
	case "sessionStart":
		doc["session_id"], doc["is_background_agent"], doc["composer_mode"] = SessionID, false, "agent"
		p.Trigger = "session"
	default:
		return Payload{}, fmt.Errorf("hook run builds no cursor %s payload; pass --payload <file>", event)
	}
	if event != "sessionStart" {
		fires, err := cursorMatches(matcher, cursorMatchValue(event, in.Bash))
		if err != nil {
			return Payload{}, err
		}
		p.Fires = fires
	} else {
		p.Fires = true
	}
	return marshal(p, doc)
}

// decideCursor follows the exit code rules: exit 2 blocks, a crash,
// timeout, or other exit fails open unless failClosed is set, and at
// exit 0 a permission hook blocks on `permission: "deny"` or on output
// that is not a JSON reply. `ask` blocks too, except on preToolUse,
// which does not enforce it. beforeSubmitPrompt blocks on
// `continue: false`.
func decideCursor(event string, h Handler, r Result) Decision {
	permission := slices.Contains(cursorPermissionEvents, event)
	canBlock := permission || event == "beforeSubmitPrompt"
	failed := Error
	if h.FailClosed && canBlock {
		failed = Block
	}
	switch {
	case r.TimedOut && failed == Block:
		return Block
	case r.TimedOut:
		return Timeout
	case r.StartErr != nil:
		return failed
	case r.Exit == 2 && canBlock:
		return Block
	case r.Exit != 0:
		return failed
	}
	var reply struct {
		Permission string `json:"permission"`
		Continue   *bool  `json:"continue"`
	}
	out := strings.TrimSpace(r.Stdout)
	valid := strings.HasPrefix(out, "{") && json.Unmarshal([]byte(out), &reply) == nil
	switch {
	case permission && !valid:
		return Block
	case permission && reply.Permission == "deny":
		return Block
	case permission && reply.Permission == "ask" && event != "preToolUse":
		return Block
	case event == "beforeSubmitPrompt" && valid && reply.Continue != nil && !*reply.Continue:
		return Block
	}
	return Allow
}

// cursorEntry is one handler in Cursor's hooks.json.
type cursorEntry struct {
	Type       string  `json:"type"`
	Command    string  `json:"command"`
	Matcher    string  `json:"matcher"`
	Timeout    float64 `json:"timeout"`
	FailClosed bool    `json:"failClosed"`
}

type cursorDoc struct {
	Hooks map[string][]cursorEntry `json:"hooks"`
}

// CursorHandlers reads the command handlers for event out of a Cursor
// hooks file, leaving out the entry whose command is skip.
func CursorHandlers(body []byte, event, skip string) ([]Handler, error) {
	if len(body) == 0 {
		return nil, nil
	}
	var doc cursorDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var out []Handler
	for _, e := range doc.Hooks[event] {
		if (e.Type != "" && e.Type != "command") || e.Command == skip {
			continue
		}
		out = append(out, Handler{Command: e.Command, Timeout: time.Duration(e.Timeout * float64(time.Second)), FailClosed: e.FailClosed})
	}
	return out, nil
}

// cursorDrift names each handler the synced Cursor hooks file does not
// run as the spec says: the command, the matcher, the timeout, and
// failClosed.
func cursorDrift(body []byte, event, matcher string, handlers []Handler) ([]HandlerDrift, error) {
	var doc cursorDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var drift []HandlerDrift
	for _, h := range handlers {
		reason := fmt.Sprintf("has no %s command %q", event, h.Command)
		for _, e := range doc.Hooks[event] {
			if e.Command != h.Command || (e.Type != "" && e.Type != "command") {
				continue
			}
			got := time.Duration(e.Timeout * float64(time.Second))
			switch {
			case e.Matcher != matcher:
				reason = fmt.Sprintf("runs %q with matcher %q, not %q", h.Command, e.Matcher, matcher)
			case got != h.Timeout:
				reason = fmt.Sprintf("runs %q with timeout %s, not %s", h.Command, got, h.Timeout)
			case e.FailClosed != h.FailClosed:
				reason = fmt.Sprintf("runs %q with failClosed %t, not %t", h.Command, e.FailClosed, h.FailClosed)
			default:
				reason = ""
			}
			if reason == "" {
				break
			}
		}
		if reason != "" {
			drift = append(drift, HandlerDrift{Handler: h, Reason: reason})
		}
	}
	return drift, nil
}

// Assumption is a contract item hook run fills in because the target's
// docs leave it out.
type Assumption struct {
	Item   string `json:"item"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

// shellSyntax is every character a POSIX shell, cmd.exe, or PowerShell
// may read differently from plain words.
const shellSyntax = "|&;<>()$`\\\"'*?[]#~=%{}!\n"

// ShellNeutral reports whether command means the same to any POSIX
// shell: plain words, such as a script path and its arguments.
func ShellNeutral(command string) bool {
	return strings.TrimSpace(command) != "" && !strings.ContainsAny(command, shellSyntax)
}

// Assumptions returns what hook run assumes to run h as target does on
// goos, and why it cannot run h at all when no safe assumption exists.
// Targets whose contract is documented return neither.
func Assumptions(target, goos string, h Handler) ([]Assumption, string) {
	if target != "cursor" {
		return nil, ""
	}
	if goos == "windows" {
		return nil, "Cursor does not document how it runs a hook command on Windows"
	}
	if !ShellNeutral(h.Command) {
		return nil, "Cursor does not document its shell; use a script path"
	}
	out := []Assumption{{Item: "shell", Value: "sh -c", Reason: "Cursor does not document the shell that runs a hook command"}}
	if h.Timeout <= 0 {
		out = append(out, Assumption{Item: "timeout", Value: cursorAssumedTimeout.String(), Reason: "Cursor documents its default timeout as \"platform default\"; set timeout in the spec"})
	}
	return out, ""
}
