package spec

import (
	"strings"
	"testing"
)

func portableHook(meta map[string]any) Entry {
	meta["command"] = "true"
	return Entry{Kind: KindHook, Name: "h", Path: "hooks/h.yaml", Meta: meta}
}

func TestNativeHook_TranslatesEveryEventAndToolKind(t *testing.T) {
	cases := []struct {
		target, on, match, event, matcher string
	}{
		{"claude", "session-start", "", "SessionStart", ""},
		{"claude", "prompt-submit", "", "UserPromptSubmit", ""},
		{"claude", "before-tool", "shell", "PreToolUse", "Bash"},
		{"claude", "after-tool", "edit", "PostToolUse", "Edit|MultiEdit|Write|NotebookEdit"},
		{"claude", "after-edit", "", "PostToolUse", "Edit|MultiEdit|Write|NotebookEdit"},
		{"codex", "after-tool", "edit", "PostToolUse", "Edit|Write"},
		{"claude", "before-tool", "read", "PreToolUse", "Read"},
		{"claude", "before-tool", "web", "PreToolUse", "WebFetch|WebSearch"},
		{"claude", "before-tool", "any", "PreToolUse", ""},
		{"claude", "before-tool", "", "PreToolUse", ""},
		{"claude", "after-tool", "mcp:github", "PostToolUse", "mcp__github__.*"},
		{"claude", "stop", "", "Stop", ""},
		{"claude", "session-end", "", "SessionEnd", ""},
		{"codex", "before-tool", "shell", "PreToolUse", "Bash"},
		{"codex", "after-edit", "", "PostToolUse", "Edit|Write"},
		{"codex", "before-tool", "mcp:docs", "PreToolUse", "mcp__docs__.*"},
		{"codex", "session-end", "", "SessionEnd", ""},
	}
	for _, c := range cases {
		meta := map[string]any{"on": c.on}
		if c.match != "" {
			meta["match"] = c.match
		}
		got, reason := portableHook(meta).NativeHook(c.target)
		if reason != "" {
			t.Errorf("%s %s/%s: %s", c.target, c.on, c.match, reason)
			continue
		}
		matcher, hasMatcher := got.Meta["matcher"]
		if got.Meta["event"] != c.event || (c.matcher == "" && hasMatcher) || (c.matcher != "" && matcher != c.matcher) {
			t.Errorf("%s %s/%s = %v %v, want %s %q", c.target, c.on, c.match, got.Meta["event"], matcher, c.event, c.matcher)
		}
		if _, ok := got.Meta["on"]; ok {
			t.Errorf("%s %s: on: must not reach the adapter", c.target, c.on)
		}
	}
}

func TestNativeHook_ReportsWhatATargetCannotExpress(t *testing.T) {
	for _, c := range []struct {
		target string
		meta   map[string]any
		want   string
	}{
		{"codex", map[string]any{"on": "before-tool", "match": "read"}, "codex has no read tool"},
		{"codex", map[string]any{"on": "after-tool", "match": "web"}, "codex has no web tool"},
		{"cursor", map[string]any{"on": "before-tool", "match": "shell"}, "write event: for cursor"},
		{"claude", map[string]any{"on": "before-tol"}, "did you mean before-tool"},
	} {
		h := portableHook(c.meta)
		got, reason := h.NativeHook(c.target)
		if !strings.Contains(reason, c.want) {
			t.Errorf("%s %v: reason = %q, want %q", c.target, c.meta, reason, c.want)
		}
		if got.Meta["event"] != nil {
			t.Errorf("%s %v: a hook that does not translate must stay as written", c.target, c.meta)
		}
	}
}

func TestNativeHook_LeavesNativeHooksAlone(t *testing.T) {
	h := portableHook(map[string]any{"event": "beforeShellExecution"})
	got, reason := h.NativeHook("cursor")
	if reason != "" || got.Meta["event"] != "beforeShellExecution" {
		t.Errorf("native hook = %v, %q", got.Meta, reason)
	}
}

func TestPortableHookProblem(t *testing.T) {
	for _, c := range []struct {
		meta map[string]any
		want string
	}{
		{map[string]any{"on": "before-tool", "match": "shell"}, ""},
		{map[string]any{"on": "before-tool", "match": "mcp:my_server-1"}, ""},
		{map[string]any{"on": "launch"}, `unknown hook event "launch" for on:`},
		{map[string]any{"on": "stop", "event": "Stop"}, "sets both on: and event:"},
		{map[string]any{"on": "before-tool", "matcher": "Bash"}, "with on:, write match:"},
		{map[string]any{"match": "shell"}, "match: needs on:"},
		{map[string]any{"on": "after-edit", "match": "edit"}, "match: applies only to on: before-tool and after-tool"},
		{map[string]any{"on": "before-tool", "match": "shel"}, "did you mean shell"},
		{map[string]any{"on": "before-tool", "match": "mcp:"}, "needs an MCP server name"},
		{map[string]any{"on": "before-tool", "match": "mcp:a.b"}, "needs an MCP server name"},
	} {
		got := PortableHookProblem(c.meta)
		if c.want == "" && got != "" || c.want != "" && !strings.Contains(got, c.want) {
			t.Errorf("%v = %q, want %q", c.meta, got, c.want)
		}
	}
}

func TestBundleFor_TranslatesPortableHooksAndDropsTheRest(t *testing.T) {
	b := NewBundle([]Entry{
		portableHook(map[string]any{"on": "before-tool", "match": "read"}),
		portableHook(map[string]any{"on": "session-start"}),
	})
	b.Hooks[0].Name, b.Hooks[1].Name = "guard", "status"
	if got := b.For("claude").Hooks; len(got) != 2 || got[0].Meta["matcher"] != "Read" {
		t.Errorf("claude hooks = %+v", got)
	}
	if got := b.For("codex").Hooks; len(got) != 1 || got[0].Name != "status" || got[0].Meta["event"] != "SessionStart" {
		t.Errorf("codex hooks = %+v, want only status", got)
	}
	if got := b.HooksFor("cursor"); len(got) != 0 {
		t.Errorf("cursor hooks = %+v, want none until it translates", got)
	}
	if b.Hooks[0].Meta["event"] != nil {
		t.Error("For must not change the source entry")
	}
}

func TestPortableHookForm(t *testing.T) {
	both := []string{"claude", "codex"}
	for _, c := range []struct {
		targets            []string
		event, matcher     string
		hasMatcher         bool
		on, match, blocker string
	}{
		{both, "PreToolUse", "Bash", true, "before-tool", "shell", ""},
		{both, "PostToolUse", "Edit|Write", true, "", "", "claude"},
		{[]string{"claude"}, "PostToolUse", "Edit|MultiEdit|Write|NotebookEdit", true, "after-tool", "edit", ""},
		{[]string{"codex"}, "PostToolUse", "Edit|Write", true, "after-tool", "edit", ""},
		{both, "PostToolUse", "", false, "after-tool", "", ""},
		{both, "PreToolUse", "", true, "before-tool", "any", ""},
		{both, "SessionStart", "", false, "session-start", "", ""},
		{both, "PreToolUse", "mcp__github__.*", true, "before-tool", "mcp:github", ""},
		{both, "PreToolUse", "Read", true, "", "", "codex"},
		{[]string{"claude"}, "PreToolUse", "Read", true, "before-tool", "read", ""},
		{both, "PreToolUse", "Write|Edit", true, "", "", "claude"},
		{both, "Notification", "", false, "", "", "claude"},
		{[]string{"claude", "cursor"}, "PreToolUse", "Bash", true, "", "", "cursor"},
	} {
		on, match, blocker := PortableHookForm(c.targets, c.event, c.matcher, c.hasMatcher)
		if on != c.on || match != c.match || blocker != c.blocker {
			t.Errorf("%v %s %q = %q %q %q, want %q %q %q", c.targets, c.event, c.matcher, on, match, blocker, c.on, c.match, c.blocker)
		}
	}
}

func TestPortableHookTargets_TranslationTable(t *testing.T) {
	type row struct{ events, shell, edit, read, web, mcp string }
	want := map[string]row{
		"claude":    {"SessionStart UserPromptSubmit PreToolUse PostToolUse PostToolUse Stop SessionEnd", "Bash", "Edit|MultiEdit|Write|NotebookEdit", "Read", "WebFetch|WebSearch", "mcp__s__.*"},
		"codex":     {"SessionStart UserPromptSubmit PreToolUse PostToolUse PostToolUse Stop SessionEnd", "Bash", "Edit|Write", "-", "-", "mcp__s__.*"},
		"gemini":    {"SessionStart BeforeAgent BeforeTool AfterTool AfterTool AfterAgent SessionEnd", "^run_shell_command$", "^(write_file|replace)$", "^(read_file|read_many_files)$", "^(web_fetch|google_web_search)$", "-"},
		"factory":   {"SessionStart UserPromptSubmit PreToolUse PostToolUse PostToolUse Stop SessionEnd", "^Execute$", "^(Create|Edit|ApplyPatch)$", "^Read$", "^(FetchUrl|WebSearch)$", "-"},
		"qoder":     {"SessionStart UserPromptSubmit PreToolUse - - Stop SessionEnd", "Bash", "Edit|Write|NotebookEdit", "Read", "WebFetch|WebSearch", "mcp__s__.*"},
		"openhands": {"SessionStart UserPromptSubmit PreToolUse - - Stop SessionEnd", "terminal", "-", "-", "-", "-"},
		"goose":     {"SessionStart - PreToolUse - - Stop SessionEnd", "^shell$", "^(write|edit)$", "-", "-", "-"},
		"augment":   {"SessionStart - PreToolUse - - - SessionEnd", "^launch-process$", "^(str-replace-editor|save-file)$", "-", "^(web-fetch|web-search)$", "-"},
		"crush":     {"- - PreToolUse - - - -", "^bash$", "^(edit|multiedit|write)$", "-", "-", "-"},
	}
	if got := PortableHookTargets(); len(got) != len(want) {
		t.Errorf("targets = %v, want %d", got, len(want))
	}
	or := func(s string, ok bool) string {
		if !ok {
			return "-"
		}
		return s
	}
	for target, w := range want {
		var events []string
		for _, on := range PortableHookEvents {
			events = append(events, or(PortableHookEvent(target, on)))
		}
		got := row{strings.Join(events, " "), or(HookToolMatcher(target, "shell")), or(HookToolMatcher(target, "edit")),
			or(HookToolMatcher(target, "read")), or(HookToolMatcher(target, "web")), or(HookToolMatcher(target, "mcp:s"))}
		if got != w {
			t.Errorf("%s =\n  %+v\nwant\n  %+v", target, got, w)
		}
		if m, ok := HookToolMatcher(target, "any"); !ok || m != "" {
			t.Errorf("%s: any must write no matcher", target)
		}
	}
}
