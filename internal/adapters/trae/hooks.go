package trae

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/claudehooks"
	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// defaultHooksFile is Trae's project hook file: "Project Hook |
// `$PROJECT_FOLDER/.trae/hooks.json` | Applies only to the current
// project or workspace" (docs.trae.ai/ide/hook-configuration-reference).
const defaultHooksFile = ".trae/hooks.json"

// traeToolNames is the `tool_name` vocabulary a PreToolUse or
// PostToolUse matcher is tested against, from the hook reference's own
// "Tools supported by PreToolUse and PostToolUse events" table. It is
// not Trae's subagent `tools` vocabulary (see the package doc): the two
// tables share Read, Write, Edit, Glob, Grep, WebFetch, WebSearch and
// Skill, but the terminal tool is `RunCommand` here and `Bash` there,
// and `TodoWrite` exists only on the subagent side. MCP tools are
// matched as `mcp__<serverName>__<toolName>`, which no literal entry
// can enumerate, so the prefix is checked separately in isTraeMatcher.
var traeToolNames = map[string]bool{
	"Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "LS": true,
	"RunCommand":      true,
	"WebSearch":       true,
	"WebFetch":        true,
	"AskUserQuestion": true,
	"Skill":           true,
}

// hookLifecycle is the event order docs.trae.ai/ide/automate-actions-with-hooks
// uses in its own "Hook events" table. Trae documents exactly these six;
// anything else passes through verbatim (see buildHooks) and follows in
// first-seen order.
var hookLifecycle = []string{
	"SessionStart", "UserPromptSubmit",
	"PreToolUse", "PostToolUse",
	"Stop", "Notification",
}

// toolNameMatcherEvents names the two events whose matcher is tested
// against a tool name. Trae reads a matcher on three events, "PreToolUse,
// PostToolUse, and Notification", but Notification matches a
// notification_type (`idle_prompt`, `permission_prompt`, ...) instead,
// so a tool-vocabulary check would misreport it.
var toolNameMatcherEvents = map[string]bool{
	"PreToolUse": true, "PostToolUse": true,
}

// group mirrors one hook group inside `hooks.<EventName>`: the shared
// `{matcher, hooks: [...]}` shape claude, codex, openhands, and qoder
// already emit, plus Trae's own `loop_limit`. Group is embedded rather
// than copied so the nested command entries keep one definition.
//
// loop_limit caps how many times a Stop hook may block the agent from
// stopping ("When loop_count >= loop_limit, this hook group will be
// skipped"), and Trae reads it on the Stop event only. omitempty is
// exact rather than lossy here: the vendor treats an absent key and a
// value "less than or equal to 0" identically, both falling back to its
// default of 5.
type group struct {
	Matcher   string                     `json:"matcher"`
	LoopLimit int                        `json:"loop_limit,omitempty"`
	Hooks     []claudehooks.CommandEntry `json:"hooks"`
}

// hooksDoc is the `.trae/hooks.json` shape: an integer `version` pegged
// to 1 ("The default value is 1, and currently only 1 is supported")
// wrapping a `hooks` map keyed by event name, the same envelope copilot
// emits. Events are held in an ordered list so lifecycle order survives
// marshaling, since Go alphabetizes map keys.
type hooksDoc struct {
	order  []string
	events map[string][]group
}

// MarshalJSON renders `{"version":1,"hooks":{<event>: [...], ...}}` in
// order.
func (d *hooksDoc) MarshalJSON() ([]byte, error) {
	var buf strings.Builder
	buf.WriteString(`{"version":1,"hooks":{`)
	for i, event := range d.order {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(event)
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := json.Marshal(d.events[event])
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}
	buf.WriteString("}}")
	return []byte(buf.String()), nil
}

// emitHooks writes `.trae/hooks.json` (override via
// outputs.trae.hooks-file). No-op when no hook spec produces an entry.
func emitHooks(sess *emit.Session, hooks []spec.Entry, cfg *config.Config, dryRun bool) error {
	doc := buildHooks(hooks)
	if doc == nil {
		return nil
	}
	body, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	path := emit.OutputHooksFile(cfg, target, defaultHooksFile)
	return sess.WriteFile(path, string(body)+"\n", dryRun)
}

// buildHooks returns the rendered document, or nil when no spec
// produces a hook entry. `event:` passes through verbatim, matching
// every other hook emitter in this repo and docs/user/spec-format.md's
// own stated policy.
//
// Only the three fields Trae's hook-definition table documents are
// written: `type` ("The default value is command, and currently only
// command is supported"), `command`, and `timeout` (seconds, default
// 30). The shared CommandEntry struct carries several more; none of
// them is documented here, so nothing sets them and they stay absent.
func buildHooks(hooks []spec.Entry) *hooksDoc {
	type matcherKey struct{ event, matcher string }
	byKey := map[matcherKey][]claudehooks.CommandEntry{}
	loopLimits := map[matcherKey]int{}
	var keyOrder []matcherKey
	var foreignMatchers int

	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		if event == "" {
			continue
		}
		commands := emit.HookCommands(h.Meta["command"])
		if len(commands) == 0 {
			continue
		}
		matcher, _ := h.Meta["matcher"].(string)
		if toolNameMatcherEvents[event] && !isTraeMatcher(matcher) {
			foreignMatchers++
		}
		k := matcherKey{event: event, matcher: matcher}
		if _, seen := byKey[k]; !seen {
			keyOrder = append(keyOrder, k)
		}
		if limit := emit.HookIntMeta(h.Meta, "loop_limit"); limit > 0 {
			loopLimits[k] = limit
		}
		for _, command := range commands {
			byKey[k] = append(byKey[k], claudehooks.CommandEntry{
				Type:    "command",
				Command: emit.RewriteHookPath(command, target),
				Timeout: emit.HookIntMeta(h.Meta, "timeout"),
			})
		}
	}
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", foreignMatchers,
		"Trae's hook tool names are not its subagent tool names: the terminal tool is RunCommand, not Bash, and there is no TodoWrite, so a Claude-style matcher parses as a valid regex and then matches nothing; use Read/Write/Edit/Glob/Grep/LS/RunCommand/WebSearch/WebFetch/AskUserQuestion/Skill, an mcp__<server>__<tool> name, `*`, or a regex")
	if len(keyOrder) == 0 {
		return nil
	}

	doc := &hooksDoc{events: map[string][]group{}}
	for _, k := range keyOrder {
		if _, seen := doc.events[k.event]; !seen {
			doc.order = append(doc.order, k.event)
		}
		doc.events[k.event] = append(doc.events[k.event], group{
			Matcher:   k.matcher,
			LoopLimit: loopLimits[k],
			Hooks:     byKey[k],
		})
	}
	for event, groups := range doc.events {
		sort.SliceStable(groups, func(i, j int) bool { return groups[i].Matcher < groups[j].Matcher })
		doc.events[event] = groups
	}
	doc.order = orderEvents(doc.order)
	return doc
}

// isTraeMatcher reports whether matcher can still reach a Trae tool. An
// empty matcher, `*`, and anything that is not a bare tool name (a
// regex, an alternation, an mcp__ name) are all left alone: only a
// single identifier that Trae's own table does not list is worth
// flagging, since that is the case a Claude-authored spec produces and
// the one that silently matches nothing.
func isTraeMatcher(matcher string) bool {
	if matcher == "" || matcher == "*" {
		return true
	}
	if strings.HasPrefix(matcher, "mcp__") {
		return true
	}
	if strings.ContainsAny(matcher, `|.*+?()[]{}^$\ `) {
		return true
	}
	return traeToolNames[matcher]
}

// orderEvents puts the vendor's documented lifecycle first, then any
// remaining event in the order it was first seen.
func orderEvents(seen []string) []string {
	present := map[string]bool{}
	for _, e := range seen {
		present[e] = true
	}
	out := make([]string, 0, len(seen))
	for _, e := range hookLifecycle {
		if present[e] {
			out = append(out, e)
			delete(present, e)
		}
	}
	for _, e := range seen {
		if present[e] {
			out = append(out, e)
			delete(present, e)
		}
	}
	return out
}
