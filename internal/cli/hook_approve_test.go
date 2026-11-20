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
	t.Setenv(adapters.HookTargetEnv, "")
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
	for _, target := range []string{"claude", "codex"} {
		t.Run(target, func(t *testing.T) {
			source, project := approveSetup(t, yoloHome)
			if got := runApprove(t, target, recordedPayload(t, target, project, "go test ./...")); got != approveAllowReply {
				t.Fatalf("reply = %q, want %q", got, approveAllowReply)
			}
			entries := approveLog(t, source)
			if len(entries) != 1 || entries[0].Decision != approvals.Approved || entries[0].Tool != target || entries[0].Mode != "yolo" || entries[0].Session == "" {
				t.Errorf("log = %+v, want one approved line for %s", entries, target)
			}
		})
	}
}

func TestHookApprove_GitPushAsks(t *testing.T) {
	for _, target := range []string{"claude", "codex"} {
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
			t.Setenv("AGNOSTIC_AI_APPROVALS", "ask")
		}},
		{name: "project file says ask", home: yoloHome, target: "claude", setup: func(t *testing.T, _, project string) {
			writeFile(t, filepath.Join(project, "agnostic-ai.yaml"), "version: 1\napprovals:\n  mode: ask\n")
		}},
		{name: "claude project ask rule", home: yoloHome, target: "claude", rule: approvals.RuleAlwaysAsk, setup: func(t *testing.T, _, project string) {
			writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{"permissions":{"ask":["Bash(go test*)"]}}`)
		}},
		{name: "claude user ask rule", home: yoloHome, target: "claude", rule: approvals.RuleAlwaysAsk, setup: func(t *testing.T, _, _ string) {
			home, _ := os.UserHomeDir()
			writeFile(t, filepath.Join(home, ".claude", "settings.json"), `{"permissions":{"ask":["Bash(go *)"]}}`)
		}},
		{name: "claude local ask rule", home: yoloHome, target: "claude", rule: approvals.RuleAlwaysAsk, setup: func(t *testing.T, _, project string) {
			writeFile(t, filepath.Join(project, ".claude", "settings.local.json"), `{"permissions":{"ask":["Bash"]}}`)
		}},
		{name: "claude managed ask rule", home: yoloHome, target: "claude", rule: approvals.RuleAlwaysAsk, setup: func(t *testing.T, _, _ string) {
			writeFile(t, filepath.Join(claudeManagedDir, "managed-settings.json"), `{"permissions":{"ask":["Bash(go test:*)"]}}`)
		}},
		{name: "claude settings do not parse", home: yoloHome, target: "claude", setup: func(t *testing.T, _, project string) {
			writeFile(t, filepath.Join(project, ".claude", "settings.json"), `{"permissions":`)
		}},
		{name: "home always-ask", home: yoloHome + "  always-ask:\n    - \"Bash(go test*)\"\n", target: "codex", rule: approvals.RuleAlwaysAsk},
		{name: "log cannot be written", home: yoloHome, target: "codex", setup: func(t *testing.T, source, _ string) {
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
			if target == "codex" {
				payload = recordedPayload(t, "codex", project, "go test ./...")
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
