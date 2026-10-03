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
	"time"
)

// Targets lists the targets Build writes payloads for.
func Targets() []string {
	return []string{"claude", "codex", "gemini", "trae", "openhands", "goose", "augment", "cursor", "crush", "copilot", "factory", "qoder", "antigravity", "kiro", "windsurf", "cline"}
}

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

// SessionID is the session_id every payload carries.
const SessionID = "agnostic-ai-hook-run"

const (
	toolUseID = "hook-run-tool-use"
	turnID    = "hook-run-turn"
)

// vocabulary is what one target calls the events Build writes, and the
// SessionStart sources it documents, in the order a matcher is tried.
type vocabulary struct {
	pre, post, prompt string
	sources           []string
}

var vocabularies = map[string]vocabulary{
	"claude": {"PreToolUse", "PostToolUse", "UserPromptSubmit", []string{"startup", "resume", "clear", "compact"}},
	"codex":  {"PreToolUse", "PostToolUse", "UserPromptSubmit", []string{"startup", "resume", "clear", "compact"}},
	// gemini-cli c6bccb7 packages/core/src/hooks/types.ts SessionStartSource.
	"gemini": {"BeforeTool", "AfterTool", "BeforeAgent", []string{"startup", "resume", "clear"}},
}

// Build writes target's payload for event, from the fields each vendor
// documents: Claude Code (code.claude.com/docs/en/hooks), Codex
// (learn.chatgpt.com/docs/hooks), and Gemini CLI (gemini-cli c6bccb7,
// packages/core/src/hooks/hookEventHandler.ts). tool_response carries
// placeholder values.
func Build(target, event, matcher, root string, in Input) (Payload, error) {
	if !Supported(target) {
		return Payload{}, fmt.Errorf("hook run builds no %s payload", target)
	}
	if target == "qoder" && qoderUndecided(event) != "" {
		return Payload{}, Unbuilt{qoderUndecided(event)}
	}
	if target == "cline" && clineInert(event) != "" {
		return Payload{}, Unbuilt{clineInert(event)}
	}
	if target == "kiro" && kiroUndocumented(event) != "" {
		return Payload{}, Unbuilt{kiroUndocumented(event)}
	}
	if in.Raw != nil {
		return rawPayload(target, event, matcher, in.Raw)
	}
	if build, ok := otherBuilders[target]; ok {
		return build(event, matcher, root, in)
	}
	v := vocabularies[target]
	if err := checkInput(target, event, v.pre, v.post, v.prompt, in); err != nil {
		return Payload{}, err
	}
	doc := map[string]any{
		"session_id":      SessionID,
		"cwd":             root,
		"hook_event_name": event,
	}
	switch target {
	case "claude":
		doc["transcript_path"] = ""
		doc["permission_mode"] = "default"
	case "codex":
		doc["transcript_path"] = nil
		doc["model"] = ""
	case "gemini":
		doc["transcript_path"] = ""
		doc["timestamp"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	p := Payload{Fires: true}
	var err error
	switch event {
	case "SessionStart":
		p.Trigger, p.Fires, err = firstMatch(sourceMatcher(target), matcher, v.sources)
		doc["source"] = p.Trigger
	case v.prompt:
		p.Trigger = "prompt"
		doc["prompt"] = in.Prompt
	default:
		err = toolCall(doc, &p, target, event == v.post, matcher, root, in)
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

func rawPayload(target, event, matcher string, body []byte) (Payload, error) {
	p := Payload{Body: body, Fires: true, Trigger: "--payload"}
	if target == "goose" {
		var ctx struct {
			MatcherContext string `json:"matcher_context"`
		}
		_ = json.Unmarshal(body, &ctx)
		fires, err := gooseMatches(matcher, ctx.MatcherContext)
		p.Fires, p.Trigger = fires, ctx.MatcherContext
		if p.Trigger == "" {
			p.Trigger = "session"
		}
		return p, err
	}
	if target == "cursor" {
		var doc map[string]any
		_ = json.Unmarshal(body, &doc)
		value := cursorMatchValue(event, doc)
		if value == "" {
			return p, nil
		}
		fires, err := cursorMatches(matcher, value)
		p.Fires, p.Trigger = fires, value
		return p, err
	}
	if target == "crush" {
		p.Trigger = PayloadTool(body)
		fires, err := crushMatches(matcher, p.Trigger)
		p.Fires = fires
		return p, err
	}
	if target == "antigravity" {
		return antigravityRawPayload(event, matcher, p)
	}
	if target == "cline" {
		return clineRawPayload(p), nil
	}
	if target == "kiro" {
		return kiroRawPayload(event, matcher, p)
	}
	if target == "windsurf" {
		return windsurfRawPayload(event, matcher, p)
	}
	if target == "copilot" {
		var doc map[string]any
		_ = json.Unmarshal(body, &doc)
		fires, value, err := copilotMatches(event, matcher, doc)
		p.Fires = fires
		if value != "" {
			p.Trigger = value
		}
		return p, err
	}
	if !slices.Contains(claudeIfEvents, event) && event != "BeforeTool" && event != "AfterTool" && event != "PreToolUseResult" {
		return p, nil
	}
	p.Trigger = PayloadTool(body)
	match := toolMatcher(target)
	var err error
	if target == "codex" && p.Trigger == "apply_patch" {
		_, p.Fires, err = firstMatch(match, matcher, []string{"apply_patch", "Edit", "Write"})
	} else {
		p.Fires, err = match(matcher, p.Trigger)
	}
	return p, err
}

// checkInput rejects an input the event has no builder for. pre and post
// are the target's tool events and prompt its prompt event, "" when it
// has none.
func checkInput(target, event, pre, post, prompt string, in Input) error {
	isTool := event == pre || event == post
	switch {
	case !isTool && event != "SessionStart" && (prompt == "" || event != prompt):
		return fmt.Errorf("hook run builds no %s %s payload; pass --payload <file>", target, event)
	case !isTool && (in.Edit != "" || in.Bash != ""):
		return fmt.Errorf("--edit and --bash build tool events, not %s", event)
	case in.Prompt != "" && prompt == "":
		return fmt.Errorf("%s has no prompt event; --prompt does not apply", target)
	case in.Prompt != "" && event != prompt:
		return fmt.Errorf("--prompt builds %s, not %s", prompt, event)
	case isTool && in.Edit == "" && in.Bash == "":
		return fmt.Errorf("%s needs --edit <path>, --bash <command>, or --payload <file>", event)
	case in.Edit != "" && in.Bash != "":
		return errors.New("--edit and --bash are mutually exclusive")
	}
	return nil
}

type matchFunc func(matcher, value string) (bool, error)

func toolCall(doc map[string]any, p *Payload, target string, post bool, matcher, root string, in Input) error {
	match := toolMatcher(target)
	file := in.Edit
	if !filepath.IsAbs(file) {
		file = filepath.Join(root, file)
	}
	var input, response map[string]any
	var err error
	switch {
	case in.Bash != "" && target == "gemini":
		p.Trigger = "run_shell_command"
		p.Fires, err = match(matcher, p.Trigger)
		input = map[string]any{"command": in.Bash}
		response = map[string]any{"llmContent": "", "returnDisplay": ""}
	case in.Bash != "":
		p.Trigger = "Bash"
		p.Fires, err = match(matcher, "Bash")
		input = map[string]any{"command": in.Bash}
		response = map[string]any{"stdout": "", "stderr": "", "interrupted": false}
	case target == "claude":
		p.Trigger, p.Fires, err = firstMatch(match, matcher, []string{"Write", "Edit", "MultiEdit"})
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
	case target == "gemini":
		// EDIT_TOOL_NAMES in packages/core/src/tools/tool-names.ts.
		p.Trigger, p.Fires, err = firstMatch(match, matcher, []string{"write_file", "replace"})
		input = map[string]any{"file_path": file}
		if p.Trigger == "write_file" {
			input["content"] = ""
		} else {
			input["old_string"], input["new_string"] = "", ""
		}
		response = map[string]any{"llmContent": "", "returnDisplay": ""}
	default:
		// Codex reports every edit as apply_patch and takes Edit and
		// Write as matcher aliases for it.
		p.Trigger = "apply_patch"
		_, p.Fires, err = firstMatch(match, matcher, []string{"apply_patch", "Edit", "Write"})
		input = map[string]any{"command": patch(root, in.Edit)}
		response = map[string]any{}
	}
	if err != nil {
		return err
	}
	doc["tool_name"] = p.Trigger
	doc["tool_input"] = input
	if target != "gemini" {
		doc["tool_use_id"] = toolUseID
	}
	if post {
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
func firstMatch(match matchFunc, matcher string, candidates []string) (string, bool, error) {
	for _, c := range candidates {
		ok, err := match(matcher, c)
		if err != nil || ok {
			return c, ok, err
		}
	}
	return candidates[0], false, nil
}

var exactMatcher = regexp.MustCompile(`^[A-Za-z0-9_\-, |]+$`)

// matches follows Claude Code's matcher rules, which Codex shares: empty
// or `*` matches everything; letters, digits, `_`, `-`, spaces, `,`, and
// `|` list exact names separated by `|` or `,`; anything else is a
// regular expression.
func matches(matcher, value string) (bool, error) {
	if matcher == "" || matcher == "*" {
		return true, nil
	}
	if exactMatcher.MatchString(matcher) {
		names := strings.FieldsFunc(matcher, func(r rune) bool { return r == '|' || r == ',' })
		return slices.ContainsFunc(names, func(n string) bool { return strings.TrimSpace(n) == value }), nil
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return false, errors.Join(fmt.Errorf("matcher %q", matcher), err)
	}
	return re.MatchString(value), nil
}

func toolMatcher(target string) matchFunc {
	switch target {
	case "gemini", "trae", "augment":
		return geminiToolMatches
	case "openhands":
		return func(matcher, value string) (bool, error) { return openHandsMatches(matcher, value) }
	}
	return matches
}

func sourceMatcher(target string) matchFunc {
	if target == "gemini" {
		return geminiSourceMatches
	}
	return matches
}

// geminiToolMatches follows gemini-cli c6bccb7 hookPlanner.ts: the
// trimmed matcher is an unanchored regular expression, and one that does
// not compile is compared as a literal name.
func geminiToolMatches(matcher, value string) (bool, error) {
	matcher = strings.TrimSpace(matcher)
	if matcher == "" || matcher == "*" {
		return true, nil
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return matcher == value, nil
	}
	return re.MatchString(value), nil
}

// geminiSourceMatches compares a lifecycle source exactly.
func geminiSourceMatches(matcher, value string) (bool, error) {
	matcher = strings.TrimSpace(matcher)
	return matcher == "" || matcher == "*" || matcher == value, nil
}
