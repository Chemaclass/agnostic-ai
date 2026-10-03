package hookrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Source: docs.devin.ai/cli/extensibility/hooks and
// docs.devin.ai/cli/extensibility/hooks/lifecycle-hooks (rechecked
// 2026-10-03), for the Devin CLI hooks sync writes to
// `.devin/hooks.v1.json`. Documented there:
//   - Payload: "Event data is passed as JSON on **stdin**", with
//     hook_event_name, session_id, and prompt_id on every event, plus the
//     event's own fields: tool_name and tool_input on the tool events,
//     tool_response ("Object with `success` (boolean), `output` (string),
//     and `error` (string or null)") on PostToolUse, prompt on
//     UserPromptSubmit, stop_hook_active on Stop, and summary ("may be
//     null") on PostCompaction. The shell tool is `exec`, whose
//     tool_input the page shows as `{ "command": "rm -rf /", "shell_id":
//     "main" }`.
//   - Env: "The `DEVIN_PROJECT_DIR` environment variable is automatically
//     set to the project root directory."
//   - Matcher: "Regex matched against the hook event's `tool_name`. Empty
//     string or an omitted matcher matches all tool names", and `"exec"`
//     matches "Tool names containing `exec`". It is "available for
//     tool-related events: `PreToolUse`, `PostToolUse`, and
//     `PermissionRequest`".
//   - Exit codes and replies: see readWindsurf.
//
// Not documented, so hook run assumes or refuses them:
//   - The shell: `command` is a "Shell command to run", with no shell
//     named and nothing on Windows.
//   - The working directory a command runs in.
//   - The default timeout: "Timeout in seconds (optional)".
//   - The input of the file tools: "`read`, `write`, `edit`,
//     `apply_patch`" are named with no tool_input fields.
//   - SessionStart's `source` ("How the session was started") and
//     SessionEnd's `reason` ("Why the session ended") list no values.
const windsurfDocs = "https://docs.devin.ai/cli/extensibility/hooks"

// windsurfAssumedTimeout is the default hook run uses when a Devin CLI
// hook sets no timeout, the same 30 seconds it assumes for Cursor.
const windsurfAssumedTimeout = cursorAssumedTimeout

// windsurfToolEvents are the events that carry a tool_name and read the
// matcher.
var windsurfToolEvents = []string{"PreToolUse", "PostToolUse", "PermissionRequest"}

// windsurfDecisionEvents are the events a block stops: PreToolUse ("Use
// this to block"), PermissionRequest ("custom approval logic"), Stop
// ("prevent premature stopping"), and UserPromptSubmit, the user's
// message, which exit 2's "action is denied" applies to.
var windsurfDecisionEvents = []string{"PreToolUse", "PermissionRequest", "UserPromptSubmit", "Stop"}

// windsurfMatches treats the matcher as an unanchored regular expression
// and empty as everything. A matcher that does not compile leaves the
// target unbuilt: the docs do not say what Devin CLI does with one.
func windsurfMatches(matcher, value string) (bool, error) {
	if matcher == "" {
		return true, nil
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return false, Unbuilt{fmt.Sprintf("Devin CLI does not document how it reads matcher %q, which hook run cannot compile: %v", matcher, err)}
	}
	return re.MatchString(value), nil
}

// windsurfEventMatcher refuses a matcher on an event with no tool_name:
// the docs say only that "" or no matcher runs the hook "for every
// event of that type".
func windsurfEventMatcher(event, matcher string) error {
	if matcher == "" || slices.Contains(windsurfToolEvents, event) {
		return nil
	}
	return Unbuilt{fmt.Sprintf("Devin CLI does not document what matcher %q does on %s, which has no tool_name; drop it from the spec", matcher, event)}
}

// buildWindsurf writes Devin CLI's payload. --bash calls exec on the tool
// events, --prompt builds UserPromptSubmit, and Stop and PostCompaction
// need no input. --edit is refused, since the file tools' input is
// undocumented.
func buildWindsurf(event, matcher, _ string, in Input) (Payload, error) {
	tool := slices.Contains(windsurfToolEvents, event)
	switch {
	case in.Edit != "" && in.Bash != "":
		return Payload{}, errors.New("--edit and --bash are mutually exclusive")
	case !tool && (in.Edit != "" || in.Bash != ""):
		return Payload{}, fmt.Errorf("--edit and --bash build tool events, not %s", event)
	case in.Prompt != "" && event != "UserPromptSubmit":
		return Payload{}, fmt.Errorf("--prompt builds UserPromptSubmit, not %s", event)
	case in.Edit != "":
		return Payload{}, Unbuilt{"Devin CLI documents no tool_input for its edit, write, or apply_patch tools; pass --payload <file>"}
	}
	if err := windsurfEventMatcher(event, matcher); err != nil {
		return Payload{}, err
	}
	doc := map[string]any{"hook_event_name": event, "session_id": SessionID, "prompt_id": turnID}
	p := Payload{Fires: true}
	switch event {
	case "PreToolUse", "PostToolUse", "PermissionRequest":
		if in.Bash == "" {
			return Payload{}, fmt.Errorf("%s needs --bash <command> or --payload <file>", event)
		}
		p.Trigger = "exec"
		var err error
		if p.Fires, err = windsurfMatches(matcher, p.Trigger); err != nil {
			return Payload{}, err
		}
		doc["tool_name"], doc["tool_input"] = p.Trigger, map[string]any{"command": in.Bash, "shell_id": "main"}
		if event == "PostToolUse" {
			doc["tool_response"] = map[string]any{"success": true, "output": "", "error": nil}
		}
	case "UserPromptSubmit":
		if in.Prompt == "" {
			return Payload{}, fmt.Errorf("%s needs --prompt <text> or --payload <file>", event)
		}
		p.Trigger = "prompt"
		doc["prompt"] = in.Prompt
	case "Stop":
		p.Trigger = "stop"
		doc["stop_hook_active"] = false
	case "PostCompaction":
		p.Trigger = "compaction"
		doc["summary"] = nil
	case "SessionStart":
		return Payload{}, Unbuilt{"Devin CLI documents no values for SessionStart's source; pass --payload <file>"}
	case "SessionEnd":
		return Payload{}, Unbuilt{"Devin CLI documents no values for SessionEnd's reason; pass --payload <file>"}
	default:
		return Payload{}, fmt.Errorf("hook run builds no windsurf %s payload; pass --payload <file>", event)
	}
	return marshal(p, doc)
}

// windsurfRawPayload matches a --payload tool call's tool_name; the
// other events take no matcher.
func windsurfRawPayload(event, matcher string, p Payload) (Payload, error) {
	if !slices.Contains(windsurfToolEvents, event) {
		return p, windsurfEventMatcher(event, matcher)
	}
	p.Trigger = PayloadTool(p.Body)
	var err error
	p.Fires, err = windsurfMatches(matcher, p.Trigger)
	return p, err
}

// windsurfRead is what hook run reads from one Devin CLI result.
// Uncounted, when set, is why the result stays out of --expect and the
// comparison whatever --include-assumed says.
type windsurfRead struct {
	decision  Decision
	note      string
	uncounted string
}

// readWindsurf follows the exit codes: 0 is "Success" ("hook continues
// normally"), 2 is "Block" ("action is denied"), and any other is
// "Error" ("logged but doesn't block"). A reply's `decision` is
// "`"approve"` to allow the action, or `"block"` to deny it". The docs
// do not say whether a reply counts on a non-zero exit, or what a block
// does on the events with no decision to make, so those results are not
// counted; exit 2 with `decision: "block"` blocks either way.
func readWindsurf(event string, r Result) windsurfRead {
	switch {
	case r.TimedOut:
		return windsurfRead{decision: Timeout}
	case r.StartErr != nil:
		return windsurfRead{decision: Error}
	}
	decision, replied, problem := windsurfReply(r)
	read := windsurfRead{decision: Allow}
	switch {
	case r.Exit == 2, r.Exit == 0 && decision == "block":
		read.decision = Block
	case r.Exit != 0:
		read.decision = Error
	}
	switch {
	case read.decision == Block && !slices.Contains(windsurfDecisionEvents, event):
		read.uncounted = "Devin CLI does not document what a block does on " + event
	case r.Exit != 0 && replied && (r.Exit != 2 || decision != "block"):
		read.uncounted = "Devin CLI does not document whether it reads a reply on a non-zero exit"
	case problem != "":
		read.uncounted = problem
	}
	if event == "PreToolUse" && read.decision == Allow && r.Exit == 0 && len(windsurfEventOutput(event, r).UpdatedInput) > 0 {
		read.note = "replied updatedInput: Devin CLI merges it into the tool's arguments before the tool runs"
	}
	return read
}

// windsurfReply reads the `decision` of a JSON reply on stdout. replied
// is false when stdout is no JSON object or names no decision, and
// problem is set when the decision is not one the docs list.
func windsurfReply(r Result) (decision string, replied bool, problem string) {
	var reply map[string]json.RawMessage
	out := strings.TrimSpace(r.Stdout)
	if !strings.HasPrefix(out, "{") || json.Unmarshal([]byte(out), &reply) != nil {
		return "", false, ""
	}
	raw, present := reply["decision"]
	if !present || string(raw) == "null" {
		return "", false, ""
	}
	if json.Unmarshal(raw, &decision) != nil || decision != "approve" && decision != "block" {
		return decision, true, `Devin CLI documents decision "approve" or "block", and this reply's is neither`
	}
	return decision, true, ""
}

// windsurfOutput is a reply's hookSpecificOutput: additionalContext,
// "Text injected into the agent's context", and updatedInput, an
// "Object merged into the tool's arguments before execution".
type windsurfOutput struct {
	HookEventName     string         `json:"hookEventName"`
	AdditionalContext string         `json:"additionalContext"`
	UpdatedInput      map[string]any `json:"updatedInput"`
}

// windsurfEventOutput is the reply's hookSpecificOutput when its
// hookEventName, the "Event the output applies to", is event, and empty
// otherwise, so an untagged or mistagged output claims no effect.
func windsurfEventOutput(event string, r Result) windsurfOutput {
	var reply struct {
		HookSpecificOutput windsurfOutput `json:"hookSpecificOutput"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(r.Stdout)), &reply) != nil || reply.HookSpecificOutput.HookEventName != event {
		return windsurfOutput{}
	}
	return reply.HookSpecificOutput
}

// WindsurfNote is the note a reply earns, such as a rewritten tool
// input, or "" when it needs none.
func WindsurfNote(event string, r Result) string {
	return readWindsurf(event, r).note
}

// WindsurfUncounted is why a result stays out of --expect and the
// comparison even with --include-assumed, or "" when it counts.
func WindsurfUncounted(event string, r Result) string {
	return readWindsurf(event, r).uncounted
}

// windsurfContextEvents take hookSpecificOutput.additionalContext:
// "Text injected into the agent's context (for `UserPromptSubmit`,
// `SessionStart`, `PostToolUse`)".
var windsurfContextEvents = []string{"UserPromptSubmit", "SessionStart", "PostToolUse"}

func windsurfAddsContext(event string, r Result) bool {
	if r.TimedOut || r.StartErr != nil || r.Exit != 0 || !slices.Contains(windsurfContextEvents, event) {
		return false
	}
	return windsurfEventOutput(event, r).AdditionalContext != ""
}
