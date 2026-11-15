package hookrun

import (
	"encoding/json"
	"slices"
	"strings"
	"time"
)

// Source: docs.factory.com/harness/hooks (rechecked
// 2026-11-15, CLI v1.11.0). Documented there:
//   - Project root: "Use `"$FACTORY_PROJECT_DIR"/path/to/script.sh` for
//     project scripts". The command runs as written, with
//     FACTORY_PROJECT_DIR in its env; see expandRootVar.
//   - Timeout: "Per-command timeout in seconds. Defaults to `60`."
//   - Payload: session_id, transcript_path, cwd, permission_mode, and
//     hook_event_name on every hook; a Create call's tool_input is
//     file_path and content; an Execute call's is command (the quickstart
//     reads `.tool_input.command`).
//   - Matcher: "Empty, omitted, or `*` matches everything. Exact strings
//     match one tool or lifecycle matcher. Regex patterns are supported
//     and are case-sensitive."
//   - Replies: see decideFactory.
//
// Not documented, so hook run assumes or refuses them:
//   - The working directory: "Hooks execute from Droid's current working
//     directory, which can differ from your repository root". hook run
//     assumes the project root.
//   - The shell: a command is a "Shell command executed with JSON hook
//     input on stdin", with no shell named and nothing on Windows.
//   - The Edit and ApplyPatch input: "The exact shape of `tool_input`
//     and `tool_response` depends on the tool", and only Create is shown.
const factoryDocs = "https://docs.factory.com/harness/hooks"

const factoryDefaultTimeout = 60 * time.Second

const factoryRootVar = "FACTORY_PROJECT_DIR"

// expandRootVar replaces each project root reference the shell would
// expand, `$VARIABLE` or `${VARIABLE}`, such as `$FACTORY_PROJECT_DIR`,
// with the word root, only to check that a command written as the
// vendor guide says is a script path. The command itself runs unchanged,
// so the shell expands the reference and splits an unquoted one as the
// target's would. A reference inside single quotes or after a backslash
// is literal to the shell and stays as written; one inside other
// double-quoted text gets the quotes closed around root, which leaves
// shell syntax.
func expandRootVar(command, variable, root string) string {
	var out strings.Builder
	var quote byte
	for i := 0; i < len(command); {
		c := command[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			}
		case c == '\\' && i+1 < len(command):
			out.WriteString(command[i : i+2])
			i += 2
			continue
		case c == '\'' && quote == 0:
			quote = c
		case c == '"' && quote == 0:
			// The guides' own form, "$FACTORY_PROJECT_DIR", is the root word.
			if n := rootVarRef(command[i+1:], variable); n > 0 && i+1+n < len(command) && command[i+1+n] == '"' {
				out.WriteString(root)
				i += n + 2
				continue
			}
			quote = c
		case c == '"':
			quote = 0
		case c == '$':
			if n := rootVarRef(command[i:], variable); n > 0 {
				if quote == '"' {
					out.WriteString(`"` + root + `"`)
				} else {
					out.WriteString(root)
				}
				i += n
				continue
			}
		}
		out.WriteByte(c)
		i++
	}
	return out.String()
}

// rootVarRef is the length of the reference to variable s starts with,
// or 0 when it starts with none, such as $FACTORY_PROJECT_DIRS.
func rootVarRef(s, variable string) int {
	if braced := "${" + variable + "}"; strings.HasPrefix(s, braced) {
		return len(braced)
	}
	plain := "$" + variable
	if !strings.HasPrefix(s, plain) || len(s) > len(plain) && rootVariableByte(s[len(plain)]) {
		return 0
	}
	return len(plain)
}

func rootVariableByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// factorySources are SessionStart's documented sources: "`source`
// (`startup`, `resume`, `clear`, `compact`)".
var factorySources = []string{"startup", "resume", "clear", "compact"}

// buildFactory writes Factory's payload. --bash calls Execute; --edit
// calls Create, the one file tool whose input the docs show, and is
// refused when the matcher picks Edit or ApplyPatch instead.
func buildFactory(event, matcher, root string, in Input) (Payload, error) {
	if err := checkInput("factory", event, "PreToolUse", "PostToolUse", "UserPromptSubmit", in); err != nil {
		return Payload{}, err
	}
	doc := map[string]any{
		"session_id": SessionID, "transcript_path": "", "cwd": root,
		"permission_mode": "off", "hook_event_name": event,
	}
	p := Payload{Fires: true}
	var err error
	switch event {
	case "UserPromptSubmit":
		p.Trigger = "prompt"
		doc["prompt"], doc["has_images"] = in.Prompt, false
	case "SessionStart":
		p.Trigger, p.Fires, err = firstMatch(matches, matcher, factorySources)
		doc["source"] = p.Trigger
	default:
		var input map[string]any
		if in.Bash != "" {
			p.Trigger = "Execute"
			p.Fires, err = matches(matcher, p.Trigger)
			input = map[string]any{"command": in.Bash}
		} else {
			p.Trigger, p.Fires, err = firstMatch(matches, matcher, []string{"Create", "Edit", "ApplyPatch"})
			if err == nil && p.Fires && p.Trigger != "Create" {
				return Payload{}, Unbuilt{"Factory documents no tool_input for its Edit and ApplyPatch tools; pass --payload <file>"}
			}
			input = map[string]any{"file_path": absPath(root, in.Edit), "content": ""}
		}
		doc["tool_name"], doc["tool_input"] = p.Trigger, input
		if event == "PostToolUse" {
			doc["tool_response"] = map[string]any{}
		}
	}
	if err != nil {
		return Payload{}, err
	}
	return marshal(p, doc)
}

// factoryExitTwoErrors read exit 2 as a non-blocking error: "On
// `Notification`, `SessionStart`, and `SessionEnd`, exit `2` is a
// non-blocking error and none of the hook's JSON applies", and
// "`SubagentStop` and automatic `PreCompact` cannot be blocked". hook run
// reads PreCompact as the manual compaction, which exit 2 blocks.
var factoryExitTwoErrors = []string{"Notification", "SubagentStop", "SessionStart", "SessionEnd"}

// factoryBlockEvents are where `continue: false`, `decision: "block"`, and
// `onFailure: "block"` block (Control by event, Block when a hook fails).
// Stop is apart: `decision: "block"` keeps Droid running, while
// `continue: false` stops the turn, which is what Stop asked for.
var factoryBlockEvents = []string{"PreToolUse", "PostToolUse", "UserPromptSubmit", "PreCompact"}

// factoryDecisionEvents act on `decision: "block"` outside PreToolUse,
// whose permission decision reads it apart.
var factoryDecisionEvents = []string{"PostToolUse", "UserPromptSubmit", "Stop", "PreCompact"}

// factoryOutputFields are the keys that make stdout JSON output: "an
// object that sets at least one output field".
var factoryOutputFields = []string{"continue", "stopReason", "suppressOutput", "systemMessage", "decision", "reason", "hookSpecificOutput"}

type factoryStdout int

const (
	factoryPlain factoryStdout = iota
	factoryJSON
	factoryInvalid
)

// factoryReply is a JSON reply as Droid validates it: a field of the
// wrong type fails the decode.
type factoryReply struct {
	Continue           *bool   `json:"continue"`
	StopReason         *string `json:"stopReason"`
	SuppressOutput     *bool   `json:"suppressOutput"`
	SystemMessage      *string `json:"systemMessage"`
	Decision           *string `json:"decision"`
	Reason             *string `json:"reason"`
	HookSpecificOutput *struct {
		HookEventName            *string         `json:"hookEventName"`
		PermissionDecision       *string         `json:"permissionDecision"`
		PermissionDecisionReason *string         `json:"permissionDecisionReason"`
		AdditionalContext        *string         `json:"additionalContext"`
		UpdatedInput             json.RawMessage `json:"updatedInput"`
	} `json:"hookSpecificOutput"`
}

// readFactory sorts stdout as "How Droid reads stdout" does: text in
// braces is JSON output when it is `{}` or sets an output field, plain
// text when it is one object or several lines of objects that set none,
// and invalid otherwise, as is output with a wrong type, an unknown
// decision, or another event's hookEventName.
func readFactory(event, stdout string) (factoryReply, factoryStdout) {
	var reply factoryReply
	out := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stdout), "\ufeff"))
	if !strings.HasPrefix(out, "{") || !strings.HasSuffix(out, "}") {
		return reply, factoryPlain
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(out), &fields) != nil {
		if factoryPlainLines(out) {
			return reply, factoryPlain
		}
		return reply, factoryInvalid
	}
	if len(fields) > 0 && !factorySetsOutput(fields) {
		return reply, factoryPlain
	}
	if json.Unmarshal([]byte(out), &reply) != nil {
		return reply, factoryInvalid
	}
	if d := factoryString(reply.Decision); d != "" && d != "approve" && d != "block" {
		return reply, factoryInvalid
	}
	if hso := reply.HookSpecificOutput; hso != nil {
		switch factoryString(hso.PermissionDecision) {
		case "", "allow", "deny", "ask":
		default:
			return reply, factoryInvalid
		}
		if name := factoryString(hso.HookEventName); name != "" && name != event {
			return reply, factoryInvalid
		}
	}
	return reply, factoryJSON
}

// factoryPlainLines reports whether every nonblank line is a JSON object
// that sets no output field, such as a log line per row.
func factoryPlainLines(out string) bool {
	for line := range strings.Lines(out) {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal([]byte(line), &fields) != nil || factorySetsOutput(fields) {
			return false
		}
	}
	return true
}

func factorySetsOutput(fields map[string]json.RawMessage) bool {
	for _, key := range factoryOutputFields {
		if raw, ok := fields[key]; ok && string(raw) != "null" {
			return true
		}
	}
	return false
}

func factoryString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// factoryPermission is a PreToolUse reply's decision: permissionDecision,
// else the older top-level decision, where approve is allow and block is
// deny. "When both are set, `permissionDecision` wins."
func factoryPermission(reply factoryReply) string {
	if hso := reply.HookSpecificOutput; hso != nil {
		if d := factoryString(hso.PermissionDecision); d != "" {
			return d
		}
	}
	switch factoryString(reply.Decision) {
	case "approve":
		return "allow"
	case "block":
		return "deny"
	}
	return ""
}

// decideFactory follows the hooks guide (Hook output, Control by event,
// Block when a hook fails). Exit 2 blocks, except where the event cannot
// block. On any other exit, a JSON reply applies as on exit 0, and plain
// or empty stdout on a non-zero exit is a non-blocking error, as are a
// timeout and JSON that cannot be applied. With `onFailure: "block"`, on
// the factoryBlockEvents, each of those failures blocks, and so does any
// stdout on a non-zero exit. A JSON reply blocks with:
//   - PreToolUse: a deny, from permissionDecision or the older top-level
//     decision. "ask" does not block: "normal permissions apply".
//   - `decision: "block"` on PostToolUse, UserPromptSubmit, Stop, and
//     PreCompact; SubagentStop: "Neither a block nor `continue: false`
//     keeps the subagent running".
//   - `continue: false` on the factoryBlockEvents. It "stops nothing" on
//     Notification, SessionStart, and SessionEnd, and on Stop it ends the
//     turn.
func decideFactory(event string, h Handler, r Result) Decision {
	failure := Error
	if h.FailClosed && slices.Contains(factoryBlockEvents, event) {
		failure = Block
	}
	switch {
	case r.TimedOut && failure == Block:
		return Block
	case r.TimedOut:
		return Timeout
	case r.StartErr != nil:
		return failure
	case r.Exit == 2 && slices.Contains(factoryExitTwoErrors, event):
		return Error
	case r.Exit == 2:
		return Block
	}
	reply, kind := readFactory(event, r.Stdout)
	switch {
	case kind == factoryInvalid:
		return failure
	case r.Exit != 0 && (failure == Block || kind == factoryPlain):
		return failure
	case kind == factoryPlain:
		return Allow
	case event == "PreToolUse" && factoryPermission(reply) == "deny":
		return Block
	case reply.Continue != nil && !*reply.Continue && slices.Contains(factoryBlockEvents, event):
		return Block
	case factoryString(reply.Decision) == "block" && slices.Contains(factoryDecisionEvents, event):
		return Block
	}
	return Allow
}

// FactoryAsks reports whether a PreToolUse hook's applied JSON reply is
// `ask`, which leaves the call to Factory's normal permissions.
func FactoryAsks(event string, r Result) bool {
	if event != "PreToolUse" || r.TimedOut || r.StartErr != nil || r.Exit == 2 {
		return false
	}
	reply, kind := readFactory(event, r.Stdout)
	return kind == factoryJSON && factoryPermission(reply) == "ask"
}

// factoryAddsContext reports whether Droid adds the hook's output to the
// session: a JSON reply's additionalContext when the hook did not block,
// or plain stdout on exit 0 for UserPromptSubmit and SessionStart.
func factoryAddsContext(event string, r Result) bool {
	if decideFactory(event, Handler{}, r) != Allow || r.Exit == 2 {
		return false
	}
	reply, kind := readFactory(event, r.Stdout)
	switch kind {
	case factoryJSON:
		return reply.HookSpecificOutput != nil && factoryString(reply.HookSpecificOutput.AdditionalContext) != ""
	case factoryPlain:
		return r.Exit == 0 && slices.Contains(contextEvents, event) && strings.TrimSpace(r.Stdout) != ""
	}
	return false
}
