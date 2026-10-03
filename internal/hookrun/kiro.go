package hookrun

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Source: kiro.dev/docs/hooks and kiro.dev/docs/hooks/types (rechecked
// 2026-10-03). Documented there:
//   - Working directory: "Command actions run a shell command in your
//     project root. The command receives session context as JSON on
//     STDIN."
//   - Timeout: "Timeout in seconds for command actions (default: 60).
//     `0` disables the timeout." The CLI tab of kiro.dev/docs/hooks/actions
//     gives 30 seconds for `timeout_ms`, a field of the CLI 2.x agent
//     format, which sync does not write.
//   - Matcher: "Regex pattern to filter which events fire this hook. For
//     `PreToolUse`/`PostToolUse`, matches tool name. ... Defaults to
//     always-match." A UserPromptSubmit matcher matches the "Prompt text"
//     (kiro.dev/docs/ide/whats-new-v1/hooks), and "Stop hooks do not use
//     matchers". No anchoring is stated, so hook run reads a matcher as
//     an unanchored regular expression. "Hook matchers support both
//     canonical names (`fs_read`, `fs_write`, `execute_bash`, `use_aws`)
//     and their aliases (`read`, `write`, `shell`, `aws`)."
//   - Payloads: userPromptSubmit and stop carry hook_event_name, cwd,
//     and session_id, plus prompt or assistant_response. The IDE also
//     gives the prompt as USER_PROMPT: "the user prompt can be accessed
//     via the `USER_PROMPT` environment variable".
//   - Replies: see readKiro.
//
// Not documented, so hook run assumes, refuses, or does not count them:
//   - The shell: a command is a "Shell command to run", with no shell
//     named and nothing on Windows.
//   - The tool_input of the shell and write tools: the only built-in
//     tool example is the read tool's.
//   - The payload of the other triggers: SessionStart, SessionEnd, the
//     file and task triggers, and Manual.
//   - Whether an exit other than 0 and 2 blocks; see readKiro.
const kiroDocs = "https://kiro.dev/docs/hooks"

const kiroDefaultTimeout = 60 * time.Second

// kiroEvents are the triggers whose payload the docs show.
var kiroEvents = []string{"UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"}

// kiroToolAliases pairs each canonical tool name with its alias, both
// ways, since a matcher may name either.
var kiroToolAliases = map[string]string{
	"fs_read": "read", "fs_write": "write", "execute_bash": "shell", "use_aws": "aws",
	"read": "fs_read", "write": "fs_write", "shell": "execute_bash", "aws": "use_aws",
}

// kiroSourceFilters are matchers for "all MCP tools", "all Powers
// tools", and "all built-in tools", whose tool names the docs do not
// list.
var kiroSourceFilters = []string{"@mcp", "@powers", "@builtin"}

// kiroExitConflict is why a non-zero exit other than 2 on a blocking
// event is not counted. kiro.dev/docs/hooks/actions says that on any
// other exit code "in the case of the **Pre Tool Use** hook, the tool
// invocation is blocked, and for the **Prompt Submit** hook, the user
// prompt submission is blocked". kiro.dev/docs/hooks/types gives Pre
// Tool Use "**Other**: Show STDERR warning to user, allow tool
// execution", the IDE 1.0 page says "Any other non-zero exit code is
// treated as an error, not a block", and the troubleshooting page says
// such codes "don't block execution".
const kiroExitConflict = "Kiro's docs disagree on whether a non-zero exit other than 2 blocks"

// kiroUndocumented is why hook run does not run a hook on event, "" when
// it does.
func kiroUndocumented(event string) string {
	if slices.Contains(kiroEvents, event) {
		return ""
	}
	return "Kiro documents no payload for " + event
}

// kiroMatches reads a matcher as an unanchored regular expression, and
// empty or `*` as everything. A matcher that does not compile leaves
// the target unbuilt: the docs do not say what Kiro does with one.
func kiroMatches(matcher, value string) (bool, error) {
	if matcher == "" || matcher == "*" {
		return true, nil
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return false, Unbuilt{fmt.Sprintf("Kiro does not document how it reads matcher %q, which hook run cannot compile: %v", matcher, err)}
	}
	return re.MatchString(value), nil
}

// kiroToolMatches matches a tool name or its alias.
func kiroToolMatches(matcher, tool string) (bool, error) {
	if slices.Contains(kiroSourceFilters, matcher) {
		return false, Unbuilt{fmt.Sprintf("Kiro does not list the tools matcher %q covers", matcher)}
	}
	ok, err := kiroMatches(matcher, tool)
	if ok || err != nil || kiroToolAliases[tool] == "" {
		return ok, err
	}
	return kiroMatches(matcher, kiroToolAliases[tool])
}

// buildKiro writes Kiro's payload: --prompt builds userPromptSubmit, and
// Stop needs no input. --bash and --edit are refused, since the docs
// show no tool_input for the shell and write tools.
func buildKiro(event, matcher, root string, in Input) (Payload, error) {
	doc := map[string]any{"cwd": root, "session_id": SessionID}
	if event == "Stop" {
		if in.Edit != "" || in.Bash != "" || in.Prompt != "" {
			return Payload{}, fmt.Errorf("--edit, --bash, and --prompt do not build %s", event)
		}
		doc["hook_event_name"], doc["assistant_response"] = "stop", ""
		return marshal(Payload{Fires: true, Trigger: "stop"}, doc)
	}
	if err := checkInput("kiro", event, "PreToolUse", "PostToolUse", "UserPromptSubmit", in); err != nil {
		return Payload{}, err
	}
	switch {
	case in.Bash != "":
		return Payload{}, Unbuilt{"Kiro documents no tool_input for its shell tool; pass --payload <file>"}
	case in.Edit != "":
		return Payload{}, Unbuilt{"Kiro documents no tool_input for its write tool; pass --payload <file>"}
	}
	doc["hook_event_name"], doc["prompt"] = "userPromptSubmit", in.Prompt
	fires, err := kiroMatches(matcher, in.Prompt)
	if err != nil {
		return Payload{}, err
	}
	return marshal(Payload{Fires: fires, Trigger: "prompt"}, doc)
}

// kiroRawPayload matches a --payload tool call's tool_name, and a
// prompt's text; Stop ignores the matcher.
func kiroRawPayload(event, matcher string, p Payload) (Payload, error) {
	var err error
	switch event {
	case "PreToolUse", "PostToolUse":
		p.Trigger = PayloadTool(p.Body)
		p.Fires, err = kiroToolMatches(matcher, p.Trigger)
	case "UserPromptSubmit":
		p.Fires, err = kiroMatches(matcher, PayloadPrompt(p.Body))
	}
	return p, err
}

// kiroRead is what hook run reads from one Kiro result. Uncounted, when
// set, is why the result stays out of --expect and the comparison
// whatever --include-assumed says.
type kiroRead struct {
	decision  Decision
	note      string
	uncounted string
}

// readKiro follows the exit codes on kiro.dev/docs/hooks/types: Pre Tool
// Use "**2**: Block tool execution", and exit 2 "blocks the triggering
// event (`PreToolUse`, `UserPromptSubmit`, `PreTaskExec`)" (the IDE 1.0
// page). Post Tool Use: "**Other**: Show STDERR warning to user. Tool
// already ran." Stop: "**0**: Hook succeeded. If STDOUT contains a block
// decision (see below), the agent continues instead of stopping", with
// `{"decision": "block", "reason": ...}`. Another non-zero exit on
// PreToolUse or UserPromptSubmit is not counted; see kiroExitConflict.
func readKiro(event string, r Result) kiroRead {
	blocking := event == "PreToolUse" || event == "UserPromptSubmit"
	switch {
	case r.TimedOut:
		return kiroRead{decision: Timeout}
	case r.StartErr != nil:
		return kiroRead{decision: Error}
	case r.Exit == 2 && blocking:
		return kiroRead{decision: Block}
	case r.Exit != 0 && blocking:
		return kiroRead{decision: Error, uncounted: kiroExitConflict}
	case r.Exit != 0:
		return kiroRead{decision: Error}
	case event == "Stop" && kiroStopReply(r).Decision == "block":
		return kiroRead{decision: Block, note: `replied "decision": "block": Kiro keeps the agent running; read as block`}
	}
	return kiroRead{decision: Allow}
}

type kiroStop struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

// kiroStopReply reads a Stop hook's JSON reply, empty when stdout holds
// none.
func kiroStopReply(r Result) kiroStop {
	var reply kiroStop
	out := strings.TrimSpace(r.Stdout)
	if !strings.HasPrefix(out, "{") || json.Unmarshal([]byte(out), &reply) != nil {
		return kiroStop{}
	}
	return reply
}

// KiroNote is the note a reply earns, such as a Stop block, or "" when
// it needs none.
func KiroNote(event string, r Result) string {
	return readKiro(event, r).note
}

// KiroUncounted is why a result stays out of --expect and the comparison
// even with --include-assumed, or "" when it counts.
func KiroUncounted(event string, r Result) string {
	return readKiro(event, r).uncounted
}

// kiroAddsContext reports whether Kiro adds a hook's output to the
// session: stdout at exit 0 on UserPromptSubmit ("STDOUT is added to
// agent's context"), and a Stop block's reason, which "is sent as a new
// user message to the agent".
func kiroAddsContext(event string, r Result) bool {
	if r.TimedOut || r.StartErr != nil || r.Exit != 0 {
		return false
	}
	switch event {
	case "UserPromptSubmit":
		return strings.TrimSpace(r.Stdout) != ""
	case "Stop":
		reply := kiroStopReply(r)
		return reply.Decision == "block" && reply.Reason != ""
	}
	return false
}

// kiroAssumptions runs a command action on an assumed `sh -c`, and
// leaves out what hook run cannot run as Kiro does: an agent action, a
// hook that asks the user first, and any command on Windows.
func kiroAssumptions(goos string, h Handler) ([]Assumption, string) {
	switch {
	case h.Agent:
		return nil, "Kiro runs an agent prompt, not a command"
	case h.Confirm:
		return nil, "Kiro asks the user before a hook with confirm runs"
	case goos == "windows":
		return nil, "Kiro does not document how it runs a hook command on Windows"
	case !ShellNeutral(h.Command):
		return nil, "Kiro does not document its shell; use a script path"
	}
	out := []Assumption{{Item: "shell", Value: "sh -c", Reason: "Kiro does not document the shell that runs a hook command"}}
	if h.Untimed {
		out = append(out, Assumption{Item: "timeout", Value: kiroDefaultTimeout.String(), Reason: "Kiro waits with no limit on a hook with timeout 0, and hook run does not"})
	}
	return out, ""
}

// kiroEntry is one hook in a `.kiro/hooks/<name>.json` file.
type kiroEntry struct {
	Trigger string `json:"trigger"`
	Matcher string `json:"matcher"`
	Action  struct {
		Type    string `json:"type"`
		Command string `json:"command"`
	} `json:"action"`
	Timeout *float64        `json:"timeout"`
	Enabled *bool           `json:"enabled"`
	Confirm json.RawMessage `json:"confirm"`
}

// handler is what Kiro runs for the entry. `"enabled": false` skips it.
func (e kiroEntry) handler() (Handler, bool) {
	if e.Enabled != nil && !*e.Enabled {
		return Handler{}, false
	}
	h := Handler{
		Command: e.Action.Command,
		Agent:   e.Action.Type == "agent",
		Confirm: len(e.Confirm) > 0 && string(e.Confirm) != "null",
	}
	if e.Timeout != nil {
		h.Timeout = time.Duration(*e.Timeout * float64(time.Second))
		h.Untimed = *e.Timeout == 0
	}
	return h, true
}

func kiroEntries(body []byte) ([]kiroEntry, error) {
	var doc struct {
		Hooks []kiroEntry `json:"hooks"`
	}
	err := json.Unmarshal(body, &doc)
	return doc.Hooks, err
}

// KiroHandlers reads the handlers Kiro runs out of the hook file sync
// writes for one spec: its command and agent actions.
func KiroHandlers(body []byte) ([]Handler, error) {
	if len(body) == 0 {
		return nil, nil
	}
	entries, err := kiroEntries(body)
	if err != nil {
		return nil, err
	}
	var out []Handler
	for _, e := range entries {
		if h, runs := e.handler(); runs {
			out = append(out, h)
		}
	}
	return out, nil
}

// kiroDrift names each command handler the synced Kiro hook file does
// not run as the spec says: the command, the matcher, the timeout, and
// confirm.
func kiroDrift(body []byte, event, matcher string, handlers []Handler, covers func(native, spec string) bool) ([]HandlerDrift, error) {
	entries, err := kiroEntries(body)
	if err != nil {
		return nil, err
	}
	var drift []HandlerDrift
	for _, h := range handlers {
		if h.Agent {
			continue
		}
		reason := fmt.Sprintf("has no %s command %q", event, h.Command)
		for _, e := range entries {
			n, runs := e.handler()
			if !runs || e.Trigger != event || n.Agent || n.Command != h.Command {
				continue
			}
			switch {
			case !covers(e.Matcher, matcher):
				reason = fmt.Sprintf("runs %q with matcher %q, not %q", h.Command, e.Matcher, matcher)
			case n.Timeout != h.Timeout || n.Untimed != h.Untimed:
				reason = fmt.Sprintf("runs %q with timeout %s, not %s", h.Command, kiroTimeout(n), kiroTimeout(h))
			case n.Confirm != h.Confirm:
				reason = fmt.Sprintf("runs %q with confirm %t, not %t", h.Command, n.Confirm, h.Confirm)
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

// kiroTimeout names a handler's timeout as its file sets it.
func kiroTimeout(h Handler) string {
	switch {
	case h.Untimed:
		return "0 (none)"
	case h.Timeout <= 0:
		return "default"
	}
	return h.Timeout.String()
}
