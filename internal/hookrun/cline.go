package hookrun

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"
)

// Source: cline/cline 39ff2359f7e08231281539696e48a166ce49270c, the SDK
// file-hook runtime the Cline CLI and other SDK hosts run
// (sdk/packages/core/src/hooks). The VS Code extension turns it off
// (apps/vscode/src/sdk/vscode-session-host.ts:171-178) and runs only
// extensionless executables under .clinerules/hooks
// (apps/vscode/src/core/hooks/hook-factory.ts:1022-1033), so it never runs
// the .cline/hooks/<Event>.sh scripts sync writes. Read from that source:
//   - launch: a .sh script with no shebang runs as `bash <file>`, with no
//     shell parsing of the path (hook-file-hooks.ts:356-368,
//     subprocess-runner.ts:193-201), in the process env.
//   - stdin payload: the event as JSON (subprocess-runner.ts:358): the
//     base fields (hook-file-hooks.ts:234-254) plus tool_call (:842-869),
//     tool_result (:871-900), or prompt_submit (:819-840).
//   - timeout: 120 s on PreToolUse and PostToolUse (`toolCallTimeoutMs ??
//     120000`, :853, :882), which no host sets
//     (services/local-runtime-bootstrap.ts:425-436), then SIGKILL
//     (subprocess-runner.ts:287-292).
//   - async: every other event runs detached with stdout ignored, so it
//     cannot block (hook-file-hooks.ts:456-512, :1037-1048).
//   - replies: see readCline.
//   - matcher and timeout: an event script has neither, so sync drops
//     both and the script runs on every occurrence of its event.
//
// Assumed: the working directory. Cline runs a hook from the directory
// the CLI started in (local-runtime-bootstrap.ts:426,
// apps/cli/src/main.ts:864-865), and hook run uses the project root.
const clineSource = "https://github.com/cline/cline/tree/39ff2359f7e08231281539696e48a166ce49270c/sdk/packages/core/src/hooks"

const (
	clineTimeout       = 120 * time.Second
	clineControlPrefix = "HOOK_CONTROL\t"
	clineAgentID       = "hook-run-agent"
	clineCwdReason     = "Cline runs a hook from the directory the CLI started in"
)

// clineBlockingEvents are the events Cline waits on and reads stdout for
// (hook-file-hooks.ts:1037-1048).
var clineBlockingEvents = []string{"PreToolUse", "PostToolUse"}

// clineAssumptions runs every script with bash, which the source names,
// from an assumed working directory.
func clineAssumptions(goos string, _ Handler) ([]Assumption, string) {
	if goos == "windows" {
		return nil, "Cline runs hooks with bash; hook run does not assume which bash Windows resolves"
	}
	return []Assumption{{Item: "working directory", Value: "project root", Reason: clineCwdReason}}, ""
}

// clineInert is why hook run cannot run a Cline script for event, or ""
// when it can. PreCompact is a file name Cline lists but maps to no
// runtime event (hook-file-config.ts:41, hook-file-hooks.ts:404-406).
func clineInert(event string) string {
	if event == "PreCompact" {
		return "Cline has no PreCompact hook event"
	}
	return ""
}

// buildCline writes the SDK payload. --bash calls run_commands
// (extensions/tools/definitions.ts:517, schemas.ts:149-153) and --edit
// calls editor with an absolute path (definitions.ts:706,
// schemas.ts:194-224). preToolUse and postToolUse repeat the input as
// parameters, a string map (hook-file-hooks.ts:118-127). Cline has no
// matcher, so every call fires.
func buildCline(event, _, root string, in Input) (Payload, error) {
	if err := checkInput("cline", event, "PreToolUse", "PostToolUse", "UserPromptSubmit", in); err != nil {
		return Payload{}, err
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	doc := map[string]any{
		"clineVersion": "", "timestamp": now, "taskId": SessionID,
		"sessionContext": map[string]any{"rootSessionId": SessionID},
		"workspaceRoots": []string{root}, "userId": clineUserID(),
		"agent_id": clineAgentID, "parent_agent_id": nil,
	}
	p := Payload{Fires: true}
	if event == "UserPromptSubmit" {
		p.Trigger = "prompt"
		doc["hookName"] = "prompt_submit"
		doc["userPromptSubmit"] = map[string]any{"prompt": in.Prompt, "attachments": []any{}}
		return clineMarshal(p, doc)
	}
	input := map[string]any{"commands": []string{in.Bash}}
	p.Trigger = "run_commands"
	if in.Edit != "" {
		input = map[string]any{"path": absPath(root, in.Edit), "new_text": ""}
		p.Trigger = "editor"
	}
	params, err := clineParams(input)
	if err != nil {
		return Payload{}, err
	}
	call := map[string]any{"id": toolUseID, "name": p.Trigger, "input": input}
	doc["iteration"] = 1
	if event == "PreToolUse" {
		doc["hookName"] = "tool_call"
		doc["tool_call"] = call
		doc["preToolUse"] = map[string]any{"toolName": p.Trigger, "parameters": params}
		return clineMarshal(p, doc)
	}
	call["output"], call["durationMs"], call["startedAt"], call["endedAt"] = "", 0, now, now
	doc["hookName"] = "tool_result"
	doc["tool_result"] = call
	doc["postToolUse"] = map[string]any{"toolName": p.Trigger, "parameters": params, "result": "", "success": true, "executionTimeMs": 0}
	return clineMarshal(p, doc)
}

// clineUserID is the userId Cline sends: CLINE_USER_ID, else USER, else
// "unknown" (hook-file-hooks.ts:238-239).
func clineUserID() string {
	for _, key := range []string{"CLINE_USER_ID", "USER"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v
		}
	}
	return "unknown"
}

// clineParams maps a tool input the way mapParams does: a string stays
// as is, and anything else becomes its JSON text.
func clineParams(input map[string]any) (map[string]string, error) {
	out := make(map[string]string, len(input))
	for key, value := range input {
		if s, ok := value.(string); ok {
			out[key] = s
			continue
		}
		text, err := clineJSON(value)
		if err != nil {
			return nil, err
		}
		out[key] = string(text)
	}
	return out, nil
}

// clineJSON encodes v as JSON.stringify does: no HTML escaping and no
// trailing newline.
func clineJSON(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	err := enc.Encode(v)
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), err
}

func clineMarshal(p Payload, doc map[string]any) (Payload, error) {
	var err error
	p.Body, err = clineJSON(doc)
	return p, err
}

// clineRawPayload names the tool a --payload tool call reports. Cline has
// no matcher, so the hook always fires.
func clineRawPayload(p Payload) Payload {
	var call struct {
		ToolCall   struct{ Name string } `json:"tool_call"`
		ToolResult struct{ Name string } `json:"tool_result"`
	}
	_ = json.Unmarshal(p.Body, &call)
	p.Trigger = cmp.Or(call.ToolCall.Name, call.ToolResult.Name, p.Trigger)
	return p
}

// clineControl reads stdout as parseStdout does
// (subprocess-runner.ts:68-97): the last trimmed line that starts with
// `HOOK_CONTROL<TAB>`, else the whole trimmed stdout, is JSON. It returns
// nil for empty stdout.
func clineControl(stdout string) (any, error) {
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, nil
	}
	candidate := trimmed
	for _, line := range strings.Split(trimmed, "\n") {
		if line = strings.TrimSpace(line); strings.HasPrefix(line, clineControlPrefix) {
			candidate = strings.TrimPrefix(line, clineControlPrefix)
		}
	}
	var value any
	err := json.Unmarshal([]byte(candidate), &value)
	return value, err
}

// clineRead is what hook run reads from one Cline result.
type clineRead struct {
	decision Decision
	notes    []string
	context  bool
}

// readCline follows runBlockingHookCommands (hook-file-hooks.ts:415-454),
// which never reads the exit code. A JSON object with `cancel: true`
// stops the run (parseHookControl, :191-223; agents/src/agent-runtime.ts:
// 2408, 2718-2728). Empty stdout or any other JSON allows. A timeout,
// stdout that is not JSON, and a hook that did not start are logged, and
// Cline goes on as if the hook allowed. A reply that does not cancel adds
// its `context`, else `contextModification`, else `errorMessage`, to the
// session (:577-626). Nothing a detached hook does reaches Cline.
func readCline(event string, r Result) clineRead {
	blocking := slices.Contains(clineBlockingEvents, event)
	switch {
	case r.TimedOut && blocking:
		return clineRead{decision: Timeout, notes: []string{"Cline logs a hook that times out and goes on as if it allowed"}}
	case r.TimedOut:
		return clineRead{decision: Timeout}
	case r.StartErr != nil && blocking:
		return clineRead{decision: Error, notes: []string{"Cline logs a hook that does not start and goes on as if it allowed"}}
	case r.StartErr != nil:
		return clineRead{decision: Error}
	case !blocking:
		return clineRead{decision: Allow}
	}
	read := clineRead{decision: Allow}
	value, err := clineControl(r.Stdout)
	reply, _ := value.(map[string]any)
	cancel := reply["cancel"] == true
	switch {
	case err != nil:
		read = clineRead{decision: Error, notes: []string{"stdout is not JSON: Cline logs it and goes on as if the hook allowed"}}
	case cancel && event == "PostToolUse":
		read = clineRead{decision: Block, notes: []string{"cancel: the tool already ran; Cline stops the run"}}
	case cancel:
		read = clineRead{decision: Block, notes: []string{"cancel: Cline skips the tool call and stops the run"}}
	default:
		read.context = strings.TrimSpace(clineContext(reply)) != ""
	}
	if r.Exit != 0 && !cancel {
		read.notes = append(read.notes, `Cline ignores the exit code; print {"cancel": true} to block`)
	}
	return read
}

// clineContext is the text a reply adds to the session.
func clineContext(reply map[string]any) string {
	for _, key := range []string{"context", "contextModification"} {
		if text, ok := reply[key].(string); ok {
			return text
		}
	}
	text, _ := reply["errorMessage"].(string)
	return text
}

// ClineNotes explains a Cline result: what Cline does with a failure, a
// cancel, or an exit code it ignores.
func ClineNotes(event string, r Result) []string {
	return readCline(event, r).notes
}

// ClineRunNotes names what Cline does not take from the spec or the
// payload hook run builds: the matcher and timeout sync drops, the tool
// some models call instead of editor, and a prompt event the CLI may not
// send.
func ClineRunNotes(event, matcher string, timeout time.Duration, trigger string) []string {
	var notes []string
	blocking := slices.Contains(clineBlockingEvents, event)
	if matcher != "" {
		occurrence := event + " event"
		if blocking {
			occurrence = "tool call"
		}
		notes = append(notes, fmt.Sprintf("Cline has no matcher: sync drops %q, and the script runs on every %s", matcher, occurrence))
	}
	if timeout > 0 {
		note := fmt.Sprintf("Cline has no per-hook timeout: sync drops the spec's %gs", timeout.Seconds())
		if blocking {
			note += fmt.Sprintf(", and hook run uses Cline's %gs", clineTimeout.Seconds())
		}
		notes = append(notes, note)
	}
	if trigger == "editor" {
		// extensions/tools/model-tool-routing.ts:60-75.
		notes = append(notes, "Cline calls apply_patch instead of editor for a model whose id holds gpt or codex")
	}
	if event == "UserPromptSubmit" {
		// hook-file-hooks.ts:1011-1017.
		notes = append(notes, "Cline may not send this event: its source says orchestrated sessions, which the CLI runs, seed the prompt without it")
	}
	return notes
}

// clineDrift names each handler whose commands the synced event script
// does not hold. Sync writes a prologue, a blank line, then each command
// with a blank line before it, and joins every spec on the event in one
// script.
func clineDrift(body []byte, event string, handlers []Handler) []HandlerDrift {
	var drift []HandlerDrift
	for _, h := range handlers {
		prologue, commands, _ := strings.Cut(h.Script, "\n\n")
		if !bytes.Contains(body, []byte(prologue+"\n\n")) || !bytes.Contains(body, []byte("\n\n"+commands)) {
			drift = append(drift, HandlerDrift{Handler: h, Reason: fmt.Sprintf("does not run this spec's %s commands", event)})
		}
	}
	return drift
}
