package spec

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/suggest"
)

// PortableHookEvents are the values a hook spec's `on:` takes, in
// lifecycle order.
var PortableHookEvents = []string{"session-start", "prompt-submit", "before-tool", "after-tool", "after-edit", "stop", "session-end"}

// HookToolKinds are the values `match:` takes besides mcp:<server>.
var HookToolKinds = []string{"shell", "edit", "read", "web", "any"}

const mcpToolKind = "mcp:"

var mcpServerName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// nativeHookNames is one target's translation of the portable form. An
// event or tool kind missing from it has no exact native form there. mcp
// formats a server name into the matcher for its tools, or is empty.
type nativeHookNames struct {
	events map[string]string
	tools  map[string]string
	mcp    string
}

// sharedHookEvents are the event names Claude Code and Codex share.
var sharedHookEvents = map[string]string{
	"session-start": "SessionStart",
	"prompt-submit": "UserPromptSubmit",
	"before-tool":   "PreToolUse",
	"after-tool":    "PostToolUse",
	"after-edit":    "PostToolUse",
	"stop":          "Stop",
	"session-end":   "SessionEnd",
}

// portableHookTargets holds every target the portable form translates
// for. An event is listed only where the target reads exit 0, exit 1,
// and exit 2 with stderr as Claude Code does on it, so one script decides
// the same everywhere; hookrun's per-target models are the source, and a
// test holds the table to them. Tool names come from the same models.
// Claude Code's edit names every edit tool a version may have; a name it
// lacks never matches. Codex takes Edit and Write as aliases for
// apply_patch. A target whose matcher is an unanchored regular
// expression gets anchored names, so no other tool matches.
var portableHookTargets = map[string]nativeHookNames{
	"claude": {
		events: sharedHookEvents,
		tools:  map[string]string{"shell": "Bash", "edit": "Edit|MultiEdit|Write|NotebookEdit", "read": "Read", "web": "WebFetch|WebSearch", "any": ""},
		mcp:    "mcp__%s__.*",
	},
	"codex": {
		events: sharedHookEvents,
		tools:  map[string]string{"shell": "Bash", "edit": "Edit|Write", "any": ""},
		mcp:    "mcp__%s__.*",
	},
	"gemini": {
		events: map[string]string{
			"session-start": "SessionStart", "prompt-submit": "BeforeAgent", "before-tool": "BeforeTool",
			"after-tool": "AfterTool", "after-edit": "AfterTool", "stop": "AfterAgent", "session-end": "SessionEnd",
		},
		tools: map[string]string{"shell": "^run_shell_command$", "edit": "^(write_file|replace)$", "read": "^(read_file|read_many_files)$", "web": "^(web_fetch|google_web_search)$", "any": ""},
	},
	"factory": {
		events: sharedHookEvents,
		tools:  map[string]string{"shell": "^Execute$", "edit": "^(Create|Edit|ApplyPatch)$", "read": "^Read$", "web": "^(FetchUrl|WebSearch)$", "any": ""},
	},
	"qoder": {
		events: pickEvents("session-start", "prompt-submit", "before-tool", "stop", "session-end"),
		tools:  map[string]string{"shell": "Bash", "edit": "Edit|Write|NotebookEdit", "read": "Read", "web": "WebFetch|WebSearch", "any": ""},
		mcp:    "mcp__%s__.*",
	},
	"openhands": {
		events: pickEvents("session-start", "prompt-submit", "before-tool", "stop", "session-end"),
		tools:  map[string]string{"shell": "terminal", "any": ""},
	},
	"goose": {
		events: pickEvents("session-start", "before-tool", "stop", "session-end"),
		tools:  map[string]string{"shell": "^shell$", "edit": "^(write|edit)$", "any": ""},
	},
	"augment": {
		events: pickEvents("session-start", "before-tool", "session-end"),
		tools:  map[string]string{"shell": "^launch-process$", "edit": "^(str-replace-editor|save-file)$", "web": "^(web-fetch|web-search)$", "any": ""},
	},
	"crush": {
		events: pickEvents("before-tool"),
		tools:  map[string]string{"shell": "^bash$", "edit": "^(edit|multiedit|write)$", "any": ""},
	},
}

// pickEvents is the subset of sharedHookEvents a target reads alike.
func pickEvents(on ...string) map[string]string {
	out := make(map[string]string, len(on))
	for _, o := range on {
		out[o] = sharedHookEvents[o]
	}
	return out
}

// PortableHookTargets lists the targets the portable form translates for.
func PortableHookTargets() []string {
	return slices.Sorted(maps.Keys(portableHookTargets))
}

// TranslatesPortableHooks reports whether target reads `on:` and `match:`.
func TranslatesPortableHooks(target string) bool {
	_, ok := portableHookTargets[target]
	return ok
}

// IsPortableHook reports whether a hook spec uses the portable form.
func IsPortableHook(meta map[string]any) bool {
	_, on := meta["on"]
	_, match := meta["match"]
	return on || match
}

// PortableHookProblem returns why a portable hook spec is invalid on
// every target, or "" when it is valid.
func PortableHookProblem(meta map[string]any) string {
	on, onSet := meta["on"]
	if !onSet {
		return "match: needs on:; set on: to before-tool or after-tool"
	}
	event, ok := on.(string)
	if !ok || !slices.Contains(PortableHookEvents, event) {
		text := fmt.Sprint(on)
		if s := suggest.Name(text, PortableHookEvents); s != "" {
			return fmt.Sprintf("unknown hook event %q for on: (did you mean %s?)", text, s)
		}
		return fmt.Sprintf("unknown hook event %q for on:; use one of %s", text, strings.Join(PortableHookEvents, ", "))
	}
	if _, set := meta["event"]; set {
		return "sets both on: and event:; keep one"
	}
	if _, set := meta["matcher"]; set {
		return "matcher: goes with event:; with on:, write match:"
	}
	match, matchSet := meta["match"]
	if !matchSet {
		return ""
	}
	if event != "before-tool" && event != "after-tool" {
		return fmt.Sprintf("match: applies only to on: before-tool and after-tool, not %s", event)
	}
	kind, _ := match.(string)
	if server, ok := strings.CutPrefix(kind, mcpToolKind); ok {
		if !mcpServerName.MatchString(server) {
			return fmt.Sprintf("match: %q needs an MCP server name of letters, digits, _, or -", kind)
		}
		return ""
	}
	if !slices.Contains(HookToolKinds, kind) {
		text := fmt.Sprint(match)
		if s := suggest.Name(text, HookToolKinds); s != "" {
			return fmt.Sprintf("unknown tool kind %q for match: (did you mean %s?)", text, s)
		}
		return fmt.Sprintf("unknown tool kind %q for match:; use one of %s, or mcp:<server>", text, strings.Join(HookToolKinds, ", "))
	}
	return ""
}

// NativeHook returns the hook with target's native `event:` and
// `matcher:` in place of `on:` and `match:`, or why it does not emit to
// target. A hook in the native form returns unchanged.
func (e Entry) NativeHook(target string) (Entry, string) {
	if e.Kind != KindHook || !IsPortableHook(e.Meta) {
		return e, ""
	}
	if problem := PortableHookProblem(e.Meta); problem != "" {
		return e, problem
	}
	on, _ := e.Meta["on"].(string)
	kind, _ := e.Meta["match"].(string)
	if on == "after-edit" {
		kind = "edit"
	}
	event, matcher, reason := nativeHookFor(target, on, kind)
	if reason != "" {
		return e, reason
	}
	meta := maps.Clone(e.Meta)
	delete(meta, "on")
	delete(meta, "match")
	meta["event"] = event
	if matcher != "" {
		meta["matcher"] = matcher
	}
	e.Meta = meta
	if e.MetaKeys != nil {
		keys := slices.Clone(e.MetaKeys)
		for i, k := range keys {
			switch k {
			case "on":
				keys[i] = "event"
			case "match":
				keys[i] = "matcher"
			}
		}
		e.MetaKeys = keys
	}
	return e, ""
}

// PortableHookEvent returns target's native event for a portable one, or
// false when the target has none.
func PortableHookEvent(target, on string) (string, bool) {
	event, ok := portableHookTargets[target].events[on]
	return event, ok
}

// HookToolMatcher returns target's native matcher for a tool kind, or
// false when the target has none. An MCP kind takes the server name.
func HookToolMatcher(target, kind string) (string, bool) {
	if server, ok := strings.CutPrefix(kind, mcpToolKind); ok {
		format := portableHookTargets[target].mcp
		if format == "" {
			return "", false
		}
		return fmt.Sprintf(format, server), true
	}
	matcher, ok := portableHookTargets[target].tools[kind]
	return matcher, ok
}

func nativeHookFor(target, on, kind string) (event, matcher, reason string) {
	names, ok := portableHookTargets[target]
	if !ok {
		return "", "", fmt.Sprintf("on: has no %s mapping yet; write event: for %s", target, target)
	}
	event, ok = names.events[on]
	if !ok {
		return "", "", fmt.Sprintf("%s has no %s event that reads exit codes as Claude Code does", target, on)
	}
	if kind == "" {
		return event, "", ""
	}
	if server, ok := strings.CutPrefix(kind, mcpToolKind); ok {
		if names.mcp == "" {
			return "", "", fmt.Sprintf("%s has no MCP tool name a hook can match", target)
		}
		return event, fmt.Sprintf(names.mcp, server), ""
	}
	matcher, ok = names.tools[kind]
	if !ok {
		return "", "", fmt.Sprintf("%s has no %s tool a hook can match", target, kind)
	}
	return event, matcher, ""
}

// PortableHookForm returns the `on:` and `match:` that translate to
// exactly event and matcher on every one of targets. A spec without a
// matcher gets no match:, so the rewrite renames keys one to one.
// blocker names the first target no common form reaches.
func PortableHookForm(targets []string, event, matcher string, hasMatcher bool) (on, match, blocker string) {
	type form struct{ on, match string }
	var candidates []form
	if hasMatcher {
		kinds := slices.Clone(HookToolKinds)
		if rest, ok := strings.CutPrefix(matcher, "mcp__"); ok {
			if server, ok := strings.CutSuffix(rest, "__.*"); ok && mcpServerName.MatchString(server) {
				kinds = append(kinds, mcpToolKind+server)
			}
		}
		for _, on := range []string{"before-tool", "after-tool"} {
			for _, kind := range kinds {
				candidates = append(candidates, form{on, kind})
			}
		}
	} else {
		for _, on := range PortableHookEvents {
			candidates = append(candidates, form{on, ""})
		}
	}
	for _, target := range targets {
		candidates = slices.DeleteFunc(candidates, func(f form) bool {
			kind := f.match
			if f.on == "after-edit" {
				kind = "edit"
			}
			e, m, reason := nativeHookFor(target, f.on, kind)
			return reason != "" || e != event || m != matcher
		})
		if len(candidates) == 0 {
			return "", "", target
		}
	}
	return candidates[0].on, candidates[0].match, ""
}
