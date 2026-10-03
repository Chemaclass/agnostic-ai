package hookrun

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// Source: docs.factory.com/cli/configuration/hooks-guide (rechecked
// 2026-10-03). Documented there:
//   - Project root: "Use `"$FACTORY_PROJECT_DIR"/path/to/script.sh` for
//     project scripts"; see expandFactoryRoot.
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
const factoryDocs = "https://docs.factory.com/cli/configuration/hooks-guide"

const factoryDefaultTimeout = 60 * time.Second

// factoryRootRefs are the forms of the project root reference the hooks
// guide writes, quoted forms first so the quotes go with them.
var factoryRootRefs = []string{`"$FACTORY_PROJECT_DIR"`, `"${FACTORY_PROJECT_DIR}"`, `${FACTORY_PROJECT_DIR}`, `$FACTORY_PROJECT_DIR`}

// expandFactoryRoot replaces each project root reference with root, so
// a command written as the guide says runs as a script path: root is
// the quoted project root when the hook runs, and a plain word when hook
// run checks the command is shell-neutral.
func expandFactoryRoot(command, root string) string {
	pairs := make([]string, 0, 2*len(factoryRootRefs))
	for _, ref := range factoryRootRefs {
		pairs = append(pairs, ref, root)
	}
	return strings.NewReplacer(pairs...).Replace(command)
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
				return Payload{}, errors.New("--edit: Factory documents no tool_input for its Edit and ApplyPatch tools; pass --payload <file>")
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

// factoryLifecycleEvents only surface exit 2's stderr: "`PreToolUse`
// blocks the tool call, `PostToolUse` and `Stop` feed stderr back to
// Droid, and `UserPromptSubmit` blocks prompt processing. Other
// lifecycle events surface stderr to the user."
var factoryLifecycleEvents = []string{"Notification", "SubagentStop", "PreCompact", "SessionStart", "SessionEnd"}

// factoryDecisionEvents block on a JSON `decision: "block"`: PostToolUse,
// UserPromptSubmit, Stop, and SubagentStop.
var factoryDecisionEvents = []string{"PostToolUse", "UserPromptSubmit", "Stop", "SubagentStop"}

// decideFactory follows the hooks guide: exit 2 blocks, except on the
// lifecycle events; "Any other non-zero exit" is a "Non-blocking error";
// on exit 0 a JSON reply blocks with `continue: false` ("Stops processing
// after the hook"), except on SessionEnd, which "Cannot block session
// termination"; with `permissionDecision` deny, or ask, which "Forces a
// user confirmation prompt", on PreToolUse; and with `decision: "block"`
// on the factoryDecisionEvents.
func decideFactory(event string, r Result) Decision {
	switch {
	case r.TimedOut:
		return Timeout
	case r.StartErr != nil:
		return Error
	case r.Exit == 2 && slices.Contains(factoryLifecycleEvents, event):
		return Error
	case r.Exit == 2:
		return Block
	case r.Exit != 0:
		return Error
	}
	reply, ok := readReply(r)
	switch {
	case !ok:
		return Allow
	case reply.Continue != nil && !*reply.Continue && event != "SessionEnd":
		return Block
	case event == "PreToolUse" && reply.HookSpecificOutput.PermissionDecision == "deny":
		return Block
	case FactoryAsks(event, r):
		return Block
	case reply.Decision == "block" && slices.Contains(factoryDecisionEvents, event):
		return Block
	}
	return Allow
}

// FactoryAsks reports whether a PreToolUse hook replied
// `permissionDecision: "ask"`, which Factory enforces by asking the user.
func FactoryAsks(event string, r Result) bool {
	if event != "PreToolUse" || r.TimedOut || r.StartErr != nil || r.Exit != 0 {
		return false
	}
	reply, ok := readReply(r)
	return ok && reply.HookSpecificOutput.PermissionDecision == "ask"
}
