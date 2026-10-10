package hookrun

import (
	"errors"
	"testing"
)

func TestKiroRawPayload_MatchesAV3ToolByIDOrTag(t *testing.T) {
	call := func(tool string) Input {
		return Input{Raw: []byte(`{"hook_event_name":"preToolUse","tool_name":"` + tool + `","tool_input":{}}`)}
	}
	for _, tc := range []struct {
		matcher, tool string
		fires         bool
	}{
		{"", "read_file", true},
		{"*", "anything", true},
		{"@git/git_status", "mcp_git_git_status", true},
		{"@git/git_log", "mcp_git_git_status", false},
		{"@git", "mcp_git_git_status", true},
		{"@git", "mcp_github_search", false},
		{"@My-Server/Do.It", "mcp_my_server_do_it", true},
		{"@git/*", "mcp_git_git_status", true},
		{"@mcp", "mcp_git_git_status", true},
		{"@mcp", "execute_bash", false},
		{"mcp_git_*", "mcp_git_git_status", true},
		{"mcp_git_*", "mcp_slack_post", false},
		{"mcp_git_*", "mcp_github_search", true},
		{"shell", "execute_bash", true},
		{"shell", "execute_pwsh", true},
		{"execute_pwsh", "execute_bash", true},
		{"read", "read_file", true},
		{"read", "grep_search", true},
		{"read", "fs_write", false},
		{"write", "fs_append", true},
		{"web", "web_fetch", true},
		{"context", "disclose_context", true},
		{"context", "read_file", false},
		{"w*", "str_replace", true},
		{"read_?ile", "read_file", true},
		{"read_*", "fs_write", false},
		{"fs_read", "read_file", false},
		{"use_aws", "aws", false},
		{"^(fs_write|str_replace)$", "str_replace", true},
		{"^read$", "read_file", false},
		{"^shell$", "execute_bash", false},
	} {
		p, err := Build("kiro", "PreToolUse", tc.matcher, "/project", call(tc.tool))
		if err != nil || p.Fires != tc.fires || p.Trigger != tc.tool {
			t.Errorf("matcher %q on %s = %+v %v, want fires %t", tc.matcher, tc.tool, p, err, tc.fires)
		}
	}

	var unbuilt Unbuilt
	for _, matcher := range []string{"@powers", "@builtin", "spec", "subagent"} {
		if _, err := Build("kiro", "PreToolUse", matcher, "/project", call("execute_bash")); !errors.As(err, &unbuilt) {
			t.Errorf("%s names tools the docs do not list: %v", matcher, err)
		}
	}
	if p, err := Build("kiro", "PreToolUse", "spec", "/project", call("spec")); err != nil || !p.Fires {
		t.Errorf("a tool ID that equals the matcher fires without listing the tag: %+v %v", p, err)
	}

	p, err := Build("kiro", "UserPromptSubmit", "deploy", "/project", Input{Raw: []byte(`{"prompt":"hello"}`)})
	if err != nil || !p.Fires {
		t.Errorf("a --payload prompt ignores the matcher on CLI V3: %+v %v", p, err)
	}
}
