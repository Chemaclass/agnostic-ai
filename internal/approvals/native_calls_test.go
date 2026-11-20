package approvals

import "testing"

func TestNativeRequest_Kiro(t *testing.T) {
	r := NativeRequest("kiro", Request{Call: "execute_bash", Input: map[string]any{"command": "go test ./...", "summary": "Run tests"}, Cwd: testRoot})
	if r.Call != "Bash" || r.Input["description"] != "Run tests" || r.Input["summary"] != nil {
		t.Errorf("execute_bash = %+v", r)
	}
	wantDecision(t, Decide(r, testPolicy()), "")
	clash := NativeRequest("kiro", Request{Call: "execute_bash", Input: map[string]any{"command": "ls", "summary": "a", "description": "b"}, Cwd: testRoot})
	wantDecision(t, Decide(clash, testPolicy()), RuleInput)
	if r := NativeRequest("kiro", Request{Call: "read_file", Input: map[string]any{"path": "a"}}); r.Call != "Read" || r.Input["file_path"] != "a" {
		t.Errorf("read_file = %+v", r)
	}
	more := NativeRequest("kiro", Request{Call: "read_file", Input: map[string]any{"path": "a.md", "paths": []any{"/etc/passwd"}}, Cwd: testRoot})
	wantDecision(t, Decide(more, testPolicy()), RuleTool)
	for _, call := range []string{"fs_write", "str_replace", "grep_search", "Bash", "Write", "Grep"} {
		r := NativeRequest("kiro", Request{Call: call, Input: map[string]any{"file_path": testRoot + "/a.go", "pattern": "x"}, Cwd: testRoot})
		wantDecision(t, Decide(r, testPolicy()), RuleTool)
	}
	p := testPolicy()
	p.AlwaysAsk = []string{"Read(./docs/private/**)"}
	read := NativeRequest("kiro", Request{Call: "read_file", Input: map[string]any{"path": "docs/private/a.md"}, Cwd: testRoot})
	wantDecision(t, Decide(read, p), RuleAlwaysAsk)
	if r := NativeRequest("claude", Request{Call: "execute_bash"}); r.Call != "execute_bash" {
		t.Errorf("claude call mapped to %s", r.Call)
	}
}
