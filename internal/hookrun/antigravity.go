package hookrun

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Source: antigravity.google/docs/hooks (rechecked 2026-10-03). Documented
// there:
//   - Payload: "Hooks receive input through stdin as JSON and return
//     output through stdout as JSON. Field names use camelCase." Every
//     hook gets conversationId, workspacePaths, transcriptPath,
//     artifactDirectoryPath, and modelName; PreToolUse and PostToolUse add
//     toolCall (name, args) and stepIdx, and PostToolUse an optional error.
//   - Tools: "run_command : Propose a Bash command to run. Arguments :
//     CommandLine , Cwd , WaitMsBeforeAsync"; "write_to_file : Create new
//     files. Arguments : TargetFile , Overwrite , CodeContent ,
//     Description"; replace_file_content and multi_replace_file_content
//     list theirs too.
//   - Matcher: "you can use a regular expression in the matcher field";
//     `""` or `"*"` match all tools, and `"run_command"` matches "exactly
//     run_command", so hook run anchors it.
//   - Timeout: "Timeout in seconds. Defaults to 30."
//   - Replies: see readAntigravity.
//
// Not documented, so hook run assumes them or does not count the result:
//   - The shell: a command is "The shell command to execute", with no
//     shell named and nothing on Windows.
//   - The working directory a command runs in.
//   - Exit codes: the page gives no meaning to any exit status, so only an
//     exit 0 with a reply of the documented shape is counted.
const antigravityDocs = "https://antigravity.google/docs/hooks"

const antigravityDefaultTimeout = 30 * time.Second

// antigravityDecisions are PreToolUse's documented `decision` values.
var antigravityDecisions = []string{"allow", "deny", "ask", "force_ask", "deny_unless_prior_grant"}

// antigravityEditTools are the file tools whose arguments the page lists,
// in the order --edit tries them.
var antigravityEditTools = []string{"write_to_file", "replace_file_content", "multi_replace_file_content"}

// antigravityMatches anchors the matcher, since "run_command" matches
// "exactly run_command". A matcher that does not compile leaves the
// target unbuilt: the page does not say what Antigravity does with one.
func antigravityMatches(matcher, value string) (bool, error) {
	if matcher == "" || matcher == "*" {
		return true, nil
	}
	re, err := regexp.Compile("^(?:" + matcher + ")$")
	if err != nil {
		return false, Unbuilt{fmt.Sprintf("Antigravity does not document how it reads matcher %q, which hook run cannot compile: %v", matcher, err)}
	}
	return re.MatchString(value), nil
}

// buildAntigravity writes Antigravity's payload: --bash calls run_command,
// and --edit the first file tool the matcher matches, with the arguments
// the page lists for it. Antigravity has no prompt event.
func buildAntigravity(event, matcher, root string, in Input) (Payload, error) {
	if err := checkInput("antigravity", event, "PreToolUse", "PostToolUse", "", in); err != nil {
		return Payload{}, err
	}
	if event == "SessionStart" {
		return Payload{}, fmt.Errorf("hook run builds no antigravity %s payload; pass --payload <file>", event)
	}
	doc := map[string]any{
		"conversationId": SessionID, "workspacePaths": []string{root}, "transcriptPath": "",
		"artifactDirectoryPath": "", "modelName": "", "stepIdx": 0,
	}
	p := Payload{}
	var args map[string]any
	var err error
	if in.Bash != "" {
		p.Trigger = "run_command"
		p.Fires, err = antigravityMatches(matcher, p.Trigger)
		args = map[string]any{"CommandLine": in.Bash, "Cwd": root, "WaitMsBeforeAsync": 0}
	} else {
		p.Trigger, p.Fires, err = firstMatch(antigravityMatches, matcher, antigravityEditTools)
		file := absPath(root, in.Edit)
		args = map[string]any{"TargetFile": file, "Description": "", "Instruction": ""}
		switch p.Trigger {
		case "write_to_file":
			_, statErr := os.Stat(file)
			delete(args, "Instruction")
			args["Overwrite"], args["CodeContent"] = statErr == nil, ""
		case "replace_file_content":
			args["AllowMultiple"], args["TargetContent"], args["ReplacementContent"] = false, "", ""
			args["StartLine"], args["EndLine"] = 1, 1
		default:
			args["ReplacementChunks"] = []any{}
		}
	}
	if err != nil {
		return Payload{}, err
	}
	doc["toolCall"] = map[string]any{"name": p.Trigger, "args": args}
	if event == "PostToolUse" {
		doc["error"] = ""
	}
	return marshal(p, doc)
}

// antigravityRawPayload matches a --payload tool call's toolCall.name;
// the other events ignore the matcher.
func antigravityRawPayload(event, matcher string, p Payload) (Payload, error) {
	if event != "PreToolUse" && event != "PostToolUse" {
		return p, nil
	}
	var call struct {
		ToolCall struct {
			Name string `json:"name"`
		} `json:"toolCall"`
	}
	_ = json.Unmarshal(p.Body, &call)
	p.Trigger = call.ToolCall.Name
	var err error
	p.Fires, err = antigravityMatches(matcher, p.Trigger)
	return p, err
}

// antigravityRead is what hook run reads from one Antigravity result.
// Uncounted, when set, is why the result stays out of --expect and the
// comparison whatever --include-assumed says.
type antigravityRead struct {
	decision  Decision
	note      string
	uncounted string
}

// readAntigravity follows the reply contract. PreToolUse needs a
// `decision`: "allow" allows; "deny" "Hard blocks execution immediately";
// "ask" "Prompts you for approval, but respects “Always Allow”
// settings"; "force_ask" "Always prompts you for approval"; and
// "deny_unless_prior_grant" "Denies execution unless the resource was
// previously approved in a prior grant". Stop needs one too: "continue"
// keeps the agent running and "Any other value allows the stop".
// PostToolUse "Returns an empty JSON object {}", and PreInvocation and
// PostInvocation return optional fields. Anything else, a non-zero exit
// included, is a result the page does not describe. ask and
// deny_unless_prior_grant depend on saved permissions hook run cannot
// see, so they are not counted either.
func readAntigravity(event string, r Result) antigravityRead {
	switch {
	case r.TimedOut:
		return antigravityRead{decision: Timeout, uncounted: "Antigravity does not document what a timed-out hook does"}
	case r.StartErr != nil:
		return antigravityRead{decision: Error, uncounted: "Antigravity does not document what a hook that did not start does"}
	case r.Exit != 0:
		return antigravityRead{decision: Error, uncounted: "Antigravity does not document exit codes"}
	}
	var reply map[string]json.RawMessage
	out := strings.TrimSpace(r.Stdout)
	if !strings.HasPrefix(out, "{") || json.Unmarshal([]byte(out), &reply) != nil {
		return antigravityRead{decision: Error, uncounted: "Antigravity replies with a JSON object on stdout, and this one is not"}
	}
	if problem := antigravityFieldProblem(event, reply); problem != "" {
		return antigravityRead{decision: Error, uncounted: problem}
	}
	var decision string
	_ = json.Unmarshal(reply["decision"], &decision)
	switch {
	case event == "Stop" && decision == "continue":
		return antigravityRead{decision: Block, note: `replied "continue": Antigravity keeps the agent running; read as block`}
	case event != "PreToolUse":
		return antigravityRead{decision: Allow}
	case decision == "deny":
		return antigravityRead{decision: Block}
	case decision == "deny_unless_prior_grant" || decision == "ask":
		// A saved grant or Always Allow setting lets the call run at once,
		// and hook run cannot see either.
		return antigravityRead{decision: Block, uncounted: "replied " + decision + ": the result depends on Antigravity's saved permissions"}
	case decision == "force_ask":
		return antigravityRead{decision: Block, note: "replied force_ask: Antigravity asks the user before the tool runs; read as block"}
	}
	return antigravityRead{decision: Allow}
}

// antigravityFieldProblem names the first reply field that is missing or
// not of its documented type, "" when the reply has the documented shape.
func antigravityFieldProblem(event string, reply map[string]json.RawMessage) string {
	isString := func(key string) bool {
		var text any
		if json.Unmarshal(reply[key], &text) != nil {
			return false
		}
		_, ok := text.(string)
		return ok
	}
	optionalString := func(key string) string {
		if _, present := reply[key]; present && !isString(key) {
			return fmt.Sprintf("Antigravity documents %s as a string, and this reply's is not", key)
		}
		return ""
	}
	switch event {
	case "PreToolUse", "Stop":
		if _, present := reply["decision"]; !present || !isString("decision") {
			return "Antigravity requires a string decision in the reply, and this one has none"
		}
		if event == "PreToolUse" {
			var decision string
			_ = json.Unmarshal(reply["decision"], &decision)
			if !slices.Contains(antigravityDecisions, decision) {
				return fmt.Sprintf("Antigravity documents no decision %q; it lists %s", decision, strings.Join(antigravityDecisions, ", "))
			}
			if raw, present := reply["permissionOverrides"]; present {
				var overrides []string
				if json.Unmarshal(raw, &overrides) != nil || overrides == nil {
					return "Antigravity documents permissionOverrides as an array of strings, and this reply's is not"
				}
			}
		}
		return optionalString("reason")
	case "PostInvocation":
		if problem := optionalString("terminationBehavior"); problem != "" {
			return problem
		}
	}
	return ""
}

// AntigravityNote is the note a reply earns, such as an ask read as
// block, or "" when it needs none.
func AntigravityNote(event string, r Result) string {
	return readAntigravity(event, r).note
}

// AntigravityUncounted is why a result stays out of --expect and the
// comparison even with --include-assumed, or "" when it counts: only an
// exit 0 whose reply has the documented shape does.
func AntigravityUncounted(event string, r Result) string {
	return readAntigravity(event, r).uncounted
}

// antigravityAddsContext reports whether a reply adds to the session:
// injectSteps on PreInvocation and PostInvocation, and a Stop reason,
// which "is injected as a system message" with decision "continue".
func antigravityAddsContext(event string, r Result) bool {
	if readAntigravity(event, r).uncounted != "" {
		return false
	}
	var reply struct {
		InjectSteps []any  `json:"injectSteps"`
		Decision    string `json:"decision"`
		Reason      string `json:"reason"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(r.Stdout)), &reply) != nil {
		return false
	}
	switch event {
	case "PreInvocation", "PostInvocation":
		return len(reply.InjectSteps) > 0
	case "Stop":
		return reply.Decision == "continue" && reply.Reason != ""
	}
	return false
}

// antigravityHooks reads `.agents/hooks.json`, which is keyed by hook
// definition name, into matcher groups per event. PreToolUse and
// PostToolUse hold `{matcher, hooks}` groups; the other events list
// handlers directly, and "the matcher is ignored". A definition with
// `"enabled": false` does not run.
func antigravityHooks(body []byte) (map[string][]nativeGroup, error) {
	var defs map[string]map[string]json.RawMessage
	if err := json.Unmarshal(body, &defs); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	slices.Sort(names)
	events := map[string][]nativeGroup{}
	for _, name := range names {
		def := defs[name]
		var enabled *bool
		if raw, ok := def["enabled"]; ok {
			if err := json.Unmarshal(raw, &enabled); err != nil {
				return nil, fmt.Errorf("%s: enabled: %w", name, err)
			}
		}
		if enabled != nil && !*enabled {
			continue
		}
		for event, raw := range def {
			if event == "enabled" {
				continue
			}
			if event == "PreToolUse" || event == "PostToolUse" {
				var groups []nativeGroup
				if err := json.Unmarshal(raw, &groups); err != nil {
					return nil, fmt.Errorf("%s: %s: %w", name, event, err)
				}
				events[event] = append(events[event], groups...)
				continue
			}
			var handlers []nativeHandler
			if err := json.Unmarshal(raw, &handlers); err != nil {
				return nil, fmt.Errorf("%s: %s: %w", name, event, err)
			}
			events[event] = append(events[event], nativeGroup{Hooks: handlers})
		}
	}
	return events, nil
}
