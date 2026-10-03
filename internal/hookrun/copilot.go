package hookrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Sources: docs.github.com/en/copilot/reference/hooks-reference and
// docs.github.com/en/copilot/how-tos/copilot-cli/customize-copilot/use-hooks
// (rechecked 2026-10-03).
//   - Timeout: "`timeoutSec` | number | No | Timeout in seconds. Default: `30`."
//   - Exec form: "Runs the executable directly without a shell."
//   - cwd: "Working directory for the command (relative to repository root
//     or absolute)."
//   - Reply: "When the hook exits, the preserved lines are concatenated,
//     trimmed, and parsed with a single `JSON.parse` call", after "type":
//     "progress" lines are removed; "If the leftover output is empty, or
//     fails to parse as JSON, the hook is treated as having produced no
//     output".
//   - preToolUse: "exit `2`, a crash, or any other non-zero exit (other than
//     a timeout) denies the tool call, even if the hook's stdout JSON reports
//     `permissionDecision: \"allow\"`"; "Timeouts are always fail-open".
//   - permissionRequest: "exit code `2` is treated as a deny".
//   - Other events: exit 2 is "Treated as a warning by default"; another
//     non-zero exit is "Logged as a hook failure. The run continues".
//   - Bash payload: the how-to's test input is
//     `"toolName":"bash","toolArgs":"{\"command\":\"ls\"}"`, and the VS Code
//     compatible `tool_input` is "parsed from JSON string when possible".
//
// Not documented, so assumed or refused: the interpreter behind the `bash`
// and `powershell` fields, the working directory of a hook without `cwd`,
// the syntax of `env` "variable expansion", the `toolArgs` of `edit`,
// `create`, and `apply_patch`, and the `tool_name` of a VS Code compatible
// `PostToolUse` (the Claude tool name is documented on `PreToolUse` only).

const copilotDocs = "https://docs.github.com/en/copilot/reference/hooks-reference"

// copilotDefaultTimeout is Copilot's documented `timeoutSec` default.
const copilotDefaultTimeout = 30 * time.Second

// Unbuilt is a payload a target documents too little to build. The
// target is listed as not run with this reason, and the others still run.
type Unbuilt struct{ Reason string }

func (u Unbuilt) Error() string { return u.Reason }

// copilotEvents maps the VS Code compatible PascalCase names onto
// Copilot's camelCase ones, which decide alike.
var copilotEvents = map[string]string{
	"SessionStart": "sessionStart", "SessionEnd": "sessionEnd", "UserPromptSubmit": "userPromptSubmitted",
	"PreToolUse": "preToolUse", "PostToolUse": "postToolUse", "PostToolUseFailure": "postToolUseFailure",
	"Stop": "agentStop", "SubagentStop": "subagentStop", "ErrorOccurred": "errorOccurred",
	"PreCompact": "preCompact", "Notification": "notification", "PermissionRequest": "permissionRequest",
}

func copilotEvent(event string) string {
	if camel, ok := copilotEvents[event]; ok {
		return camel
	}
	return event
}

// copilotClaudeNames is the documented runtime tool to Claude tool name
// table for PascalCase PreToolUse and PermissionRequest.
var copilotClaudeNames = map[string]string{
	"bash": "Bash", "powershell": "Bash", "view": "Read", "create": "Write",
	"edit": "Edit", "str_replace_editor": "Edit", "apply_patch": "Edit",
	"grep": "Grep", "rg": "Grep", "glob": "Glob", "web_fetch": "WebFetch", "web_search": "WebSearch",
	"ask_user": "AskUserQuestion", "update_todo": "TodoWrite", "task": "Agent",
}

var copilotClaudeAlternation = regexp.MustCompile(`^[A-Za-z0-9_]+(\|[A-Za-z0-9_]+)*$`)

// copilotClaudeMatches follows the Claude-format matcher rules: `*`,
// `**`, or empty fires for every tool; a name or `|` alternation fires
// when a token equals the runtime or Claude tool name; anything else is
// a regex anchored as `^(?:PATTERN)$` on the Claude tool name.
func copilotClaudeMatches(matcher, claudeName string) (bool, error) {
	if matcher == "" || matcher == "*" || matcher == "**" {
		return true, nil
	}
	if copilotClaudeAlternation.MatchString(matcher) {
		for _, token := range strings.Split(matcher, "|") {
			if token == claudeName || copilotClaudeNames[token] == claudeName || token == "Task" && claudeName == "Agent" {
				return true, nil
			}
		}
		return false, nil
	}
	return copilotRegexMatches(matcher, claudeName)
}

// copilotRegexMatches compiles the matcher as `^(?:PATTERN)$`.
func copilotRegexMatches(matcher, value string) (bool, error) {
	if matcher == "" {
		return true, nil
	}
	re, err := regexp.Compile("^(?:" + matcher + ")$")
	if err != nil {
		return false, errors.Join(fmt.Errorf("matcher %q", matcher), err)
	}
	return re.MatchString(value), nil
}

// copilotMatches tests matcher against what event's matcher filters on
// in doc: the tool name, notification_type, trigger, or agentName. An
// event without a documented matcher fires for everything.
func copilotMatches(event, matcher string, doc map[string]any) (bool, string, error) {
	if event == "PreToolUse" || event == "PermissionRequest" {
		name, _ := doc["tool_name"].(string)
		ok, err := copilotClaudeMatches(matcher, name)
		return ok, name, err
	}
	field := map[string]string{
		"preToolUse": "toolName", "postToolUse": "toolName", "permissionRequest": "toolName",
		"notification": "notification_type", "Notification": "notification_type",
		"preCompact": "trigger", "subagentStart": "agentName",
	}[event]
	if field == "" {
		return true, "", nil
	}
	value, _ := doc[field].(string)
	ok, err := copilotRegexMatches(matcher, value)
	return ok, value, err
}

// buildCopilot writes the camelCase payload for a camelCase event and
// the VS Code compatible one for a PascalCase event.
func buildCopilot(event, matcher, root string, in Input) (Payload, error) {
	pascal := event != copilotEvent(event)
	doc := map[string]any{"cwd": root}
	if pascal {
		doc["hook_event_name"], doc["session_id"] = event, SessionID
		doc["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
	} else {
		doc["sessionId"], doc["timestamp"] = SessionID, time.Now().UnixMilli()
	}
	key := func(camel, snake string) string {
		if pascal {
			return snake
		}
		return camel
	}
	p := Payload{Fires: true}
	switch copilotEvent(event) {
	case "sessionStart":
		doc["source"] = "startup"
		p.Trigger = "session"
	case "userPromptSubmitted":
		if in.Prompt == "" {
			return Payload{}, fmt.Errorf("%s needs --prompt <text> or --payload <file>", event)
		}
		doc["prompt"] = in.Prompt
		p.Trigger = "prompt"
	case "preToolUse", "postToolUse":
		if in.Edit != "" {
			return Payload{}, Unbuilt{"Copilot documents no toolArgs for edit, create, or apply_patch; pass --payload <file>"}
		}
		if in.Bash == "" {
			return Payload{}, fmt.Errorf("%s needs --bash <command> or --payload <file>", event)
		}
		if event == "PostToolUse" {
			return Payload{}, Unbuilt{"Copilot documents the Claude tool name on PreToolUse only, not the tool_name of PostToolUse; pass --payload <file>"}
		}
		args := map[string]any{"command": in.Bash}
		if pascal {
			doc["tool_name"], doc["tool_input"] = "Bash", args
		} else {
			raw, err := json.Marshal(args)
			if err != nil {
				return Payload{}, err
			}
			doc["toolName"], doc["toolArgs"] = "bash", string(raw)
		}
		if copilotEvent(event) == "postToolUse" {
			doc[key("toolResult", "tool_result")] = map[string]any{key("resultType", "result_type"): "success", key("textResultForLlm", "text_result_for_llm"): ""}
		}
	default:
		return Payload{}, Unbuilt{fmt.Sprintf("hook run builds no copilot %s payload; pass --payload <file>", event)}
	}
	if p.Trigger == "" {
		fires, trigger, err := copilotMatches(event, matcher, doc)
		if err != nil {
			return Payload{}, err
		}
		p.Fires, p.Trigger = fires, trigger
	}
	return marshal(p, doc)
}

// copilotReply is the hook output: stdout without its progress lines,
// parsed as one JSON document. ok is false for no output.
func copilotReply(stdout string) (map[string]any, bool) {
	var kept []string
	for _, line := range strings.Split(stdout, "\n") {
		var progress struct {
			Type string `json:"type"`
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "{") && json.Unmarshal([]byte(trimmed), &progress) == nil && progress.Type == "progress" {
			continue
		}
		kept = append(kept, line)
	}
	var reply map[string]any
	out := strings.TrimSpace(strings.Join(kept, "\n"))
	if out == "" || json.Unmarshal([]byte(out), &reply) != nil || reply == nil {
		return nil, false
	}
	return reply, true
}

// decideCopilot follows the exit code table and decision controls. A
// timeout is always fail-open. preToolUse is fail-closed on any other
// failure and blocks on `permissionDecision` deny or ask (ask waits for
// the user, and cloud agent reads it as deny). permissionRequest blocks
// on exit 2 and `behavior` deny. agentStop and subagentStop block on
// `decision` block. Other events cannot block.
func decideCopilot(event string, r Result) Decision {
	event = copilotEvent(event)
	failed := r.StartErr != nil || r.Exit != 0
	switch {
	case r.TimedOut:
		return Timeout
	case event == "preToolUse" && failed:
		return Block
	case event == "permissionRequest" && r.Exit == 2:
		return Block
	case event == "postToolUseFailure" && r.Exit == 2:
		return Allow
	case failed:
		return Error
	}
	reply, _ := copilotReply(r.Stdout)
	switch event {
	case "preToolUse":
		if d := reply["permissionDecision"]; d == "deny" || d == "ask" {
			return Block
		}
	case "permissionRequest":
		if reply["behavior"] == "deny" {
			return Block
		}
	case "agentStop", "subagentStop":
		if reply["decision"] == "block" {
			return Block
		}
	}
	return Allow
}

// CopilotAsks reports whether a preToolUse hook replied ask, which
// Copilot CLI enforces by asking the user.
func CopilotAsks(event string, r Result) bool {
	reply, _ := copilotReply(r.Stdout)
	return copilotEvent(event) == "preToolUse" && !r.TimedOut && r.StartErr == nil && r.Exit == 0 && reply["permissionDecision"] == "ask"
}

// copilotAddsContext reports a non-empty additionalContext on the events
// that consume it, and postToolUseFailure's exit 2 stdout.
func copilotAddsContext(event string, r Result) bool {
	event = copilotEvent(event)
	if r.TimedOut || r.StartErr != nil {
		return false
	}
	if event == "postToolUseFailure" && r.Exit == 2 {
		return strings.TrimSpace(r.Stdout) != ""
	}
	if r.Exit != 0 {
		return false
	}
	switch event {
	case "sessionStart", "postToolUse", "postToolUseFailure", "subagentStart", "notification":
		reply, _ := copilotReply(r.Stdout)
		text, _ := reply["additionalContext"].(string)
		return strings.TrimSpace(text) != ""
	}
	return false
}

// copilotEntry is one command entry in a Copilot hooks file.
type copilotEntry struct {
	Type       string            `json:"type"`
	Matcher    string            `json:"matcher"`
	Command    string            `json:"command"`
	Bash       string            `json:"bash"`
	Exec       string            `json:"exec"`
	Args       []string          `json:"args"`
	Cwd        string            `json:"cwd"`
	Env        map[string]string `json:"env"`
	TimeoutSec *float64          `json:"timeoutSec"`
	Timeout    *float64          `json:"timeout"`
}

// handler is what Copilot runs for the entry on Unix: exec with args, or
// the `bash` field, which `command` fills when absent.
func (e copilotEntry) handler() (Handler, bool) {
	if e.Type != "" && e.Type != "command" {
		return Handler{}, false
	}
	h := Handler{Command: e.Bash, Cwd: e.Cwd, Env: e.Env, Timeout: e.timeout()}
	switch {
	case e.Exec != "":
		h.Command, h.Args, h.Exec = e.Exec, e.Args, true
	case h.Command == "":
		h.Command = e.Command
	}
	return h, h.Command != ""
}

// timeout reads timeoutSec, or its alias timeout when it is absent.
func (e copilotEntry) timeout() time.Duration {
	seconds := e.TimeoutSec
	if seconds == nil {
		seconds = e.Timeout
	}
	if seconds == nil {
		return 0
	}
	return time.Duration(*seconds * float64(time.Second))
}

type copilotDoc struct {
	Hooks map[string][]copilotEntry `json:"hooks"`
}

// CopilotHandlers reads the command handlers for event out of a Copilot
// hooks file.
func CopilotHandlers(body []byte, event string) ([]Handler, error) {
	if len(body) == 0 {
		return nil, nil
	}
	var doc copilotDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var out []Handler
	for _, e := range doc.Hooks[event] {
		if h, ok := e.handler(); ok {
			out = append(out, h)
		}
	}
	return out, nil
}

// copilotDrift names each handler the synced Copilot hooks file does not
// run as the spec says: the command or exec form, the matcher, the
// timeout, and cwd.
func copilotDrift(body []byte, event, matcher string, handlers []Handler) ([]HandlerDrift, error) {
	var doc copilotDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var drift []HandlerDrift
	for _, h := range handlers {
		reason := fmt.Sprintf("has no %s command %q", event, shownCommand(h, ""))
		for _, e := range doc.Hooks[event] {
			got, ok := e.handler()
			if !ok || got.Command != h.Command || got.Exec != h.Exec || strings.Join(got.Args, "\x00") != strings.Join(h.Args, "\x00") {
				continue
			}
			switch {
			case e.Matcher != matcher:
				reason = fmt.Sprintf("runs %q with matcher %q, not %q", h.Command, e.Matcher, matcher)
			case got.Timeout != h.Timeout:
				reason = fmt.Sprintf("runs %q with timeout %s, not %s", h.Command, got.Timeout, h.Timeout)
			case got.Cwd != h.Cwd:
				reason = fmt.Sprintf("runs %q with cwd %q, not %q", h.Command, got.Cwd, h.Cwd)
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

// copilotAssumptions: exec form runs with no shell. A `bash` or
// `command` string runs under an assumed `sh -c` when it is
// shell-neutral; on Windows, the `powershell` interpreter is unknown. A
// handler without cwd runs from the assumed project root.
func copilotAssumptions(goos string, h Handler) ([]Assumption, string) {
	for _, v := range h.Env {
		if strings.Contains(v, "$") {
			return nil, "Copilot does not document the syntax of its env variable expansion"
		}
	}
	var out []Assumption
	if !h.Exec {
		if goos == "windows" {
			return nil, "Copilot does not document the interpreter behind its powershell field"
		}
		if !ShellNeutral(h.Command) {
			return nil, "Copilot does not document its shell; use a script path"
		}
		out = append(out, Assumption{Item: "shell", Value: "sh -c", Reason: "Copilot does not document the interpreter behind its bash field"})
	}
	if h.Cwd == "" {
		out = append(out, Assumption{Item: "working directory", Value: "project root", Reason: "Copilot does not document where a hook without cwd runs"})
	}
	return out, ""
}

// CopilotDir is the directory Copilot runs h in: its cwd, relative to
// the repository root or absolute, or the assumed project root.
func CopilotDir(root string, h Handler) string {
	if h.Cwd == "" {
		return root
	}
	return absPath(root, h.Cwd)
}
