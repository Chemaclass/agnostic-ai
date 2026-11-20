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

func TestNativeRequest_Qwen(t *testing.T) {
	shell := NativeRequest("qwen", Request{Call: "run_shell_command", Input: map[string]any{"command": "go test ./...", "is_background": false, "description": "x"}, PermissionMode: "default", Cwd: testRoot})
	if shell.Call != "Bash" || shell.Input["run_in_background"] != false {
		t.Errorf("run_shell_command = %+v", shell)
	}
	wantDecision(t, Decide(shell, testPolicy()), "")
	dir := NativeRequest("qwen", Request{Call: "run_shell_command", Input: map[string]any{"command": "ls", "is_background": false, "directory": "/tmp"}, Cwd: testRoot})
	wantDecision(t, Decide(dir, testPolicy()), RuleInput)
	for mode, rule := range map[string]string{"default": "", "auto_edit": "", "plan": RulePermissionMode, "auto": RulePermissionMode, "yolo": RulePermissionMode} {
		r := NativeRequest("qwen", Request{Call: "read_file", Input: map[string]any{"file_path": testRoot + "/a.go"}, PermissionMode: mode, Cwd: testRoot})
		wantDecision(t, Decide(r, testPolicy()), rule)
	}
	for call, want := range map[string]string{"edit": "Edit", "write_file": "Write", "read_file": "Read", "grep_search": "", "Bash": "", "web_fetch": ""} {
		if r := NativeRequest("qwen", Request{Call: call}); r.Call != want {
			t.Errorf("%s mapped to %q, want %q", call, r.Call, want)
		}
	}
}
