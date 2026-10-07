package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Sync keeps hook entries it did not write when a built-in hook reaches a
// target, and dropping the built-in takes out only its own (#1858).
func TestSync_KeepsHandWrittenHooks(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Clean(filepath.Join(packageDir, "..", ".."))
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	run := func(t *testing.T, dir string, args ...string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	cases := []struct {
		target, file, hand, builtins, marker string
	}{
		{"claude", ".claude/settings.json", `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "./guard.sh"}]}], "SessionStart": [{"hooks": [{"type": "command", "command": "./hello.sh"}]}]}}`, "handoff, handoff-hook", "handoff"},
		{"qoder", ".qoder/settings.json", `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "./guard.sh"}]}], "SessionStart": [{"hooks": [{"type": "command", "command": "./hello.sh"}]}]}}`, "handoff, handoff-hook", "handoff"},
		{"gemini", ".gemini/settings.json", `{"hooks": {"BeforeTool": [{"matcher": "run_shell_command", "hooks": [{"type": "command", "command": "./guard.sh"}]}], "SessionStart": [{"hooks": [{"type": "command", "command": "./hello.sh"}]}]}}`, "handoff, handoff-hook", "handoff"},
		{"codex", ".codex/hooks.json", `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "./guard.sh"}]}], "SessionStart": [{"hooks": [{"type": "command", "command": "./hello.sh"}]}]}}`, "handoff, handoff-hook", "handoff"},
		{"cursor", ".cursor/hooks.json", `{"version": 1, "hooks": {"preToolUse": [{"command": "./guard.sh"}], "sessionStart": [{"command": "./hello.sh"}]}}`, "memory", "hook memory"},
		{"factory", ".factory/hooks.json", `{"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "./guard.sh"}]}], "SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "./hello.sh"}]}]}`, "handoff, handoff-hook", "handoff"},
	}
	for _, c := range cases {
		t.Run(c.target, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			path := filepath.Join(dir, c.file)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(c.hand), 0o644); err != nil {
				t.Fatal(err)
			}
			config := func(builtins string) {
				body := "version: 1\ntargets: [" + c.target + "]\nbuiltins: [" + builtins + "]\n"
				if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			read := func() string {
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				return string(data)
			}
			config(c.builtins)
			run(t, dir, "sync", "--gitignore=off")
			run(t, dir, "sync", "--gitignore=off")
			got := read()
			for _, want := range []string{"./guard.sh", "./hello.sh", c.marker} {
				if !strings.Contains(got, want) {
					t.Errorf("after sync, %s lacks %s:\n%s", c.file, want, got)
				}
			}
			run(t, dir, "sync", "--check", "--gitignore=off")
			config("")
			run(t, dir, "sync", "--gitignore=off")
			got = read()
			if strings.Contains(got, c.marker) || !strings.Contains(got, "./guard.sh") || !strings.Contains(got, "./hello.sh") {
				t.Errorf("dropping the built-in should leave only the hand-written hooks:\n%s", got)
			}
		})
	}
	t.Run("a-file-sync-created-goes-with-its-last-hook", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
		for _, builtins := range []string{"handoff, handoff-hook", ""} {
			body := "version: 1\ntargets: [codex, factory]\nbuiltins: [" + builtins + "]\n"
			if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			run(t, dir, "sync", "--gitignore=off")
		}
		for _, file := range []string{".codex/hooks.json", ".factory/hooks.json"} {
			if _, err := os.Stat(filepath.Join(dir, file)); !os.IsNotExist(err) {
				t.Errorf("%s left behind: %v", file, err)
			}
		}
	})
	// import turns hand-written hooks into specs and leaves the entries
	// on disk; the next sync must not run them twice.
	for _, c := range cases {
		if c.target == "cursor" || c.target == "qoder" {
			continue // no hook import
		}
		t.Run("import-then-sync/"+c.target, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
			path := filepath.Join(dir, c.file)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(c.hand), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: ["+c.target+"]\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			run(t, dir, "import", c.target)
			run(t, dir, "sync", "--gitignore=off")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, command := range []string{"./guard.sh", "./hello.sh"} {
				if n := countHookCommands(t, data, command); n != 1 {
					t.Errorf("%s appears %d times, want once:\n%s", command, n, data)
				}
			}
		})
	}
}

// countHookCommands counts the handlers in a hooks file whose `command`
// runs script.
func countHookCommands(t *testing.T, data []byte, script string) int {
	t.Helper()
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	n := 0
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			if command, ok := x["command"].(string); ok && strings.HasSuffix(command, script) {
				n++
			}
			for _, child := range x {
				walk(child)
			}
		case []any:
			for _, child := range x {
				walk(child)
			}
		}
	}
	walk(doc)
	return n
}
