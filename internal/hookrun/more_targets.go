package hookrun

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Sources, by target:
//   - Trae: docs.trae.cn/ide_hook-configuration-reference and
//     docs.trae.cn/ide_automate-actions-with-hooks.
//   - OpenHands: docs.openhands.dev/openhands/usage/customization/hooks and
//     OpenHands/software-agent-sdk fad6377, openhands-sdk/openhands/sdk/hooks
//     (executor.py, config.py, manager.py, types.py).
//   - Goose: aaif-goose/goose bab8ff6, documentation/docs/guides/
//     context-engineering/hooks.md and crates/goose/src/hooks/mod.rs.
//   - Augment: docs.augmentcode.com/cli/hooks.
//   - Factory: see factory.go.
//   - Qoder: see qoder.go.
//   - Antigravity: see antigravity.go.
//   - Cline: see cline.go.
//   - Kiro: see kiro.go.
//   - Windsurf (Devin CLI): see windsurf.go.

type builder func(event, matcher, root string, in Input) (Payload, error)

var otherBuilders = map[string]builder{
	"trae":        buildTrae,
	"openhands":   buildOpenHands,
	"goose":       buildGoose,
	"augment":     buildAugment,
	"cursor":      buildCursor,
	"factory":     buildFactory,
	"copilot":     buildCopilot,
	"qoder":       buildQoder,
	"antigravity": buildAntigravity,
	"cline":       buildCline,
	"kiro":        buildKiro,
	"windsurf":    buildWindsurf,
}

func absPath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}

func relPath(root, path string) string {
	if rel, err := filepath.Rel(root, absPath(root, path)); err == nil && !strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(rel)
	}
	return path
}

func marshal(p Payload, doc map[string]any) (Payload, error) {
	var err error
	p.Body, err = json.Marshal(doc)
	return p, err
}

// unanchoredMatches treats the matcher as an unanchored regular
// expression, and empty or `*` as everything. A matcher that does not
// compile is compared as a literal name.
func unanchoredMatches(matcher, value string) (bool, error) {
	return geminiToolMatches(matcher, value)
}

// buildTrae writes Trae's payload. The docs list no tool_input fields for
// the edit tools, so --edit is Unbuilt.
func buildTrae(event, matcher, root string, in Input) (Payload, error) {
	if err := checkInput("trae", event, "PreToolUse", "PostToolUse", "UserPromptSubmit", in); err != nil {
		return Payload{}, err
	}
	if in.Edit != "" {
		return Payload{}, Unbuilt{"Trae documents no tool_input for its edit tools; pass --payload <file>"}
	}
	doc := map[string]any{"session_id": SessionID, "cwd": root, "hook_event_name": event, "workspace_roots": []string{root}}
	p := Payload{Fires: true}
	switch event {
	case "UserPromptSubmit":
		p.Trigger = "prompt"
		doc["prompt"] = in.Prompt
	case "SessionStart":
		p.Trigger = "session"
	default:
		p.Trigger = "RunCommand"
		fires, err := unanchoredMatches(matcher, p.Trigger)
		if err != nil {
			return Payload{}, err
		}
		p.Fires = fires
		doc["tool_use_id"] = toolUseID
		doc["tool_name"], doc["llm_tool_name"] = p.Trigger, p.Trigger
		doc["tool_input"] = map[string]any{"command": in.Bash}
		if event == "PostToolUse" {
			doc["tool_response"] = map[string]any{}
		}
	}
	return marshal(p, doc)
}

// buildOpenHands writes OpenHands' HookEvent. The shell tool is
// `terminal`; the file editor's input is undocumented, so --edit is
// refused.
func buildOpenHands(event, matcher, root string, in Input) (Payload, error) {
	if err := checkInput("openhands", event, "PreToolUse", "PostToolUse", "UserPromptSubmit", in); err != nil {
		return Payload{}, err
	}
	if in.Edit != "" {
		return Payload{}, Unbuilt{"OpenHands documents no tool_input for its file editor; pass --payload <file>"}
	}
	doc := map[string]any{"event_type": event, "session_id": SessionID, "working_dir": root, "metadata": map[string]any{}}
	p := Payload{}
	switch event {
	case "UserPromptSubmit":
		p.Trigger = "prompt"
		doc["message"] = in.Prompt
	case "SessionStart":
		p.Trigger = "session"
	default:
		p.Trigger = "terminal"
		doc["tool_name"] = p.Trigger
		doc["tool_input"] = map[string]any{"command": in.Bash}
		if event == "PostToolUse" {
			doc["tool_response"] = map[string]any{}
		}
	}
	var err error
	p.Fires, err = openHandsMatches(matcher, doc["tool_name"])
	if err != nil {
		return Payload{}, err
	}
	return marshal(p, doc)
}

var regexMeta = "|.*+?[]()^$\\"

// openHandsMatches follows HookMatcher.matches: `*` or empty match all; a
// matcher on an event with no tool runs only then; `/re/` or a matcher
// with a regex metacharacter must match the whole tool name; anything
// else is an exact name.
func openHandsMatches(matcher string, tool any) (bool, error) {
	if matcher == "" || matcher == "*" {
		return true, nil
	}
	name, ok := tool.(string)
	if !ok {
		return false, nil
	}
	if len(matcher) > 2 && strings.HasPrefix(matcher, "/") && strings.HasSuffix(matcher, "/") {
		re, err := regexp.Compile("^(?:" + matcher[1:len(matcher)-1] + ")$")
		return err == nil && re.MatchString(name), nil
	}
	if strings.ContainsAny(matcher, regexMeta) {
		if re, err := regexp.Compile("^(?:" + matcher + ")$"); err == nil {
			return re.MatchString(name), nil
		}
	}
	return matcher == name, nil
}

// gooseShellEvents and gooseFileEvents match on the command and the file
// path instead of the tool name.
var (
	gooseShellEvents = []string{"BeforeShellExecution", "AfterShellExecution"}
	gooseFileEvents  = []string{"BeforeReadFile", "AfterFileEdit"}
)

// buildGoose writes Goose's payload. `matcher_context` is what the
// matcher, an unanchored regular expression, is tested against; a matcher
// that does not compile, such as a bare `*`, skips the rule.
func buildGoose(event, matcher, root string, in Input) (Payload, error) {
	doc := map[string]any{"event": event, "session_id": SessionID}
	p := Payload{}
	switch {
	case slices.Contains(gooseShellEvents, event):
		if in.Bash == "" {
			return Payload{}, fmt.Errorf("%s needs --bash <command> or --payload <file>", event)
		}
		p.Trigger = in.Bash
		doc["tool_name"], doc["tool_input"], doc["working_dir"] = "shell", map[string]any{"command": in.Bash}, root
	case slices.Contains(gooseFileEvents, event):
		if in.Edit == "" {
			return Payload{}, fmt.Errorf("%s needs --edit <path> or --payload <file>", event)
		}
		p.Trigger = absPath(root, in.Edit)
		doc["working_dir"] = root
	default:
		if err := checkInput("goose", event, "PreToolUse", "PostToolUse", "UserPromptSubmit", in); err != nil {
			return Payload{}, err
		}
		switch event {
		case "UserPromptSubmit":
			p.Trigger = in.Prompt
			doc["message"] = in.Prompt
		case "SessionStart":
		default:
			doc["tool_call_id"], doc["working_dir"] = toolUseID, root
			if in.Bash != "" {
				p.Trigger = "shell"
				doc["tool_input"] = map[string]any{"command": in.Bash}
			} else {
				var err error
				if p.Trigger, _, err = firstMatch(gooseMatches, matcher, []string{"write", "edit"}); err != nil {
					return Payload{}, err
				}
				input := map[string]any{"path": absPath(root, in.Edit)}
				if p.Trigger == "write" {
					input["content"] = ""
				} else {
					input["before"], input["after"] = "", ""
				}
				doc["tool_input"] = input
			}
			doc["tool_name"] = p.Trigger
		}
	}
	doc["matcher_context"] = p.Trigger
	var err error
	p.Fires, err = gooseMatches(matcher, p.Trigger)
	if err != nil {
		return Payload{}, err
	}
	if p.Trigger == "" {
		p.Trigger = "session"
	}
	return marshal(p, doc)
}

func gooseMatches(matcher, value string) (bool, error) {
	if matcher == "" {
		return true, nil
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return false, nil
	}
	return re.MatchString(value), nil
}

var augmentInputlessEvents = []string{"Stop", "SessionEnd", "Notification"}

// buildAugment writes Auggie 1.x's payload. Edits go through edit or
// write with paths relative to the workspace root, and file_changes on
// PostToolUse. --prompt builds PromptSubmit.
func buildAugment(event, matcher, root string, in Input) (Payload, error) {
	if err := checkAugmentInput(event, in); err != nil {
		return Payload{}, err
	}
	doc := map[string]any{
		"hook_event_name": event, "hook_type": event, "cwd": root,
		"conversation_id": SessionID, "workspace_roots": []string{root},
	}
	p := Payload{Fires: true, Trigger: "session"}
	switch event {
	case "SessionStart", "SessionEnd":
		return marshal(p, doc)
	case "PromptSubmit":
		if in.Prompt == "" {
			return Payload{}, fmt.Errorf("%s needs --prompt <text> or --payload <file>", event)
		}
		p.Trigger = "prompt"
		doc["user_prompt"] = in.Prompt
		return marshal(p, doc)
	case "Stop":
		p.Trigger = "stop"
		doc["agent_stop_cause"] = "end_turn"
		return marshal(p, doc)
	case "Notification":
		p.Trigger = "notification"
		doc["notification_type"], doc["notification_message"] = "info", ""
		return marshal(p, doc)
	}
	var input map[string]any
	var err error
	if in.Bash != "" {
		p.Trigger = "bash"
		p.Fires, err = augmentToolMatches(matcher, p.Trigger)
		input = map[string]any{"command": in.Bash}
	} else {
		path := relPath(root, in.Edit)
		p.Trigger, p.Fires, err = firstMatch(augmentToolMatches, matcher, []string{"edit", "write"})
		input = map[string]any{"path": path}
		if p.Trigger == "edit" {
			input["edits"] = []map[string]any{{"oldText": "", "newText": ""}}
		}
		if event == "PostToolUse" {
			doc["file_changes"] = []map[string]any{{"path": path, "changeType": "edit", "content": "", "oldContent": ""}}
		}
	}
	if err != nil {
		return Payload{}, err
	}
	doc["tool_name"], doc["tool_input"], doc["is_mcp_tool"] = p.Trigger, input, false
	if event == "PostToolUse" {
		doc["tool_output"], doc["tool_error"] = "", nil
		doc["tool_result"], doc["tool_is_error"] = []any{}, false
	}
	return marshal(p, doc)
}

// augmentLegacyTools are the Auggie 0.x names that 1.x matchers still
// match for its renamed tools.
var augmentLegacyTools = map[string]string{
	"bash": "launch-process", "read": "view", "edit": "str-replace-editor", "write": "save-file",
}

// augmentToolMatches follows Auggie 1.x for built-in tools: the matcher
// is a case-insensitive unanchored regular expression, and it also
// matches a tool through its 0.x name. The `mcp:` prefix rules are not
// modeled.
func augmentToolMatches(matcher, value string) (bool, error) {
	matcher = strings.TrimSpace(matcher)
	if matcher == "" || matcher == "*" {
		return true, nil
	}
	names := []string{value}
	if legacy := augmentLegacyTools[strings.ToLower(value)]; legacy != "" {
		names = append(names, legacy)
	}
	re, err := regexp.Compile("(?i)" + matcher)
	return slices.ContainsFunc(names, func(name string) bool {
		if err != nil {
			return strings.EqualFold(matcher, name)
		}
		return re.MatchString(name)
	}), nil
}

func checkAugmentInput(event string, in Input) error {
	if !slices.Contains(augmentInputlessEvents, event) {
		return checkInput("augment", event, "PreToolUse", "PostToolUse", "PromptSubmit", in)
	}
	switch {
	case in.Edit != "" || in.Bash != "":
		return fmt.Errorf("--edit and --bash build tool events, not %s", event)
	case in.Prompt != "":
		return fmt.Errorf("--prompt builds PromptSubmit, not %s", event)
	}
	return nil
}
