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

type builder func(event, matcher, root string, in Input) (Payload, error)

var otherBuilders = map[string]builder{
	"trae":      buildTrae,
	"openhands": buildOpenHands,
	"goose":     buildGoose,
	"augment":   buildAugment,
	"cursor":    buildCursor,
	"factory":   buildFactory,
	"copilot":   buildCopilot,
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

// buildAugment writes Auggie's payload: no prompt event, edits through
// str-replace-editor or save-file with paths relative to the workspace
// root, and file_changes on PostToolUse.
func buildAugment(event, matcher, root string, in Input) (Payload, error) {
	if err := checkInput("augment", event, "PreToolUse", "PostToolUse", "", in); err != nil {
		return Payload{}, err
	}
	doc := map[string]any{"hook_event_name": event, "conversation_id": SessionID, "workspace_roots": []string{root}}
	p := Payload{Fires: true, Trigger: "session"}
	if event == "SessionStart" {
		return marshal(p, doc)
	}
	var input map[string]any
	var err error
	if in.Bash != "" {
		p.Trigger = "launch-process"
		p.Fires, err = unanchoredMatches(matcher, p.Trigger)
		input = map[string]any{"command": in.Bash}
	} else {
		path := relPath(root, in.Edit)
		p.Trigger, p.Fires, err = firstMatch(unanchoredMatches, matcher, []string{"str-replace-editor", "save-file"})
		input = map[string]any{"path": path}
		if p.Trigger == "str-replace-editor" {
			input["old_str_1"], input["new_str_1"] = "", ""
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
	}
	return marshal(p, doc)
}

// AugmentRuns reports whether Auggie runs command at all: "Path to the
// script to execute (must use a supported script extension: .ps1, .cmd,
// .bat, or .sh)".
func AugmentRuns(command string) bool {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(command))) {
	case ".sh", ".ps1", ".cmd", ".bat":
		return true
	}
	return false
}
