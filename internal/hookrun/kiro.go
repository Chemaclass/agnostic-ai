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
// 2026-10-10). Documented there:
//   - Working directory: "Command actions run a shell command in your
//     project root. The command receives session context as JSON on
//     STDIN."
//   - Timeout: "Timeout in seconds for command actions (default: 60).
//     `0` disables the timeout." The CLI tab of kiro.dev/docs/hooks/actions
//     gives 30 seconds for `timeout_ms`, a field of the CLI 2.x agent
//     format, which sync does not write.
//   - Matcher (CLI V3): "A matcher without regex metacharacters (`\ ^ $
//     ( ) [ ] | +`) is a selector. It matches a tool whose ID or tags
//     equal the selector; `*` and `?` act as wildcards against the ID and
//     tags." A selector is also compiled as an unanchored regex; a matcher
//     with metacharacters is only that regex, tested against the tool ID.
//     "V3 does not expand CLI 2.x tool names", so `fs_read` is not `read`.
//     MCP tools are reported as `mcp_<server>_<tool>` and selected with
//     `@<server>` or `@<server>/<tool>`. The hooks migration page lists
//     the `UserPromptSubmit` matcher as "Not evaluated", and "Stop hooks
//     do not use matchers".
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

// kiroToolTags lists the built-in tools each category tag covers, from the
// "Common category mappings" on kiro.dev/docs/tools. `spec` and `subagent`
// are absent: the docs name no tool for them.
var kiroToolTags = map[string][]string{
	"read":    {"read_file", "list_directory", "file_search", "grep_search", "code", "tool_search", "introspect"},
	"write":   {"fs_write", "fs_append", "str_replace", "delete_file", "code"},
	"shell":   {"execute_bash", "execute_pwsh", "control_bash_process", "control_pwsh_process", "get_process_output", "list_processes"},
	"web":     {"web_fetch", "remote_web_search"},
	"context": {"disclose_context", "introspect"},
}

// kiroUnlistedTags are tags Kiro documents without naming their tools.
var kiroUnlistedTags = []string{"spec", "subagent"}

// kiroUnlistedSources are source selectors whose tools the docs do not
// name: a Powers tool has no documented ID, so a built-in tool cannot be
// told from one.
var kiroUnlistedSources = []string{"@powers", "@builtin"}

// kiroRegexMeta are the characters that make a matcher a regular
// expression, not a selector.
const kiroRegexMeta = `\^$()[]|+`

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

// kiroRegex reads a matcher as an unanchored regular expression. A
// matcher that does not compile leaves the target unbuilt: the docs do not
// say what Kiro does with one.
func kiroRegex(matcher, value string) (bool, error) {
	re, err := regexp.Compile(matcher)
	if err != nil {
		return false, Unbuilt{fmt.Sprintf("Kiro does not document how it reads matcher %q, which hook run cannot compile: %v", matcher, err)}
	}
	return re.MatchString(value), nil
}

// kiroGlob matches the whole of value against a `*` and `?` wildcard
// pattern.
func kiroGlob(pattern, value string) bool {
	var re strings.Builder
	re.WriteString("^")
	for _, r := range pattern {
		switch r {
		case '*':
			re.WriteString(".*")
		case '?':
			re.WriteString(".")
		default:
			re.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	re.WriteString("$")
	ok, _ := regexp.MatchString(re.String(), value)
	return ok
}

// kiroSanitize writes a server or tool name as Kiro writes it in a tool
// ID: lowercase, every other character an underscore. Wildcards stay.
func kiroSanitize(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '*', r == '?':
			return r
		case r >= 'A' && r <= 'Z':
			return r + 'a' - 'A'
		}
		return '_'
	}, name)
}

// kiroTags are the tags of a tool: its categories, and `@mcp` for an MCP
// tool.
func kiroTags(tool string) []string {
	var tags []string
	for tag, tools := range kiroToolTags {
		if slices.Contains(tools, tool) {
			tags = append(tags, tag)
		}
	}
	if strings.HasPrefix(tool, "mcp_") {
		tags = append(tags, "@mcp")
	}
	return tags
}

// kiroSelects reports whether a selector, a matcher with no regex
// metacharacter, names the tool by ID, tag, or MCP server and tool.
func kiroSelects(selector, tool string) bool {
	if strings.HasPrefix(selector, "@") && selector != "@mcp" {
		pattern := "mcp_" + kiroSanitize(strings.ReplaceAll(selector[1:], "/", "_"))
		if !strings.Contains(selector, "/") {
			pattern += "_*"
		}
		return kiroGlob(pattern, tool)
	}
	if slices.ContainsFunc(append(kiroTags(tool), tool), func(name string) bool { return kiroGlob(selector, name) }) {
		return true
	}
	shell := []string{"execute_bash", "execute_pwsh"}
	return slices.Contains(shell, selector) && slices.Contains(shell, tool)
}

// kiroToolMatches reads a PreToolUse or PostToolUse matcher against a V3
// tool ID. A tag or source whose tools the docs do not list leaves the
// target unbuilt when nothing else matches.
func kiroToolMatches(matcher, tool string) (bool, error) {
	switch {
	case matcher == "":
		return true, nil
	case strings.ContainsAny(matcher, kiroRegexMeta):
		return kiroRegex(matcher, tool)
	case slices.Contains(kiroUnlistedSources, matcher):
		return false, Unbuilt{fmt.Sprintf("Kiro does not list the tools matcher %q covers", matcher)}
	case kiroSelects(matcher, tool):
		return true, nil
	}
	if ok, _ := kiroRegex(matcher, tool); ok {
		return true, nil
	}
	if slices.Contains(kiroUnlistedTags, matcher) {
		return false, Unbuilt{fmt.Sprintf("Kiro does not list the tools the %s tag covers", matcher)}
	}
	return false, nil
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
	return marshal(Payload{Fires: true, Trigger: "prompt"}, doc)
}

// kiroRawPayload matches a --payload tool call's tool_name; a prompt and
// Stop ignore the matcher.
func kiroRawPayload(event, matcher string, p Payload) (Payload, error) {
	var err error
	switch event {
	case "PreToolUse", "PostToolUse":
		p.Trigger = PayloadTool(p.Body)
		p.Fires, err = kiroToolMatches(matcher, p.Trigger)
	case "UserPromptSubmit":
		p.Fires = true
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
// Use "**2**: Block tool execution". Post Tool Use: "**Other**: Show
// STDERR warning to user. Tool already ran." Prompt Submit: "CLI Prompt
// Submit Hooks cannot block a prompt", and exit 2 is among the "Other"
// codes after which "the prompt is still sent"; only the IDE blocks it
// (the IDE 1.0 page: exit 2 "blocks the triggering event"). Stop: "**0**:
// Hook succeeded. If STDOUT contains a block decision (see below), the
// agent continues instead of stopping", with `{"decision": "block",
// "reason": ...}`, and "**1**: Start another agent turn". Another non-zero
// exit on PreToolUse or UserPromptSubmit is not counted; see
// kiroExitConflict.
func readKiro(event string, r Result) kiroRead {
	blocking := event == "PreToolUse" || event == "UserPromptSubmit"
	switch {
	case r.TimedOut:
		return kiroRead{decision: Timeout}
	case r.StartErr != nil:
		return kiroRead{decision: Error}
	case r.Exit == 2 && event == "PreToolUse":
		return kiroRead{decision: Block}
	case r.Exit == 2 && event == "UserPromptSubmit":
		return kiroRead{decision: Allow, note: "exit 2: Kiro CLI V3 sends the prompt anyway and adds the output to the agent's context; only the IDE blocks it"}
	case r.Exit != 0 && blocking:
		return kiroRead{decision: Error, uncounted: kiroExitConflict}
	case r.Exit == 1 && event == "Stop":
		return kiroRead{decision: Block, note: "exit 1: Kiro starts another agent turn and keeps the agent running; read as block"}
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
