package cli

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/hookrun"
)

const approveAllowReply = `{"hookSpecificOutput":{"hookEventName":"PermissionRequest","decision":{"behavior":"allow"}}}`

// approveSetup makes a home config with yolo and a Git project, and
// keeps the run away from the real Claude Code settings.
func approveSetup(t *testing.T, shared string) (source, project string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("approvals fall back to ask on Windows")
	}
	source = approvalsHome(t, shared)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("CLAUDE_PROJECT_DIR", "")
	t.Setenv("KIRO_HOME", "")
	t.Setenv(adapters.HookTargetEnv, "")
	t.Setenv("QWEN_HOME", "")
	t.Setenv("QWEN_CODE_SYSTEM_DEFAULTS_PATH", filepath.Join(t.TempDir(), "system-defaults.json"))
	t.Setenv("QWEN_CODE_SYSTEM_SETTINGS_PATH", filepath.Join(t.TempDir(), "settings.json"))
	managed := t.TempDir()
	old := claudeManagedDir
	claudeManagedDir = managed
	t.Cleanup(func() { claudeManagedDir = old })
	project = filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	return source, project
}

const yoloHome = "version: 1\ntargets: [claude, codex]\napprovals:\n  mode: yolo\n"

func recordedPayload(t *testing.T, target, cwd, command string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "testdata", "hook_approve", target+"-bash.json"))
	if err != nil {
		t.Fatal(err)
	}
	quoted, _ := json.Marshal(command)
	cwdQuoted, _ := json.Marshal(cwd)
	text := strings.ReplaceAll(string(data), `"{{CWD}}"`, string(cwdQuoted))
	return strings.ReplaceAll(text, `"{{COMMAND}}"`, string(quoted))
}

// runApprove runs hook approve and fails unless it exits 0 with an empty
// stderr. It returns stdout.
func runApprove(t *testing.T, target, payload string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetIn(strings.NewReader(payload))
	root.SetArgs([]string{"hook", "approve", "--target", target})
	if err := root.Execute(); err != nil {
		t.Fatalf("hook approve exited with %v", err)
	}
	if stderr.Len() > 0 {
		t.Fatalf("hook approve wrote to stderr: %q", stderr.String())
	}
	return stdout.String()
}

func approveLog(t *testing.T, source string) []approvals.Entry {
	t.Helper()
	f, err := os.Open(filepath.Join(source, "local", "approvals", "decisions.jsonl"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var entries []approvals.Entry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e approvals.Entry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, e)
	}
	return entries
}

func TestHookApprove_AllowsGoTest(t *testing.T) {
	for _, target := range []string{"claude", "codex", "kiro", "qwen"} {
		t.Run(target, func(t *testing.T) {
			source, project := approveSetup(t, yoloHome)
			want, _ := hookrun.PermissionAllowReply(target)
			if got := runApprove(t, target, recordedPayload(t, target, project, "go test ./...")); got != string(want) {
				t.Fatalf("reply = %q, want %q", got, want)
			}
			entries := approveLog(t, source)
			if len(entries) != 1 || entries[0].Decision != approvals.Approved || entries[0].Tool != target || entries[0].Mode != "yolo" || entries[0].Session == "" {
				t.Errorf("log = %+v, want one approved line for %s", entries, target)
			}
		})
	}
}

func TestHookApprove_GitPushAsks(t *testing.T) {
	for _, target := range []string{"claude", "codex", "kiro", "qwen"} {
		t.Run(target, func(t *testing.T) {
			source, project := approveSetup(t, yoloHome)
			if got := runApprove(t, target, recordedPayload(t, target, project, "git push origin main")); got != "" {
				t.Fatalf("reply = %q, want nothing", got)
			}
			entries := approveLog(t, source)
			if len(entries) != 1 || entries[0].Decision != approvals.Asked || entries[0].Rule != approvals.RulePublish {
				t.Errorf("log = %+v, want one asked line with rule %s", entries, approvals.RulePublish)
			}
		})
	}
}

func TestHookApprove_PrintsNothing(t *testing.T) {
	cases := []struct {
		name, home, target string
		rule               string
		payload            func(project string) string
		setup              func(t *testing.T, source, project string)
	}{
		{name: "mode ask", home: "version: 1\napprovals:\n  mode: ask\n", target: "claude"},
		{name: "no approvals block", home: "version: 1\n", target: "codex"},
		{name: "review for the tool", home: yoloHome + "  tools:\n    codex: review\n", target: "codex"},
		{name: "unreadable config", home: "version: 1\napprovals: [\n", target: "claude"},
		{name: "unknown mode", home: "version: 1\napprovals:\n  mode: always\n", target: "claude"},
		{name: "unknown target", home: yoloHome, target: "copilot"},
		{name: "no target", home: yoloHome, target: ""},
		{name: "broken JSON", home: yoloHome, target: "claude", payload: func(string) string { return `{"tool_name":"Bash",` }},
		{name: "empty input", home: yoloHome, target: "codex", payload: func(string) string { return "" }},
		{name: "relative cwd", home: yoloHome, target: "claude", payload: func(string) string {
			return `{"hook_event_name":"PermissionRequest","cwd":"project","tool_name":"Bash","tool_input":{"command":"go test ./..."}}`
		}},
		{name: "other event", home: yoloHome, target: "claude", payload: func(project string) string {
			return `{"hook_event_name":"PreToolUse","cwd":` + strconvQuote(project) + `,"tool_name":"Bash","tool_input":{"command":"go test ./..."}}`
		}},
		{name: "environment says ask", home: yoloHome, target: "codex", setup: func(t *testing.T, _, _ string) {
			t.Helper()
			t.Setenv("AGNOSTIC_AI_APPROVALS", "ask")
		}},
		{name: "project file says ask", home: yoloHome, target: "claude", setup: func(t *testing.T, _, project string) {
			t.Helper()
			writeFile(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\napprovals:\n  mode: ask\n")
		}},
		{name: "claude project ask rule", home: yoloHome, target: "claude", rule: approvals.RuleAlwaysAsk, setup: func(t *testing.T, _, project string) {
			t.Helper()
			writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{"permissions":{"ask":["Bash(go test*)"]}}`)
		}},
		{name: "claude user ask rule", home: yoloHome, target: "claude", rule: approvals.RuleAlwaysAsk, setup: func(t *testing.T, _, _ string) {
			t.Helper()
			home, _ := os.UserHomeDir()
			writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"ask":["Bash(go *)"]}}`)
		}},
		{name: "claude local ask rule", home: yoloHome, target: "claude", rule: approvals.RuleAlwaysAsk, setup: func(t *testing.T, _, project string) {
			t.Helper()
			writeFile(t, filepath.Join(project, ".claude", "settings.local.json"), `{"permissions":{"ask":["Bash"]}}`)
		}},
		{name: "claude managed ask rule", home: yoloHome, target: "claude", rule: approvals.RuleAlwaysAsk, setup: func(t *testing.T, _, _ string) {
			t.Helper()
			writeFile(t, filepath.Join(claudeManagedDir, "managed-settings.json"), `{"permissions":{"ask":["Bash(go test:*)"]}}`)
		}},
		{name: "claude settings do not parse", home: yoloHome, target: "claude", setup: func(t *testing.T, _, project string) {
			t.Helper()
			writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{"permissions":`)
		}},
		{name: "kiro mode ask", home: "version: 1\napprovals:\n  mode: ask\n", target: "kiro"},
		{name: "kiro unreadable config", home: "version: 1\napprovals: [\n", target: "kiro"},
		{name: "kiro broken JSON", home: yoloHome, target: "kiro", payload: func(string) string { return `{"tool_name":"execute_bash",` }},
		{name: "kiro user ask rule", home: yoloHome, target: "kiro", rule: approvals.RuleToolSettings, setup: func(t *testing.T, _, _ string) {
			t.Helper()
			home, _ := os.UserHomeDir()
			writeFile(t, filepath.Join(home, ".kiro", "settings", "permissions.yaml"), "rules:\n  - capability: shell\n    match: [\"git *\"]\n    effect: ask\n")
		}},
		{name: "kiro workspace ask rule", home: yoloHome, target: "kiro", rule: approvals.RuleToolSettings, setup: func(t *testing.T, _, _ string) {
			t.Helper()
			home, _ := os.UserHomeDir()
			writeFile(t, filepath.Join(home, ".kiro", "workspace-roots", "ab12", "permissions.yaml"), "rules:\n  - capability: fs_write\n    effect: ask\n")
		}},
		{name: "kiro agent profile ask rule", home: yoloHome, target: "kiro", rule: approvals.RuleToolSettings, setup: func(t *testing.T, _, project string) {
			t.Helper()
			writeFile(t, filepath.Join(project, ".kiro", "agents", "dev.md"), "---\nname: dev\npermissions:\n  rules:\n    - capability: shell\n      effect: ask\n---\nBody\n")
		}},
		{name: "kiro rules do not parse", home: yoloHome, target: "kiro", rule: approvals.RuleToolSettings, setup: func(t *testing.T, _, _ string) {
			t.Helper()
			home, _ := os.UserHomeDir()
			writeFile(t, filepath.Join(home, ".kiro", "agents", "x.json"), `{"permissions":`)
		}},
		{name: "kiro write tool", home: yoloHome, target: "kiro", rule: approvals.RuleTool, payload: func(project string) string {
			return `{"hook_event_name":"PermissionRequest","cwd":` + strconvQuote(project) + `,"tool_name":"fs_write","tool_input":{"path":"a.txt"}}`
		}},
		{name: "qwen mode ask", home: "version: 1\napprovals:\n  mode: ask\n", target: "qwen"},
		{name: "qwen unreadable config", home: "version: 1\napprovals: [\n", target: "qwen"},
		{name: "qwen broken JSON", home: yoloHome, target: "qwen", payload: func(string) string { return `{"tool_name":"run_shell_command",` }},
		{name: "qwen plan mode", home: yoloHome, target: "qwen", rule: approvals.RulePermissionMode, payload: func(project string) string {
			return `{"hook_event_name":"PermissionRequest","cwd":` + strconvQuote(project) + `,"permission_mode":"plan","tool_name":"run_shell_command","tool_input":{"command":"go test ./...","is_background":false}}`
		}},
		{name: "qwen shell in another folder", home: yoloHome, target: "qwen", rule: approvals.RuleInput, payload: func(project string) string {
			return `{"hook_event_name":"PermissionRequest","cwd":` + strconvQuote(project) + `,"permission_mode":"default","tool_name":"run_shell_command","tool_input":{"command":"go test ./...","is_background":false,"directory":"/tmp"}}`
		}},
		{name: "qwen undocumented mode", home: yoloHome, target: "qwen", rule: approvals.RulePermissionMode, payload: func(project string) string {
			return `{"hook_event_name":"PermissionRequest","cwd":` + strconvQuote(project) + `,"permission_mode":"acceptEdits","tool_name":"run_shell_command","tool_input":{"command":"go test ./...","is_background":false}}`
		}},
		{name: "qwen relative home", home: yoloHome, target: "qwen", rule: approvals.RuleToolSettings, setup: func(t *testing.T, _, _ string) {
			t.Helper()
			t.Setenv("QWEN_HOME", "alt")
		}},
		{name: "qwen other tool", home: yoloHome, target: "qwen", rule: approvals.RuleTool, payload: func(project string) string {
			return `{"hook_event_name":"PermissionRequest","cwd":` + strconvQuote(project) + `,"permission_mode":"default","tool_name":"grep_search","tool_input":{"pattern":"x"}}`
		}},
		{name: "qwen user ask rule", home: yoloHome, target: "qwen", rule: approvals.RuleToolSettings, setup: func(t *testing.T, _, _ string) {
			t.Helper()
			home, _ := os.UserHomeDir()
			writeFile(t, filepath.Join(home, ".qwen", "settings.json"), "{\n  // mine\n  \"permissions\": {\"ask\": [\"Bash(git push *)\"]}\n}")
		}},
		{name: "qwen project ask rule", home: yoloHome, target: "qwen", rule: approvals.RuleToolSettings, setup: func(t *testing.T, _, project string) {
			t.Helper()
			writeFile(t, filepath.Join(project, ".qwen", "settings.json"), `{"permissions":{"ask":["run_shell_command"]}}`)
		}},
		{name: "qwen system ask rule", home: yoloHome, target: "qwen", rule: approvals.RuleToolSettings, setup: func(t *testing.T, _, _ string) {
			t.Helper()
			writeFile(t, os.Getenv("QWEN_CODE_SYSTEM_SETTINGS_PATH"), `{"permissions":{"ask":["edit"]}}`)
		}},
		{name: "qwen settings do not parse", home: yoloHome, target: "qwen", rule: approvals.RuleToolSettings, setup: func(t *testing.T, _, project string) {
			t.Helper()
			writeFile(t, filepath.Join(project, ".qwen", "settings.json"), `{"permissions":`)
		}},
		{name: "home always-ask", home: yoloHome + "  always-ask:\n    - \"Bash(go test*)\"\n", target: "codex", rule: approvals.RuleAlwaysAsk},
		{name: "log cannot be written", home: yoloHome, target: "codex", setup: func(t *testing.T, source, _ string) {
			t.Helper()
			writeFile(t, filepath.Join(source, "local", "approvals"), "not a folder")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source, project := approveSetup(t, tc.home)
			if tc.setup != nil {
				tc.setup(t, source, project)
			}
			target := tc.target
			payload := recordedPayload(t, "claude", project, "go test ./...")
			if target == "codex" || target == "kiro" || target == "qwen" {
				payload = recordedPayload(t, target, project, "go test ./...")
			}
			if tc.payload != nil {
				payload = tc.payload(project)
			}
			if got := runApprove(t, target, payload); got != "" {
				t.Fatalf("reply = %q, want nothing", got)
			}
			if tc.name == "log cannot be written" {
				return
			}
			entries := approveLog(t, source)
			for _, e := range entries {
				if e.Decision == approvals.Approved {
					t.Errorf("log holds an approval: %+v", e)
				}
			}
			if tc.rule != "" && (len(entries) != 1 || entries[0].Rule != tc.rule) {
				t.Errorf("log = %+v, want one asked line with rule %s", entries, tc.rule)
			}
		})
	}
}

func TestHookApprove_UnreadableClaudeSettingsLogTheRule(t *testing.T) {
	source, project := approveSetup(t, yoloHome)
	writeFile(t, filepath.Join(project, ".claude", "settings.json"), `not json`)
	if got := runApprove(t, "claude", recordedPayload(t, "claude", project, "go test ./...")); got != "" {
		t.Fatalf("reply = %q, want nothing", got)
	}
	if entries := approveLog(t, source); len(entries) != 1 || entries[0].Rule != approvals.RuleToolSettings {
		t.Errorf("log = %+v, want one asked line with rule %s", entries, approvals.RuleToolSettings)
	}
}

func TestHookApprove_CodexIgnoresClaudeSettings(t *testing.T) {
	_, project := approveSetup(t, yoloHome)
	writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{"permissions":{"ask":["Bash"]}}`)
	if got := runApprove(t, "codex", recordedPayload(t, "codex", project, "go test ./...")); got != approveAllowReply {
		t.Fatalf("reply = %q, want the allow reply", got)
	}
}

func TestHookApprove_TargetFromEnvironment(t *testing.T) {
	_, project := approveSetup(t, yoloHome)
	t.Setenv(adapters.HookTargetEnv, "codex")
	if got := runApprove(t, "", recordedPayload(t, "codex", project, "go test ./...")); got != approveAllowReply {
		t.Fatalf("reply = %q, want the allow reply", got)
	}
}

func TestHookApprove_BadFlagStaysSilent(t *testing.T) {
	_, project := approveSetup(t, yoloHome)
	for _, args := range [][]string{{"--target"}, {"-t"}, {"--target", "claude", "--verbose=x"}} {
		var stdout, stderr bytes.Buffer
		root := NewRootCmd("test")
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		root.SetIn(strings.NewReader(recordedPayload(t, "claude", project, "go test ./...")))
		root.SetArgs(append([]string{"hook", "approve"}, args...))
		if err := root.Execute(); err != nil || stderr.Len() > 0 || stdout.Len() > 0 {
			t.Errorf("%v: err %v, stdout %q, stderr %q; want exit 0 and nothing", args, err, stdout.String(), stderr.String())
		}
	}
}

func TestHookApprove_ClaudeRulesFromASubfolderSession(t *testing.T) {
	_, project := approveSetup(t, yoloHome)
	pkg := filepath.Join(project, "pkg")
	writeFile(t, filepath.Join(pkg, ".claude", "settings.json"), `{"permissions":{"ask":["Write(/src/**)"]}}`)
	t.Setenv("CLAUDE_PROJECT_DIR", pkg)
	payload := `{"hook_event_name":"PermissionRequest","session_id":"s","cwd":` + strconvQuote(pkg) + `,"permission_mode":"default","tool_name":"Write","tool_input":{"file_path":` + strconvQuote(filepath.Join(pkg, "src", "a.go")) + `,"content":"x"}}`
	if got := runApprove(t, "claude", payload); got != "" {
		t.Fatalf("reply = %q, want nothing: the rule is anchored at the folder of its settings file", got)
	}
	other := strings.Replace(payload, filepath.Join(pkg, "src", "a.go"), filepath.Join(pkg, "docs", "a.md"), 1)
	if got := runApprove(t, "claude", other); got != approveAllowReply {
		t.Fatalf("reply = %q, want the allow reply for a path no rule names", got)
	}
}

func TestHookApprove_ClaudeRulesThroughASymlinkedFolder(t *testing.T) {
	_, project := approveSetup(t, yoloHome)
	writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{"permissions":{"ask":["Bash(go test*)"]}}`)
	writeFile(t, filepath.Join(project, "pkg", "a.go"), "package a\n")
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(filepath.Join(project, "pkg"), link); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	if got := runApprove(t, "claude", recordedPayload(t, "claude", link, "go test ./...")); got != "" {
		t.Fatalf("reply = %q, want nothing", got)
	}
}

func TestHookApprove_ProjectEnvThatMovesThePolicyAsks(t *testing.T) {
	source, project := approveSetup(t, yoloHome)
	writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{"env":{"AGNOSTIC_AI_HOME":"/tmp/elsewhere"}}`)
	if got := runApprove(t, "claude", recordedPayload(t, "claude", project, "go test ./...")); got != "" {
		t.Fatalf("reply = %q, want nothing", got)
	}
	if entries := approveLog(t, source); len(entries) != 1 || entries[0].Rule != approvals.RuleToolSettings {
		t.Errorf("log = %+v, want one asked line with rule %s", entries, approvals.RuleToolSettings)
	}
}

func TestHookApprove_UnknownFlagAndArgsStaySilent(t *testing.T) {
	_, project := approveSetup(t, yoloHome)
	var stdout, stderr bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetIn(strings.NewReader(recordedPayload(t, "claude", project, "go test ./...")))
	root.SetArgs([]string{"hook", "approve", "--target", "claude", "--from-a-newer-release", "extra"})
	if err := root.Execute(); err != nil || stderr.Len() > 0 {
		t.Fatalf("err = %v, stderr = %q; want exit 0 and no stderr", err, stderr.String())
	}
	if stdout.String() != approveAllowReply {
		t.Errorf("reply = %q, want the allow reply", stdout.String())
	}
}

func strconvQuote(s string) string {
	quoted, _ := json.Marshal(s)
	return string(quoted)
}

func TestHookApprove_KiroReadsAFileInTheProject(t *testing.T) {
	source, project := approveSetup(t, yoloHome)
	writeFile(t, filepath.Join(project, ".kiro", "agents", "plain.json"), `{"name":"plain","permissions":{"rules":[{"capability":"shell","effect":"allow"}]}}`)
	payload := `{"hook_event_name":"PermissionRequest","cwd":` + strconvQuote(project) + `,"session_id":"k","tool_name":"read_file","tool_input":{"path":` + strconvQuote(filepath.Join(project, "README.md")) + `},"permission":{"capability":"fs_read"}}`
	want, _ := hookrun.PermissionAllowReply("kiro")
	if got := runApprove(t, "kiro", payload); got != string(want) {
		t.Fatalf("reply = %q, want %q", got, want)
	}
	if entries := approveLog(t, source); len(entries) != 1 || entries[0].Tool != "kiro" || entries[0].Call != "read_file" || entries[0].Detail == "" {
		t.Errorf("log = %+v, want one kiro read_file line with a detail", entries)
	}
}

func TestHookApprove_KiroLogsTheCommand(t *testing.T) {
	source, project := approveSetup(t, yoloHome)
	runApprove(t, "kiro", recordedPayload(t, "kiro", project, "go test ./..."))
	if entries := approveLog(t, source); len(entries) != 1 || entries[0].Call != "execute_bash" || !strings.Contains(entries[0].Detail, "go test") {
		t.Errorf("log = %+v, want the execute_bash command as the detail", entries)
	}
}

func TestHookApprove_QwenAutoEditAndEdits(t *testing.T) {
	source, project := approveSetup(t, yoloHome)
	writeFile(t, filepath.Join(os.Getenv("HOME"), ".qwen", "settings.json"), `{"permissions":{"allow":["run_shell_command"]}}`)
	payload := `{"hook_event_name":"PermissionRequest","cwd":` + strconvQuote(project) + `,"permission_mode":"auto_edit","session_id":"q","tool_name":"edit","tool_input":{"file_path":` + strconvQuote(filepath.Join(project, "main.go")) + `,"old_string":"a","new_string":"b"}}`
	want, _ := hookrun.PermissionAllowReply("qwen")
	if got := runApprove(t, "qwen", payload); got != string(want) {
		t.Fatalf("reply = %q, want %q", got, want)
	}
	if entries := approveLog(t, source); len(entries) != 1 || entries[0].Call != "edit" {
		t.Errorf("log = %+v, want one qwen edit line", entries)
	}
}

func TestHookApprove_KiroReadsRulesFromOverride(t *testing.T) {
	_, project := approveSetup(t, yoloHome)
	root := filepath.Join(t.TempDir(), "kiro[profile]")
	t.Setenv("KIRO_HOME", root)
	writeFile(t, filepath.Join(root, "workspace-roots", "workspace", "permissions.yaml"), "rules:\n  - capability: shell\n    effect: ask\n")
	if got := runApprove(t, "kiro", recordedPayload(t, "kiro", project, "go test ./...")); got != "" {
		t.Errorf("reply = %q, want no reply when the active Kiro profile has an ask rule", got)
	}
}

func TestHookApprove_KiroKeepsPromptWhenRuleDiscoveryFails(t *testing.T) {
	_, project := approveSetup(t, yoloHome)
	t.Setenv("KIRO_HOME", "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(home, ".kiro", "workspace-roots"), "unreadable as a directory")
	if got := runApprove(t, "kiro", recordedPayload(t, "kiro", project, "go test ./...")); got != "" {
		t.Errorf("reply = %q, want no reply when Kiro workspace policies could not be listed", got)
	}
}

func TestHookApprove_MovedSpecFoldersAsk(t *testing.T) {
	source, project := approveSetup(t, yoloHome)
	writeFile(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\nsources:\n  hooks: config/hooks\n")
	write := func(rel string) string {
		payload, err := json.Marshal(map[string]any{
			"session_id": "s", "cwd": project, "permission_mode": "default", "hook_event_name": "PermissionRequest",
			"tool_name": "Write", "tool_input": map[string]any{"file_path": filepath.Join(project, rel), "content": "x"},
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(payload)
	}
	if got := runApprove(t, "claude", write("notes.md")); got != approveAllowReply {
		t.Fatalf("plain edit reply = %q, want the allow reply", got)
	}
	for _, rel := range []string{"config/hooks/guard.yaml", ".agnostic-ai/agents/reviewer.md"} {
		if got := runApprove(t, "claude", write(rel)); got != "" {
			t.Errorf("%s reply = %q, want nothing", rel, got)
		}
	}
	writeFile(t, filepath.Join(project, "agnostic-ai.local.yaml"), "sources:\n  hooks: [a, b]\n")
	if got := runApprove(t, "claude", write("notes.md")); got != "" {
		t.Errorf("broken project config reply = %q, want nothing", got)
	}
	entries := approveLog(t, source)
	rules := []string{}
	for _, e := range entries {
		rules = append(rules, e.Rule)
	}
	want := []string{"", approvals.RuleProtectedPath, approvals.RuleProtectedPath, approvals.RuleProjectConfig}
	if strings.Join(rules, ",") != strings.Join(want, ",") {
		t.Errorf("logged rules = %v, want %v", rules, want)
	}
}
