package hookrun

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// CLI V3 behavior: kiro.dev/docs/hooks/types and kiro.dev/docs/tools.
const kiroDocs = "https://kiro.dev/docs/hooks"

const kiroDefaultTimeout = 60 * time.Second

// kiroEvents are the triggers whose payload the docs show.
var kiroEvents = []string{"UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"}

var kiroToolCategories = map[string][]string{
	"read":  {"read_file", "list_directory", "file_search", "grep_search", "code", "tool_search", "introspect"},
	"write": {"fs_write", "fs_append", "str_replace", "delete_file", "code"},
	"shell": {"execute_bash", "execute_pwsh"},
	"web":   {"web_fetch", "remote_web_search"},
}

var kiroBuiltinTools = []string{
	"read_file", "list_directory", "file_search", "grep_search", "code", "tool_search", "introspect",
	"fs_write", "fs_append", "str_replace", "delete_file", "execute_bash", "execute_pwsh",
	"web_fetch", "remote_web_search", "invoke_sub_agent", "disclose_context", "knowledge", "createHook",
}

// kiroExitConflict is why a non-zero exit other than 2 on PreToolUse
// is not counted. kiro.dev/docs/hooks/actions says that on any
// other exit code "in the case of the **Pre Tool Use** hook, the tool
// invocation is blocked". kiro.dev/docs/hooks/types gives Pre
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

func kiroToolMatches(matcher, tool string) (bool, error) {
	if matcher == "" || matcher == "*" {
		return true, nil
	}
	if strings.ContainsAny(matcher, `\^$()[]|+`) {
		return kiroMatches(matcher, tool)
	}
	if slices.Contains([]string{"@powers", "spec", "subagent", "context"}, matcher) {
		return false, Unbuilt{fmt.Sprintf("Kiro does not list the tools matcher %q covers", matcher)}
	}
	values := []string{tool}
	for category, tools := range kiroToolCategories {
		if slices.Contains(tools, tool) {
			values = append(values, category)
		}
	}
	if slices.Contains(kiroBuiltinTools, tool) {
		values = append(values, "@builtin")
	}
	if strings.HasPrefix(tool, "mcp_") {
		values = append(values, "@mcp")

	}
	switch tool {
	case "execute_bash":
		values = append(values, "execute_pwsh")
	case "execute_pwsh":
		values = append(values, "execute_bash")
	}
	for _, value := range values {
		if kiroSelectorMatches(matcher, value) {
			return true, nil
		}
	}
	if strings.HasPrefix(tool, "mcp_") && strings.HasPrefix(matcher, "@") && matcher != "@builtin" && matcher != "@mcp" {
		id := strings.TrimPrefix(tool, "mcp_")
		if strings.Count(id, "_") != 1 {
			return false, Unbuilt{"Kiro's payload does not identify the MCP server/tool boundary; use an anchored regex against the internal tool ID"}
		}
		server, name, _ := strings.Cut(id, "_")
		for _, tag := range []string{"@" + server, "@" + server + "/" + name} {
			if kiroSelectorMatches(matcher, tag) {
				return true, nil
			}
		}
	}
	// Kiro also tries a selector as an unanchored regex when it compiles.
	re, err := regexp.Compile(matcher)
	return err == nil && re.MatchString(tool), nil
}

func kiroSelectorMatches(selector, value string) bool {
	pattern := regexp.QuoteMeta(selector)
	pattern = strings.ReplaceAll(pattern, `\*`, ".*")
	pattern = strings.ReplaceAll(pattern, `\?`, ".")
	return regexp.MustCompile("^(?:" + pattern + ")$").MatchString(value)
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

// kiroRawPayload applies matchers only to tool events.
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

func readKiro(event string, r Result) kiroRead {
	blocking := event == "PreToolUse"
	switch {
	case r.TimedOut:
		return kiroRead{decision: Timeout}
	case r.StartErr != nil:
		return kiroRead{decision: Error}
	case event == "UserPromptSubmit" && r.Exit != 0:
		return kiroRead{decision: Allow, note: "CLI V3 sends the prompt and adds the hook output and exit code to context; only the IDE blocks prompts"}
	case event == "Stop" && r.Exit == 1:
		return kiroRead{decision: Block, note: "exit 1: Kiro keeps the agent running; read as block"}
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

func kiroAddsContext(event string, r Result) bool {
	if r.TimedOut || r.StartErr != nil {
		return false
	}
	switch event {
	case "UserPromptSubmit":
		return r.Exit != 0 || strings.TrimSpace(r.Stdout) != ""
	case "Stop":
		reply := kiroStopReply(r)
		return (r.Exit == 1 || r.Exit == 0 && reply.Decision == "block") && reply.Reason != ""
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
