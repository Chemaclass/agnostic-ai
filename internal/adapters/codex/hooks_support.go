package codex

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

// hookEvents are the events learn.chatgpt.com/docs/hooks lists.
var hookEvents = []string{
	"PreToolUse", "PostToolUse",
	"PermissionRequest",
	"UserPromptSubmit",
	"SessionStart", "SessionEnd", "Stop",
	"SubagentStart", "SubagentStop",
	"PreCompact", "PostCompact",
	"Interrupt",
}

// HookEvents lists every event a hook spec may name for this target, so
// `validate` reads the same vocabulary the emitter maps.
func HookEvents() []string { return slices.Clone(hookEvents) }

// toolEvents match on tool_name. Codex runs Bash and apply_patch, takes
// Edit and Write as aliases for apply_patch, and names MCP tools
// mcp__<server>__<tool>.
var toolEvents = []string{"PreToolUse", "PostToolUse", "PermissionRequest"}

var toolMatchers = []string{"Bash", "apply_patch", "Edit", "Write"}

// sourceMatchers are the values Codex documents per non-tool event. An
// event absent here and from matcherFreeEvents takes no matcher on
// Codex, so a hook naming one does not run as written.
var sourceMatchers = map[string][]string{
	"SessionStart": {"startup", "resume", "clear", "compact"},
	"PreCompact":   {"manual", "auto"},
	"PostCompact":  {"manual", "auto"},
}

// matcherFreeEvents ignore the matcher or take any value: Codex matches
// SubagentStart and SubagentStop on agent_type, which names a project
// agent.
var matcherFreeEvents = []string{"UserPromptSubmit", "Stop", "Interrupt", "SubagentStart", "SubagentStop"}

// hookFieldsCodexDrops are Claude hook fields the Codex emitter has no
// key for. A hook carrying one would run wider or differently there.
var hookFieldsCodexDrops = []string{"if", "shell", "once", "asyncRewake"}

// AcceptsHook returns why Codex would not run the hook as written, or
// "" when it would: the event exists, every matcher segment names a
// tool or source Codex reports, and no field is dropped on the way.
func (Adapter) AcceptsHook(meta map[string]any) string {
	event, _ := meta["event"].(string)
	if !slices.Contains(hookEvents, event) {
		return fmt.Sprintf("Codex has no %s event", event)
	}
	if kind, _ := meta["type"].(string); kind != "" && kind != "command" && kind != "mcp_tool" {
		return fmt.Sprintf("Codex hooks have no %s handler", kind)
	}
	for _, field := range hookFieldsCodexDrops {
		if _, set := meta[field]; set {
			return fmt.Sprintf("Codex hooks have no %s field", field)
		}
	}
	matcher, _ := meta["matcher"].(string)
	names, exact := exactNameList(matcher)
	if exact {
		for _, seg := range names {
			if !codexMatcherSegment(event, seg) {
				return fmt.Sprintf("Codex %s does not match %q", event, seg)
			}
		}
		return ""
	}
	if slices.Contains(matcherFreeEvents, event) {
		return ""
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return fmt.Sprintf("Codex %s matcher %q is not a valid regular expression", event, matcher)
	}
	if !regexFires(event, matcher, re) {
		return fmt.Sprintf("Codex %s does not match %q", event, matcher)
	}
	return ""
}

// exactNames is the grammar the tools read as a list of exact names:
// letters, digits, _, -, spaces, comma, and |. Anything else is a regex.
var exactNames = regexp.MustCompile(`^[A-Za-z0-9_\- ,|]+$`)

// exactNameList returns the names in an exact-name matcher, or false when
// the matcher is a regex. An empty matcher lists no names.
func exactNameList(matcher string) ([]string, bool) {
	matcher = strings.TrimSpace(matcher)
	switch {
	case matcher == "":
		return nil, true
	case matcher == "*":
		return []string{"*"}, true
	case !exactNames.MatchString(matcher):
		return nil, false
	}
	var names []string
	for _, name := range strings.FieldsFunc(matcher, func(r rune) bool { return r == '|' || r == ',' }) {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names, true
}

// regexFires reports whether a regex matcher may fire on a tool or source
// name Codex reports for the event. It does when it matches a built-in
// name. MCP server names are unknown, so a tool matcher also counts unless
// it provably cannot match an mcp__ name; see rejectsMCPNames.
func regexFires(event, matcher string, re *regexp.Regexp) bool {
	if !slices.Contains(toolEvents, event) {
		return slices.ContainsFunc(sourceMatchers[event], re.MatchString)
	}
	return slices.ContainsFunc(toolMatchers, re.MatchString) || !rejectsMCPNames(matcher)
}

// rejectsMCPNames proves the trivial case only: a regex anchored with ^
// and with no top-level |, whose literal prefix (up to the first
// metacharacter, minus a quantified last character) neither is a prefix of
// mcp__ nor starts with it.
func rejectsMCPNames(matcher string) bool {
	if !strings.HasPrefix(matcher, "^") {
		return false
	}
	alternation := false
	scanRegex(matcher, func(_ int, c byte, depth int) {
		alternation = alternation || (c == '|' && depth == 0)
	})
	if alternation {
		return false
	}
	body := matcher[1:]
	end := strings.IndexAny(body, `\.+*?()[]{}|^$`)
	if end < 0 {
		end = len(body)
	} else if strings.IndexByte("*?+{", body[end]) >= 0 && end > 0 {
		end--
	}
	prefix := body[:end]
	return !strings.HasPrefix(spec.MCPToolPrefix, prefix) && !strings.HasPrefix(prefix, spec.MCPToolPrefix)
}

// scanRegex calls visit for each ( ) and | in a regex that is not escaped
// or inside a [...] class, with the group depth before that byte. A class
// may open with ^, then a literal ], and may hold [:name:] sets.
func scanRegex(s string, visit func(i int, c byte, depth int)) {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '\\':
			i++
		case '[':
			i = classEnd(s, i)
		case '(', ')', '|':
			visit(i, c, depth)
			if c == '(' {
				depth++
			} else if c == ')' {
				depth--
			}
		}
	}
}

// classEnd returns the index of the ] that closes the class opening at
// start, or the last index when it never closes.
func classEnd(s string, start int) int {
	i := start + 1
	if i < len(s) && s[i] == '^' {
		i++
	}
	if i < len(s) && s[i] == ']' {
		i++
	}
	for ; i < len(s); i++ {
		switch {
		case s[i] == '\\':
			i++
		case strings.HasPrefix(s[i:], "[:"):
			if end := strings.Index(s[i+2:], ":]"); end >= 0 {
				i += end + 3
			}
		case s[i] == ']':
			return i
		}
	}
	return len(s) - 1
}

// outerGroup returns the body of s when one (...) or (?:...) group spans
// all of it.
func outerGroup(s string) (string, bool) {
	if !strings.HasPrefix(s, "(") || !strings.HasSuffix(s, ")") {
		return "", false
	}
	spans := true
	scanRegex(s, func(i int, c byte, depth int) {
		if c == ')' && depth == 1 && i != len(s)-1 {
			spans = false
		}
	})
	if !spans {
		return "", false
	}
	return strings.TrimPrefix(s[1:len(s)-1], "?:"), true
}

func codexMatcherSegment(event, seg string) bool {
	if seg == "*" || slices.Contains(matcherFreeEvents, event) {
		return true
	}
	if slices.Contains(toolEvents, event) {
		return slices.Contains(toolMatchers, seg) || strings.HasPrefix(seg, spec.MCPToolPrefix)
	}
	return slices.Contains(sourceMatchers[event], seg)
}

// editPayloadReason is the coverage note for an edit hook that reads the
// Claude payload. Codex reports an edit as tool_name apply_patch with
// the patch in tool_input.command (learn.chatgpt.com/docs/hooks).
const editPayloadReason = "Codex reports an edit as apply_patch with the patch in tool_input.command, so a command reading tool_input.file_path gets an empty value; read the edited paths with `agnostic-ai hook paths` instead"

// readsFilePath matches the field access in jq (.tool_input.file_path)
// and in a script's subscript (["tool_input"]["file_path"]).
var readsFilePath = regexp.MustCompile(`tool_input\W{1,4}file_path`)

// editMatchers are the matcher segments that fire on a Codex edit.
var editMatchers = []string{"apply_patch", "Edit", "Write", "*"}

// Codex never sends tool_input.file_path; inspect commands and the scripts sync copies.
func noteEditHookPayload(sess *emit.Session, hooks []spec.Entry, mode string) error {
	if mode == emit.OnUnsupportedSilent {
		return nil
	}
	var paths []string
	for _, h := range hooks {
		if kind, _ := h.Meta["type"].(string); kind != "" && kind != "command" {
			continue
		}
		if !firesOnEdit(h.Meta) {
			continue
		}
		for _, command := range hookCommands(h.Meta["command"]) {
			reads := readsFilePath.MatchString(command)
			if !reads {
				sourceTool, _ := emit.SourceToolFromHookCommand(command)
				bodies, err := sess.MaterializedHookScriptBodies(command, target, sourceTool, h.Meta)
				if err != nil {
					return fmt.Errorf("inspect hook %s: %w", h.Path, err)
				}
				reads = slices.ContainsFunc(bodies, readsFilePath.Match)
			}
			if reads {
				paths = append(paths, h.Path)
				break
			}
		}
	}
	if len(paths) == 0 {
		return nil
	}
	switch mode {
	case emit.OnUnsupportedError:
		return fmt.Errorf("%s: %s", strings.Join(paths, ", "), editPayloadReason)
	case emit.OnUnsupportedSilent:
		return nil
	}
	emit.NoteFieldNoOp(target, spec.KindHook, "tool_input.file_path", len(paths), editPayloadReason)
	return nil
}

func firesOnEdit(meta map[string]any) bool {
	event, _ := meta["event"].(string)
	if !slices.Contains(toolEvents, event) {
		return false
	}
	matcher, _ := meta["matcher"].(string)
	segments, exact := exactNameList(matcher)
	if exact {
		return len(segments) == 0 || slices.ContainsFunc(segments, func(seg string) bool { return slices.Contains(editMatchers, seg) })
	}
	re, err := regexp.Compile(matcher)
	return err != nil || slices.ContainsFunc(editMatchers, re.MatchString)
}
