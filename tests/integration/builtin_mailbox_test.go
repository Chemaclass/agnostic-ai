package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	mailboxClaudeSession = "4e8b1c2d-9a3f-4b6e-8c7d-1f2a3b4c5d6e"
	mailboxCodexSession  = "0199a2b4-5c1e-7d30-9f2a-3b6c8d1e4f50"
)

func TestBuiltinMailbox(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := builtBinary(t)
	fixture, err := os.ReadFile(filepath.Join(packageDir, "fixtures", "builtin-mailbox", "agnostic-ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AppData", filepath.Join(home, "AppData"))
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	dir := t.TempDir()
	run := func(t *testing.T, args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	config := filepath.Join(dir, "agnostic-ai.yaml")
	if err := os.WriteFile(config, fixture, 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, "sync", "--gitignore=off")

	emitted := []string{".claude/settings.json", ".codex/hooks.json"}
	skills := []string{".claude/skills/mailbox/SKILL.md", ".agents/skills/mailbox/SKILL.md"}
	emitted = append(emitted, skills...)
	output := map[string]string{}
	for _, path := range emitted {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		output[path] = string(data)
	}
	expectedDir := filepath.Join(packageDir, "fixtures", "golden", "builtin-mailbox")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		updateGolden(t, expectedDir, output)
	} else {
		compareGolden(t, expectedDir, output, "builtin-mailbox")
	}
	run(t, "sync", "--check", "--gitignore=off")
	if output[skills[0]] != output[skills[1]] {
		t.Errorf("the mailbox skill differs between tools:\n%s\n---\n%s", output[skills[0]], output[skills[1]])
	}

	t.Run("emitted-commands-deliver", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("the emitted Unix launcher is what runs here")
		}
		configDir, err := os.UserConfigDir()
		if err != nil {
			t.Fatal(err)
		}
		threads := filepath.Join(configDir, "agnostic-ai", "mailbox", "threads")
		for _, tc := range []struct {
			hook, target, session, from, fromSession, payload string
			delivers                                          bool
		}{
			{"mailbox-user-prompt", "claude", mailboxClaudeSession, "codex", mailboxCodexSession, `"hook_event_name":"UserPromptSubmit"`, true},
			{"mailbox-session-start", "claude", mailboxClaudeSession, "codex", mailboxCodexSession, `"hook_event_name":"SessionStart","source":"compact"`, false},
			{"mailbox-session-start", "claude", mailboxClaudeSession, "codex", mailboxCodexSession, `"hook_event_name":"SessionStart","source":"resume"`, true},
			{"mailbox-user-prompt", "codex", mailboxCodexSession, "claude", mailboxClaudeSession, `"hook_event_name":"UserPromptSubmit"`, true},
			{"mailbox-session-start", "codex", mailboxCodexSession, "claude", mailboxClaudeSession, `"hook_event_name":"SessionStart","source":"startup"`, true},
		} {
			t.Run(tc.hook+"-"+tc.target, func(t *testing.T) {
				id := seedMailboxThread(t, threads, tc.from, tc.fromSession, tc.target, tc.session)
				payload := filepath.Join(t.TempDir(), "payload.json")
				body := fmt.Sprintf(`{"session_id":%q,%s}`, tc.session, tc.payload)
				if err := os.WriteFile(payload, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				out := run(t, "hook", "run", tc.hook, "--target", tc.target, "--payload", payload, "--format", "json")
				delivered := strings.Contains(out, `"adds_context": true`) && strings.Contains(out, id)
				if delivered != tc.delivers {
					t.Errorf("delivered = %t, want %t:\n%s", delivered, tc.delivers, out)
				}
				if err := os.Remove(filepath.Join(threads, id+".json")); err != nil {
					t.Fatal(err)
				}
			})
		}
	})

	if err := os.WriteFile(config, []byte("version: 1\ntargets: [claude, codex]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, "sync", "--gitignore=off")
	for _, path := range skills {
		if _, err := os.Stat(filepath.Join(dir, path)); !os.IsNotExist(err) {
			t.Errorf("%s remains after the built-in is off: %v", path, err)
		}
	}
	for _, path := range emitted[:2] {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err == nil && strings.Contains(string(data), "hook mailbox") {
			t.Errorf("%s keeps the mailbox hook after the built-in is off:\n%s", path, data)
		}
	}
	run(t, "sync", "--check", "--gitignore=off")
}

var mailboxThreadCount int

func seedMailboxThread(t *testing.T, threads, fromTool, fromID, toTool, toID string) string {
	t.Helper()
	mailboxThreadCount++
	id := fmt.Sprintf("th_20261121_%08x", mailboxThreadCount)
	thread := fmt.Sprintf(`{"schemaVersion":"mailbox/v1","id":%q,"status":"open","cap":8,
"between":[{"tool":%q,"id":%q},{"tool":%q,"id":%q}],
"messages":[{"n":1,"type":"question","from":{"tool":%q,"id":%q,"by":"human"},"to":{"tool":%q,"id":%q},
"body":"Does the store lock cover the claims file too?","state":"waiting","at":"2026-11-21T10:02:11Z","queuedAt":"2026-11-21T10:03:40Z"}],
"events":[{"at":"2026-11-21T10:02:11Z","by":"human","action":"send","n":1}]}`,
		id, fromTool, fromID, toTool, toID, fromTool, fromID, toTool, toID)
	if err := os.MkdirAll(threads, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(threads, id+".json"), []byte(thread), 0o600); err != nil {
		t.Fatal(err)
	}
	return id
}
