package antigravity

import (
	"encoding/json"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

const defaultHooksFile = ".agents/hooks.json"

// hookEvents is the closed set from antigravity.google/docs/ide/hooks'
// own Hook Event Names list. A spec naming anything else earns a
// coverage note rather than a key the vendor documents no handler for.
var hookEvents = map[string]bool{
	"PreToolUse": true, "PostToolUse": true,
	"PreInvocation": true, "PostInvocation": true, "Stop": true,
}

// matcherEvents are the two events whose array holds `{matcher, hooks}`
// groups. For the other three "the structure is simpler (a list of
// handlers directly under the event key) and the matcher is ignored"
// (same page), so this adapter writes each shape where the vendor
// documents it rather than one shape everywhere.
var matcherEvents = map[string]bool{"PreToolUse": true, "PostToolUse": true}

// claudeToolNames are the Claude-style matcher values Antigravity does
// not answer to. Its own tool vocabulary is its own (`view_file`,
// `replace_file_content`, `grep_search`, `run_command`, ...) with no
// name in common, so a matcher carried over from a Claude spec parses
// as a valid regex and then matches nothing, the same trap openhands,
// crush, and windsurf hit with their own vocabularies. No rename is
// attempted: the vendor gives no mapping, and its subagent page warns
// that an unmapped tool name can hang the process.
var claudeToolNames = map[string]bool{
	"Bash": true, "Read": true, "Write": true, "Edit": true,
	"Glob": true, "Grep": true, "WebFetch": true, "WebSearch": true,
}

// hookHandler mirrors one object in a Hook Handler Fields table row set:
// `type` optional and defaulting to `"command"`, `command` required,
// `timeout` an integer in seconds defaulting to 30. `type` is written
// explicitly even though it is optional, the same as every other
// hook-emitting adapter here, so the emitted file states the shape it
// relies on instead of leaning on a default the vendor may re-point.
type hookHandler struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

// hookGroup mirrors one `{matcher, hooks}` object under PreToolUse or
// PostToolUse. Matcher stays non-omitempty: the vendor states an empty
// string is itself meaningful ("empty string or `\"*\"` matches all
// tools").
type hookGroup struct {
	Matcher string        `json:"matcher"`
	Hooks   []hookHandler `json:"hooks"`
}

// hookDefinition is one named top-level object in `.agents/hooks.json`.
// Antigravity keys the file by hook definition name rather than by
// event, so each spec becomes its own definition holding the one event
// it names. `enabled` is the definition's own field, a sibling of the
// event keys rather than a member of the handler array: "Set to `false`
// to disable the hook without removing it."
type hookDefinition struct {
	name string
	// disabled renders as `"enabled": false`. Antigravity spells the
	// portable `disabled: true` as its inverse here, the way codex and
	// kilo do for MCP servers, unlike this adapter's own MCP file which
	// takes `disabled` literally.
	disabled bool
	event    string
	// payload is the event's array: `[]hookGroup` for the two events
	// that read a matcher, `[]hookHandler` for the three that do not.
	payload any
}

// MarshalJSON writes `enabled` ahead of the event key so a disabled
// definition reads as disabled before the reader reaches its handlers,
// matching the vendor's own `safety-gate` example.
func (d *hookDefinition) MarshalJSON() ([]byte, error) {
	body, err := json.Marshal(d.payload)
	if err != nil {
		return nil, err
	}
	key, err := json.Marshal(d.event)
	if err != nil {
		return nil, err
	}
	var buf strings.Builder
	buf.WriteByte('{')
	if d.disabled {
		buf.WriteString(`"enabled":false,`)
	}
	buf.Write(key)
	buf.WriteByte(':')
	buf.Write(body)
	buf.WriteByte('}')
	return []byte(buf.String()), nil
}

// hooksDoc is `.agents/hooks.json`'s top-level shape: an object keyed by
// hook definition name. Order is stored on the side so spec order
// survives Go's map-key alphabetizer.
type hooksDoc struct {
	order []string
	defs  map[string]*hookDefinition
}

// MarshalJSON renders `{<name>: {...}, ...}` in `order`.
func (d *hooksDoc) MarshalJSON() ([]byte, error) {
	var buf strings.Builder
	buf.WriteByte('{')
	for i, name := range d.order {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(name)
		if err != nil {
			return nil, err
		}
		val, err := json.Marshal(d.defs[name])
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(val)
	}
	buf.WriteByte('}')
	return []byte(buf.String()), nil
}

// emitHooks writes `.agents/hooks.json` (override via
// outputs.antigravity.hooks-file), the file Antigravity reads at
// workspace scope: "Hooks are configured in a `hooks.json` file located
// in your customization directory (e.g., `.agents/` in your workspace)"
// (antigravity.google/docs/ide/hooks, #629). No-op when no hooks emit.
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

// buildHooks returns the rendered document, or nil when no spec produces
// a hook definition. Each spec becomes one named definition, since the
// file's top level is keyed by definition name and a definition carries
// the events it handles rather than the other way round.
func buildHooks(hooks []spec.Entry) *hooksDoc {
	doc := &hooksDoc{defs: map[string]*hookDefinition{}}
	var otherEvents, claudeMatchers, ignoredMatchers int

	for _, h := range hooks {
		event, _ := h.Meta["event"].(string)
		if !hookEvents[event] {
			if event != "" {
				otherEvents++
			}
			continue
		}
		commands := emit.HookCommands(h.Meta["command"])
		if len(commands) == 0 {
			continue
		}
		timeout := emit.HookIntMeta(h.Meta, "timeout")
		handlers := make([]hookHandler, 0, len(commands))
		for _, command := range commands {
			handlers = append(handlers, hookHandler{
				Type:    "command",
				Command: emit.RewriteHookPath(command, target),
				Timeout: timeout,
			})
		}

		matcher, _ := h.Meta["matcher"].(string)
		def := &hookDefinition{
			name:     h.Name,
			disabled: emit.HookBoolMeta(h.Meta, "disabled"),
			event:    event,
		}
		if matcherEvents[event] {
			if claudeToolNames[matcher] {
				claudeMatchers++
			}
			def.payload = []hookGroup{{Matcher: matcher, Hooks: handlers}}
		} else {
			if matcher != "" {
				ignoredMatchers++
			}
			def.payload = handlers
		}

		if _, seen := doc.defs[def.name]; !seen {
			doc.order = append(doc.order, def.name)
		}
		doc.defs[def.name] = def
	}

	emit.NoteFieldNoOp(target, spec.KindHook, "event", otherEvents,
		"Antigravity documents five events (PreToolUse, PostToolUse, PreInvocation, PostInvocation, Stop); any other name has no handler in hooks.json")
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", claudeMatchers,
		"Antigravity's own tool names are its own (view_file, replace_file_content, grep_search, run_command), not Claude's; a Claude-style matcher parses but matches nothing")
	emit.NoteFieldNoOp(target, spec.KindHook, "matcher", ignoredMatchers,
		"PreInvocation, PostInvocation and Stop take a handler list directly and Antigravity ignores the matcher there")

	if len(doc.order) == 0 {
		return nil
	}
	return doc
}
