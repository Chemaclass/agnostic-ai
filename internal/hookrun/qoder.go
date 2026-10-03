package hookrun

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Sources: docs.qoder.com/cli/hooks and docs.qoder.com/cli/hooks-reference
// (rechecked 2026-10-03). Sync writes Qoder CLI's `.qoder/settings.json`,
// so the CLI pages are the contract; the IDE's 30 second default does not
// apply. Documented there:
//   - Timeout: "Timeout in seconds, default 600".
//   - Shell: "`\"bash\"` or `\"powershell\"`; system default if omitted",
//     and "Shell form (default): `command` is a shell snippet. The CLI runs
//     `bash -c \"<command>\"` (or PowerShell)".
//   - Exec form: "When set, the hook runs in exec form (no shell)", and
//     "`.bat`/`.cmd` scripts cannot be exec'd directly" on Windows.
//   - Project root: "`QODER_PROJECT_DIR` | Working directory of the current
//     project", and "Under `bash`, the shell expands them at runtime".
//   - Payload: session_id, transcript_path, cwd, hook_event_name, and
//     permission_mode on every event; PreToolUse's tool_name, tool_input
//     (`{"command": ...}` on Bash), and tool_use_id; PostToolUse adds
//     tool_response and shows Write's `{"file_path": ..., "content": ...}`.
//   - Matcher: "Omitted or `\"*\"` | Match all", an exact value, `|`
//     separated values, or a regular expression.
//   - `if`: "The tool-name part reuses the same matching logic as
//     `matcher`", and "The `arg_pattern` inside parentheses uses **glob
//     matching** (not regex), and is checked against the tool's primary
//     argument (e.g., Bash's `command`, file tools' `file_path`)".
//   - Replies: see decideQoder.
//
// Not documented, so hook run assumes or refuses them:
//   - The working directory a hook runs in. hook run assumes the project
//     root.
//   - The shell when `shell` is omitted, "system default". hook run
//     assumes `sh -c` for a shell-neutral command on macOS and Linux, and
//     runs no shell-form command on Windows.
//   - Which PowerShell `shell: powershell` starts, and which bash
//     `shell: bash` starts on Windows.
//   - Whether exec form expands `${VAR}` in `command` or `args`.
//   - The Edit tool's tool_input; only Write's is shown.
//   - The decision rules of TaskCreated, TaskCompleted, TeammateIdle, and
//     Setup, which the event reference leaves out.
const qoderDocs = "https://docs.qoder.com/cli/hooks"

const qoderDefaultTimeout = 600 * time.Second

const qoderRootVar = "QODER_PROJECT_DIR"

// qoderSources are SessionStart's documented sources.
var qoderSources = []string{"startup", "resume", "clear", "compact", "new"}

// qoderUndecidedEvents are in the hooks reference's event list but not in
// its event catalog, so what their result does is undocumented.
var qoderUndecidedEvents = []string{"TaskCreated", "TaskCompleted", "TeammateIdle", "Setup"}

// qoderUndecided is why hook run cannot read a Qoder result for event, or
// "" when the docs say how.
func qoderUndecided(event string) string {
	if slices.Contains(qoderUndecidedEvents, event) {
		return fmt.Sprintf("Qoder documents no decision rules for %s", event)
	}
	return ""
}

// buildQoder writes Qoder's payload. --bash calls Bash; --edit calls
// Write, the one file tool whose input the docs show, and is refused when
// the matcher picks Edit instead.
func buildQoder(event, matcher, root string, in Input) (Payload, error) {
	if err := checkInput("qoder", event, "PreToolUse", "PostToolUse", "UserPromptSubmit", in); err != nil {
		return Payload{}, err
	}
	doc := map[string]any{
		"session_id": SessionID, "transcript_path": "", "cwd": root,
		"hook_event_name": event, "permission_mode": "default",
	}
	p := Payload{Fires: true}
	var err error
	switch event {
	case "UserPromptSubmit":
		p.Trigger = "prompt"
		doc["prompt"] = in.Prompt
	case "SessionStart":
		p.Trigger, p.Fires, err = firstMatch(matches, matcher, qoderSources)
		doc["source"], doc["model"] = p.Trigger, "Auto"
	default:
		var input map[string]any
		if in.Bash != "" {
			p.Trigger = "Bash"
			p.Fires, err = matches(matcher, p.Trigger)
			input = map[string]any{"command": in.Bash}
		} else {
			p.Trigger, p.Fires, err = firstMatch(matches, matcher, []string{"Write", "Edit"})
			if err == nil && p.Fires && p.Trigger != "Write" {
				return Payload{}, Unbuilt{"Qoder documents no tool_input for its Edit tool; pass --payload <file>"}
			}
			input = map[string]any{"file_path": absPath(root, in.Edit), "content": ""}
		}
		doc["tool_name"], doc["tool_input"], doc["tool_use_id"] = p.Trigger, input, toolUseID
		if event == "PostToolUse" {
			doc["tool_response"] = map[string]any{}
		}
	}
	if err != nil {
		return Payload{}, err
	}
	return marshal(p, doc)
}

// qoderExit2Events are the events whose overview row says "exit 2
// blocks", plus Elicitation and ElicitationResult, where exit 2 "declines
// the elicitation" or "rewrites action to `decline`". ConfigChange blocks
// "except `policy_settings` source"; see QoderPolicyChange.
var qoderExit2Events = []string{"UserPromptSubmit", "PreToolUse", "Stop", "SubagentStop", "PreCompact", "ConfigChange", "Elicitation", "ElicitationResult"}

// qoderIgnoredEvents are notification only: "output and exit code are
// ignored".
var qoderIgnoredEvents = []string{"StopFailure", "InstructionsLoaded"}

type qoderReply struct {
	Continue           *bool           `json:"continue"`
	Decision           string          `json:"decision"`
	HookSpecificOutput json.RawMessage `json:"hookSpecificOutput"`
}

type qoderSpecificOutput struct {
	HookEventName      *string `json:"hookEventName"`
	PermissionDecision string  `json:"permissionDecision"`
	Action             string  `json:"action"`
	Decision           struct {
		Behavior string `json:"behavior"`
	} `json:"decision"`
}

// readQoderReply parses exit 0's stdout: "When exit is 0 and stdout is
// valid JSON, the CLI parses it"; otherwise it is plain text. valid is
// false for a hookSpecificOutput without hookEventName: "otherwise the
// entire JSON output is rejected and the TUI shows `<hookName> hook
// error`".
func readQoderReply(r Result) (reply qoderReply, specific qoderSpecificOutput, isJSON, valid bool) {
	out := strings.TrimSpace(r.Stdout)
	if !strings.HasPrefix(out, "{") || json.Unmarshal([]byte(out), &reply) != nil {
		return reply, specific, false, true
	}
	if len(reply.HookSpecificOutput) == 0 || string(reply.HookSpecificOutput) == "null" {
		return reply, specific, true, true
	}
	if json.Unmarshal(reply.HookSpecificOutput, &specific) != nil || specific.HookEventName == nil {
		return reply, specific, true, false
	}
	return reply, specific, true, true
}

// decideQoder follows the hooks guide. Exit 2 blocks on the
// qoderExit2Events; on WorktreeCreate "any non-zero exit code is treated
// as failure"; "Other values: non-blocking error". At exit 0 a JSON
// reply blocks with `continue: false` ("requests stopping subsequent
// execution"), with `decision: "deny"` ("equivalent to exit 2"), and on
// PreToolUse with `permissionDecision` deny or ask, which "takes
// precedence" over `decision`. PermissionRequest blocks on a
// `decision.behavior` of deny, and the elicitation events on an
// `action` of decline or cancel.
func decideQoder(event string, r Result) Decision {
	ignored := slices.Contains(qoderIgnoredEvents, event)
	switch {
	case r.TimedOut:
		return Timeout
	case r.StartErr != nil:
		return Error
	case ignored:
		return Allow
	case r.Exit != 0 && event == "WorktreeCreate":
		return Block
	case r.Exit == 2 && slices.Contains(qoderExit2Events, event):
		return Block
	case r.Exit != 0:
		return Error
	}
	reply, specific, isJSON, valid := readQoderReply(r)
	switch {
	case !isJSON:
		return Allow
	case !valid:
		return Error
	case reply.Continue != nil && !*reply.Continue:
		return Block
	case event == "PreToolUse" && specific.PermissionDecision != "":
		if specific.PermissionDecision == "deny" || specific.PermissionDecision == "ask" {
			return Block
		}
		return Allow
	case reply.Decision == "deny" && slices.Contains(qoderExit2Events, event):
		return Block
	case event == "PermissionRequest" && specific.Decision.Behavior == "deny":
		return Block
	case (event == "Elicitation" || event == "ElicitationResult") && (specific.Action == "decline" || specific.Action == "cancel"):
		return Block
	}
	return Allow
}

// QoderAsks reports whether a PreToolUse hook replied
// `permissionDecision: "ask"`, which Qoder enforces by asking the user.
func QoderAsks(event string, r Result) bool {
	if event != "PreToolUse" || r.TimedOut || r.StartErr != nil || r.Exit != 0 {
		return false
	}
	_, specific, isJSON, valid := readQoderReply(r)
	return isJSON && valid && specific.PermissionDecision == "ask"
}

// QoderPolicyChange reports a ConfigChange payload whose source is
// policy_settings: "hooks still fire for audit purposes but the change is
// enforced and cannot be blocked".
func QoderPolicyChange(event string, body []byte) bool {
	var change struct {
		Source string `json:"source"`
	}
	return event == "ConfigChange" && json.Unmarshal(body, &change) == nil && change.Source == "policy_settings"
}

// IfRuns reports whether target runs a handler whose `if` is rule for
// the event in body: Qoder by its own rules, Claude Code by its
// permission rules.
func IfRuns(target, rule, event string, body []byte, root string) (bool, error) {
	if target == "qoder" {
		return QoderIfRuns(rule, body)
	}
	return ClaudeIfRuns(rule, event, body, root)
}

// QoderIfRuns reports whether Qoder fires a handler whose `if` is rule,
// `ToolName` or `ToolName(arg_pattern)`, for the tool call in body. The
// glob's `*` spans `/`, since the guide's `Edit(*.ts)` matches an
// absolute file_path. A call with no tool name matches no rule.
func QoderIfRuns(rule string, body []byte) (bool, error) {
	rule = strings.TrimSpace(rule)
	if rule == "" {
		return true, nil
	}
	var call struct {
		ToolName  string         `json:"tool_name"`
		ToolInput map[string]any `json:"tool_input"`
	}
	if err := json.Unmarshal(body, &call); err != nil {
		return false, fmt.Errorf("read the payload for if: %w", err)
	}
	if call.ToolName == "" {
		return false, nil
	}
	tool, pattern, hasPattern := strings.Cut(rule, "(")
	if hasPattern {
		var ok bool
		if pattern, ok = strings.CutSuffix(pattern, ")"); !ok {
			return false, fmt.Errorf("if %q: missing closing parenthesis", rule)
		}
	}
	if ok, err := matches(tool, call.ToolName); err != nil || !ok {
		return false, err
	}
	if !hasPattern {
		return true, nil
	}
	field := "file_path"
	if call.ToolName == "Bash" {
		field = "command"
	}
	arg, ok := call.ToolInput[field].(string)
	if !ok {
		return false, Unbuilt{fmt.Sprintf("Qoder documents no primary argument for %s, which if %q tests", call.ToolName, rule)}
	}
	return globMatch(pattern, arg), nil
}

// qoderAssumptions follows the shell rules above: exec form and
// `shell: bash` on macOS and Linux are documented; an omitted shell is
// assumed `sh -c` for a shell-neutral command. The working directory is
// always assumed.
func qoderAssumptions(goos string, h Handler) ([]Assumption, string) {
	cwd := Assumption{Item: "working directory", Value: "project root", Reason: "Qoder does not document the directory a hook runs in"}
	if len(h.Args) > 0 {
		if strings.Contains(h.Command, "$") || slices.ContainsFunc(h.Args, func(a string) bool { return strings.Contains(a, "$") }) {
			return nil, "Qoder does not document whether exec form expands variables in command or args"
		}
		if ext := strings.ToLower(filepath.Ext(h.Command)); goos == "windows" && (ext == ".bat" || ext == ".cmd") {
			return nil, "Qoder documents that .bat and .cmd scripts cannot be exec'd directly on Windows"
		}
		return []Assumption{cwd}, ""
	}
	switch {
	case h.Shell == "powershell":
		return nil, "Qoder does not document which PowerShell runs a shell: powershell hook"
	case goos == "windows":
		return nil, "Qoder documents its Windows shell only as \"system default\""
	case h.Shell == "bash":
		return []Assumption{cwd}, ""
	case !ShellNeutral(expandRootVar(h.Command, qoderRootVar, "root")):
		return nil, "Qoder does not document its shell; use a script path"
	}
	shell := Assumption{Item: "shell", Value: "sh -c", Reason: "Qoder documents its default shell only as \"system default\""}
	return []Assumption{shell, cwd}, ""
}

// qoderArgv runs exec form with no shell, `shell: bash` with bash, and an
// omitted shell with the assumed `sh -c`.
func qoderArgv(h Handler) []string {
	switch {
	case len(h.Args) > 0:
		return append([]string{h.Command}, h.Args...)
	case h.Shell == "bash":
		return []string{"bash", "-c", h.Command}
	}
	return []string{"sh", "-c", h.Command}
}

// QoderHandlers reads the command handlers for event out of the
// `.qoder/settings.json` hooks block sync writes for one spec.
func QoderHandlers(body []byte, event string) ([]Handler, error) {
	if len(body) == 0 {
		return nil, nil
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Type    string            `json:"type"`
				Command string            `json:"command"`
				Args    []string          `json:"args"`
				Shell   string            `json:"shell"`
				If      string            `json:"if"`
				Env     map[string]string `json:"env"`
				Timeout float64           `json:"timeout"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	var out []Handler
	for _, group := range doc.Hooks[event] {
		for _, n := range group.Hooks {
			if n.Type != "command" {
				continue
			}
			out = append(out, Handler{
				Command: n.Command, Args: n.Args, Shell: n.Shell, If: n.If, Env: n.Env,
				Timeout: time.Duration(n.Timeout * float64(time.Second)),
			})
		}
	}
	return out, nil
}
