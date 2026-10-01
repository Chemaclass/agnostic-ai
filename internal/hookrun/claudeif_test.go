package hookrun

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

func toolBody(t *testing.T, tool string, input map[string]any) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"tool_name": tool, "tool_input": input})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// Rows come from code.claude.com/docs/en/hooks (the `if` field) and
// code.claude.com/docs/en/permissions (permission rule syntax).
func TestClaudeIfRuns_BashRules(t *testing.T) {
	for _, tc := range []struct {
		rule, command string
		want          bool
	}{
		{"Bash(git *)", "FOO=bar git push", true},
		{"Bash(git *)", "npm test && git push", true},
		{"Bash(rm *)", "echo $(rm -rf /)", true},
		{"Bash(rm *)", "echo `rm -rf /`", true},
		{"Bash(rm *)", "echo $(date)", false},
		{"Bash(git push *)", "echo $(date)", true},
		{"Bash(npm run build)", "npm run build", true},
		{"Bash(npm run build)", "npm run build --watch", false},
		{"Bash(npm run *)", "npm run", true},
		{"Bash(npm run *)", "npm install", false},
		{"Bash(git log * main)", "git log --oneline main", true},
		{"Bash(git log * main)", "git log main", false},
		{"Bash(* --version)", "node --version", true},
		{"Bash(ls *)", "ls", true},
		{"Bash(ls *)", "lsof", false},
		{"Bash(ls*)", "lsof", true},
		{"Bash(* --help *)", "npm --help", false},
		{"Bash(ls:*)", "ls -la", true},
		{"Bash(npm test *)", "timeout 30 npm test", true},
		{"Bash(git *)", "echo 'git push'", false},
		{"Bash(git *)", "ls | grep x; git status", true},
		{"Bash(git *)", "npm test &&", true},
		{"Bash", "anything at all", true},
		{"Bash(*)", "anything at all", true},
	} {
		got, err := ClaudeIfRuns(tc.rule, "PreToolUse", toolBody(t, "Bash", map[string]any{"command": tc.command}), "/p")
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s on %q = %v, want %v", tc.rule, tc.command, got, tc.want)
		}
	}
}

func TestClaudeIfRuns_FileRules(t *testing.T) {
	root := filepath.FromSlash("/p")
	for _, tc := range []struct {
		rule, tool, file string
		want             bool
	}{
		{"Edit(*.ts)", "Write", "/p/src/app.ts", true},
		{"Edit(*.ts)", "Edit", "/p/src/app.go", false},
		{"Edit(/src/**)", "Edit", "/p/src/x/a.ts", true},
		{"Edit(/src/**)", "Edit", "/p/vendor/src/a.ts", false},
		{"Edit(**/src/**)", "MultiEdit", "/p/vendor/pkg/src/lib.js", true},
		{"Edit(src/**)", "Edit", "/p/vendor/pkg/src/lib.js", false},
		{"Edit(src/components/**)", "Edit", "/p/vendor/src/components/a.ts", false},
		{"Edit(.env)", "Write", "/p/a/b/.env", true},
		{"Edit(//tmp/scratch.txt)", "Write", "/tmp/scratch.txt", true},
		{"Edit(./docs/*.md)", "Write", "/p/docs/a.md", true},
		{"Edit(./docs/*.md)", "Write", "/p/docs/x/a.md", false},
		{"Edit(docs/**)", "Bash", "", false},
		{"Write", "Write", "/p/a", true},
		{"Write", "Edit", "/p/a", false},
	} {
		input := map[string]any{"command": "ls"}
		if tc.file != "" {
			input = map[string]any{"file_path": filepath.FromSlash(tc.file)}
		}
		got, err := ClaudeIfRuns(tc.rule, "PostToolUse", toolBody(t, tc.tool, input), root)
		if err != nil {
			t.Fatal(err)
		}
		if got != tc.want {
			t.Errorf("%s on %s %s = %v, want %v", tc.rule, tc.tool, tc.file, got, tc.want)
		}
	}
}

func TestClaudeIfRuns_OnlyOnToolEvents(t *testing.T) {
	body := toolBody(t, "Bash", map[string]any{"command": "git push"})
	if got, _ := ClaudeIfRuns("Bash(git *)", "SessionStart", body, "/p"); got {
		t.Error("a hook with if ran on SessionStart")
	}
	if got, _ := ClaudeIfRuns("", "SessionStart", body, "/p"); !got {
		t.Error("a hook without if did not run")
	}
	if got, _ := ClaudeIfRuns("Bash(git *)", "PermissionRequest", body, "/p"); !got {
		t.Error("if was not evaluated on PermissionRequest")
	}
}

func TestClaudeIfRuns_ParameterRule(t *testing.T) {
	body := toolBody(t, "Bash", map[string]any{"command": "sleep 9", "run_in_background": true})
	if got, _ := ClaudeIfRuns("Bash(run_in_background:true)", "PreToolUse", body, "/p"); !got {
		t.Error("parameter rule did not match")
	}
	if got, _ := ClaudeIfRuns("Bash(timeout:*)", "PreToolUse", body, "/p"); got {
		t.Error("a parameter the call leaves unset matched")
	}
}
