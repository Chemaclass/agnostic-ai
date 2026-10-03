package hookrun

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// otherDefaultTimeouts are the documented defaults: Trae 30 seconds,
// OpenHands 60, Goose 30, Augment 60000 milliseconds, Factory 60, Qoder
// CLI 600, Antigravity 30, Kiro 60, and Cline 120 from its source. Cursor's and Devin
// CLI's are assumed.
var otherDefaultTimeouts = map[string]time.Duration{
	"trae":        30 * time.Second,
	"openhands":   60 * time.Second,
	"goose":       30 * time.Second,
	"augment":     60 * time.Second,
	"cursor":      cursorAssumedTimeout,
	"factory":     factoryDefaultTimeout,
	"copilot":     copilotDefaultTimeout,
	"qoder":       qoderDefaultTimeout,
	"antigravity": antigravityDefaultTimeout,
	"cline":       clineTimeout,
	"kiro":        kiroDefaultTimeout,
	"windsurf":    windsurfAssumedTimeout,
}

// otherArgv is how each target starts a command: Trae in Bash, or
// PowerShell on Windows; OpenHands through Python's shell=True, which is
// /bin/sh or cmd.exe; Goose with `sh -c` everywhere; Augment runs the
// script itself on Unix, and by extension on Windows.
func otherArgv(target, goos string, h Handler) ([]string, bool) {
	switch target {
	case "trae":
		if goos == "windows" {
			return []string{"powershell.exe", "-NoProfile", "-Command", h.Command}, true
		}
		return []string{"bash", "-c", h.Command}, true
	case "openhands":
		if goos == "windows" {
			return []string{"cmd.exe", "/c", h.Command}, true
		}
		return []string{"/bin/sh", "-c", h.Command}, true
	case "goose":
		return []string{"sh", "-c", h.Command}, true
	case "cursor", "factory", "antigravity", "kiro", "windsurf":
		// Assumed; see Assumptions.
		return []string{"sh", "-c", h.Command}, true
	case "copilot":
		if h.Exec {
			return append([]string{h.Command}, h.Args...), true
		}
		// Assumed; see Assumptions.
		return []string{"sh", "-c", h.Command}, true
	case "qoder":
		return qoderArgv(h), true
	case "cline":
		// bash -c with $0 as the script path runs it as `bash <file>` does, minus BASH_SOURCE, and needs no temp file.
		return []string{"bash", "-c", h.Script, h.Command}, true
	case "augment":
		switch ext := strings.ToLower(filepath.Ext(h.Command)); {
		case goos == "windows" && ext == ".ps1":
			return []string{"powershell.exe", "-Command", h.Command}, true
		case goos == "windows" && (ext == ".cmd" || ext == ".bat"):
			return []string{"cmd.exe", "/c", h.Command}, true
		}
		return []string{h.Command}, true
	}
	return nil, false
}

// DecideHandler is Decide for one handler, whose own failure policy can
// turn a failed run into a block.
func DecideHandler(target, event string, h Handler, r Result) Decision {
	switch target {
	case "openhands":
		return decideOpenHands(event, r)
	case "goose":
		return decideGoose(event, h, r)
	case "augment":
		return decideAugment(event, r)
	case "cursor":
		return decideCursor(event, h, r)
	case "crush":
		return decideCrush(r)
	case "factory":
		return decideFactory(event, r)
	case "copilot":
		return decideCopilot(event, r)
	case "qoder":
		return decideQoder(event, r)
	case "antigravity":
		return readAntigravity(event, r).decision
	case "cline":
		return readCline(event, r).decision
	case "kiro":
		return readKiro(event, r).decision
	case "windsurf":
		return readWindsurf(event, r).decision
	}
	return Decide(target, event, r)
}

// decideOpenHands follows executor.py: exit 2 blocks, and so does a JSON
// reply with decision deny or continue false, whatever the exit code.
// Only PreToolUse, UserPromptSubmit, and Stop act on a block (manager.py).
func decideOpenHands(event string, r Result) Decision {
	switch {
	case r.TimedOut:
		return Timeout
	case r.StartErr != nil:
		return Error
	}
	var reply struct {
		Decision any   `json:"decision"`
		Continue *bool `json:"continue"`
	}
	blocks := r.Exit == 2
	if json.Unmarshal([]byte(strings.TrimSpace(r.Stdout)), &reply) == nil {
		if d, ok := reply.Decision.(string); ok && strings.EqualFold(d, "deny") {
			blocks = true
		}
		if reply.Continue != nil && !*reply.Continue {
			blocks = true
		}
	}
	switch {
	case blocks && slices.Contains([]string{"PreToolUse", "UserPromptSubmit", "Stop"}, event):
		return Block
	case r.Exit != 0:
		return Error
	}
	return Allow
}

// decideGoose follows the decision order of the Goose hooks guide: exit
// 2 blocks; stdout starting with `{` whose decision is "block" blocks
// whatever the exit; exit 0 with empty stdout or decision "allow"
// allows; anything else is no decision, which fails open unless the
// action sets on_failure block. Only PreToolUse and Stop can block, and
// on_failure applies to PreToolUse only.
func decideGoose(event string, h Handler, r Result) Decision {
	blocking := event == "PreToolUse" || event == "Stop"
	out := strings.TrimSpace(r.Stdout)
	var reply struct {
		Decision string `json:"decision"`
	}
	isJSON := strings.HasPrefix(out, "{") && json.Unmarshal([]byte(out), &reply) == nil
	failed := Error
	if h.FailClosed && event == "PreToolUse" {
		failed = Block
	}
	switch {
	case r.TimedOut && failed == Block:
		return Block
	case r.TimedOut:
		return Timeout
	case r.StartErr != nil:
		return failed
	case blocking && r.Exit == 2:
		return Block
	case blocking && isJSON && reply.Decision == "block":
		return Block
	case r.Exit == 0 && (out == "" || isJSON && reply.Decision == "allow"):
		return Allow
	case !blocking && r.Exit == 0:
		return Allow
	case !blocking:
		return Error
	}
	return failed
}

// decideAugment follows the Augment hooks reference: exit 2 blocks on
// PreToolUse only, another non-zero exit is a non-blocking error, and on
// exit 0 permissionDecision deny, hookSpecificOutput.decision block, or
// continue false blocks.
func decideAugment(event string, r Result) Decision {
	switch {
	case r.TimedOut:
		return Timeout
	case r.StartErr != nil:
		return Error
	case r.Exit == 2 && event == "PreToolUse":
		return Block
	case r.Exit != 0:
		return Error
	}
	// Stop and PostToolUse nest their decision in hookSpecificOutput.
	reply, ok := readReply(r)
	if ok && (reply.HookSpecificOutput.PermissionDecision == "deny" || reply.HookSpecificOutput.Decision == "block" ||
		reply.Decision == "block" || (reply.Continue != nil && !*reply.Continue)) {
		return Block
	}
	return Allow
}

// PayloadTool is the tool_name a payload names, "" for an event with no
// tool call.
func PayloadTool(body []byte) string {
	var call struct {
		ToolName string `json:"tool_name"`
	}
	_ = json.Unmarshal(body, &call)
	return call.ToolName
}

// PayloadPrompt is the prompt a payload carries, "" when it has none.
func PayloadPrompt(body []byte) string {
	var doc struct {
		Prompt string `json:"prompt"`
	}
	_ = json.Unmarshal(body, &doc)
	return doc.Prompt
}

// HandlersFromDoc reads the command handlers out of the hooks file sync
// writes for one spec, in the `hooks` shape Trae, OpenHands, Goose, and
// Augment share, or the top-level one of Factory and Devin CLI.
func HandlersFromDoc(target string, body []byte) ([]Handler, error) {
	if len(body) == 0 {
		return nil, nil
	}
	events, err := nativeHooks(target, body)
	if err != nil {
		return nil, err
	}
	var out []Handler
	for _, groups := range events {
		for _, g := range groups {
			for _, n := range g.Hooks {
				if n.Type != "" && n.Type != "command" {
					continue
				}
				out = append(out, Handler{Command: n.Command, Timeout: n.timeout(target), FailClosed: n.OnFailure == "block"})
			}
		}
	}
	return out, nil
}
