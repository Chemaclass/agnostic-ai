// Package hookrun runs one hook command the way a target would: with
// that target's payload on stdin, its shell, and its timeout, and reads
// what the target does with the result.
package hookrun

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Targets lists the targets Build writes payloads for.
func Targets() []string { return []string{"claude", "codex"} }

// Supported reports whether Build writes payloads for target.
func Supported(target string) bool { return slices.Contains(Targets(), target) }

// Input is what the caller says the event is about. Raw, when set, is
// the payload itself.
type Input struct {
	Edit   string
	Bash   string
	Prompt string
	Raw    []byte
}

// Payload is one target's event JSON. Fires is false when the hook's
// matcher does not match what the event reports, so the target would
// not run it; Trigger names that tool or source.
type Payload struct {
	Body    []byte
	Fires   bool
	Trigger string
}

const (
	sessionID = "agnostic-ai-hook-run"
	toolUseID = "hook-run-tool-use"
	turnID    = "hook-run-turn"
)

var toolEvents = []string{"PreToolUse", "PostToolUse"}

// sessionSources are the SessionStart sources Claude Code and Codex
// both document, in the order a matcher is tried against them.
var sessionSources = []string{"startup", "resume", "clear", "compact"}

// Build writes target's payload for event. Both Claude Code
// (code.claude.com/docs/en/hooks) and Codex (learn.chatgpt.com/docs/hooks)
// document the fields; tool_response carries placeholder values.
func Build(target, event, matcher, root string, in Input) (Payload, error) {
	if !Supported(target) {
		return Payload{}, fmt.Errorf("hook run builds no %s payload", target)
	}
	if in.Raw != nil {
		return Payload{Body: in.Raw, Fires: true, Trigger: "--payload"}, nil
	}
	isTool := slices.Contains(toolEvents, event)
	switch {
	case !isTool && event != "SessionStart" && event != "UserPromptSubmit":
		return Payload{}, fmt.Errorf("hook run builds no %s payload; pass --payload <file>", event)
	case !isTool && (in.Edit != "" || in.Bash != ""):
		return Payload{}, fmt.Errorf("--edit and --bash build tool events, not %s", event)
	case in.Prompt != "" && event != "UserPromptSubmit":
		return Payload{}, fmt.Errorf("--prompt builds UserPromptSubmit, not %s", event)
	case isTool && in.Edit == "" && in.Bash == "":
		return Payload{}, fmt.Errorf("%s needs --edit <path>, --bash <command>, or --payload <file>", event)
	case in.Edit != "" && in.Bash != "":
		return Payload{}, errors.New("--edit and --bash are mutually exclusive")
	}
	doc := map[string]any{
		"session_id":      sessionID,
		"cwd":             root,
		"hook_event_name": event,
	}
	if target == "claude" {
		doc["transcript_path"] = ""
		doc["permission_mode"] = "default"
	} else {
		doc["transcript_path"] = nil
		doc["model"] = ""
	}
	p := Payload{Fires: true}
	var err error
	switch event {
	case "SessionStart":
		p.Trigger, p.Fires, err = firstMatch(matcher, sessionSources)
		doc["source"] = p.Trigger
	case "UserPromptSubmit":
		p.Trigger = "prompt"
		doc["prompt"] = in.Prompt
	default:
		err = toolCall(doc, &p, target, event, matcher, root, in)
	}
	if err != nil {
		return Payload{}, err
	}
	if target == "codex" && event != "SessionStart" {
		doc["turn_id"] = turnID
	}
	p.Body, err = json.Marshal(doc)
	return p, err
}

func toolCall(doc map[string]any, p *Payload, target, event, matcher, root string, in Input) error {
	var input, response map[string]any
	var err error
	switch {
	case in.Bash != "":
		p.Trigger = "Bash"
		p.Fires, err = matches(matcher, "Bash")
		input = map[string]any{"command": in.Bash}
		response = map[string]any{"stdout": "", "stderr": "", "interrupted": false}
	case target == "claude":
		file := in.Edit
		if !filepath.IsAbs(file) {
			file = filepath.Join(root, file)
		}
		var fires bool
		p.Trigger, fires, err = firstMatch(matcher, []string{"Write", "Edit", "MultiEdit"})
		p.Fires = fires
		input = map[string]any{"file_path": file}
		switch p.Trigger {
		case "Write":
			input["content"] = ""
		case "Edit":
			input["old_string"], input["new_string"] = "", ""
		case "MultiEdit":
			input["edits"] = []any{}
		}
		response = map[string]any{"filePath": file, "success": true}
	default:
		// Codex reports every edit as apply_patch and takes Edit and
		// Write as matcher aliases for it.
		p.Trigger = "apply_patch"
		_, p.Fires, err = firstMatch(matcher, []string{"apply_patch", "Edit", "Write"})
		input = map[string]any{"command": patch(root, in.Edit)}
		response = map[string]any{}
	}
	if err != nil {
		return err
	}
	doc["tool_name"] = p.Trigger
	doc["tool_input"] = input
	doc["tool_use_id"] = toolUseID
	if event == "PostToolUse" {
		doc["tool_response"] = response
	}
	return nil
}

// patch adds a file that does not exist yet and updates one that does.
func patch(root, path string) string {
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, abs)
	}
	header := "*** Add File: "
	if _, err := os.Stat(abs); err == nil {
		header = "*** Update File: "
	}
	if rel, err := filepath.Rel(root, abs); err == nil && !strings.HasPrefix(rel, "..") {
		path = rel
	}
	return "*** Begin Patch\n" + header + filepath.ToSlash(path) + "\n+\n*** End Patch\n"
}

// firstMatch returns the first candidate matcher matches, or the first
// candidate and false when it matches none.
func firstMatch(matcher string, candidates []string) (string, bool, error) {
	for _, c := range candidates {
		ok, err := matches(matcher, c)
		if err != nil || ok {
			return c, ok, err
		}
	}
	return candidates[0], false, nil
}

var exactMatcher = regexp.MustCompile(`^[A-Za-z0-9_|]+$`)

// matches follows Claude Code's matcher rules, which Codex shares: empty
// or `*` matches everything, letters, digits, `_`, and `|` list exact
// names, and anything else is a regular expression.
func matches(matcher, value string) (bool, error) {
	if matcher == "" || matcher == "*" {
		return true, nil
	}
	if exactMatcher.MatchString(matcher) {
		return slices.Contains(strings.Split(matcher, "|"), value), nil
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return false, errors.Join(fmt.Errorf("matcher %q", matcher), err)
	}
	return re.MatchString(value), nil
}
