package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuiltinTools_EnableAndRemoveIndependently(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Clean(filepath.Join(packageDir, "..", ".."))
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	dir := t.TempDir()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	run := func(args ...string) string {
		t.Helper()
		command := exec.Command(binary, args...)
		command.Dir = dir
		out, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("init", "--all", "--gitignore=off")
	data, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte("rtk")) || bytes.Contains(data, []byte("caveman")) {
		t.Fatalf("init opted into tools: %s", data)
	}
	settings := filepath.Join(dir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"env":{"KEEP":"sentinel"},"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"./unrelated.sh"}]}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	const nativeHook = "command -v rtk >/dev/null 2>&1 || exit 0; rtk hook claude"
	var emittedHook string
	for _, names := range []string{"", "rtk", "caveman", "rtk, caveman", "rtk", "rtk, caveman", "caveman", ""} {
		config := "version: 1\ntargets: [claude, codex]\nbuiltins: [" + names + "]\n"
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
		run("sync", "--gitignore=off")
		data, err := os.ReadFile(settings)
		if err != nil {
			t.Fatal(err)
		}
		wantHooks := 0
		if strings.Contains(names, "rtk") {
			wantHooks = 1
			emittedHook = nativeHook
		}
		if count := countHookCommands(t, data, nativeHook); count != wantHooks {
			t.Errorf("%q: hook count %d", names, count)
		}
		if !bytes.Contains(data, []byte("./unrelated.sh")) || !bytes.Contains(data, []byte("sentinel")) {
			t.Errorf("%q lost unrelated settings: %s", names, data)
		}
		for _, target := range []string{".claude", ".agents"} {
			for _, asset := range []string{"SKILL.md", "LICENSE", "LICENSE-MIT", "NOTICE", "LICENSING.md", "UPSTREAM-README.md"} {
				path := filepath.Join(dir, target, "skills", "caveman", asset)
				_, err := os.Stat(path)
				if strings.Contains(names, "caveman") && err != nil || !strings.Contains(names, "caveman") && !os.IsNotExist(err) {
					t.Errorf("%q %s: %v", names, path, err)
				}
			}
		}
		if data, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks.json")); err == nil && bytes.Contains(data, []byte("rtk hook")) {
			t.Errorf("RTK reached Codex: %s", data)
		}
		run("sync", "--check", "--gitignore=off")
	}
	t.Run("installed-native-rtk", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("native hook requires POSIX shell")
		}
		rtk, err := exec.LookPath("rtk")
		if err != nil {
			t.Skip("optional native RTK protocol probe requires an existing RTK installation")
		}
		stores := t.TempDir()
		for _, command := range []string{"git status", "printf unsupported", "rtk git status"} {
			request, err := json.Marshal(map[string]any{
				"hook_event_name": "PreToolUse", "tool_name": "Bash", "permission_mode": "default",
				"tool_input": map[string]any{"command": command, "description": "keep description", "timeout": 1000},
			})
			if err != nil {
				t.Fatal(err)
			}
			hook := exec.Command("/bin/sh", "-c", emittedHook)
			hook.Dir = dir
			hook.Env = append(os.Environ(),
				"PATH="+filepath.Dir(rtk),
				"CLAUDE_CONFIG_DIR="+stores,
				"RTK_DB_PATH="+filepath.Join(stores, "tracking.db"),
				"RTK_RECALL_DB="+filepath.Join(stores, "recall.db"),
				"RTK_TEE_DIR="+filepath.Join(stores, "tee"),
			)
			hook.Stdin = bytes.NewReader(request)
			out, err := hook.Output()
			if err != nil {
				t.Fatalf("native hook %q: %v", command, err)
			}
			if command != "git status" {
				if len(out) != 0 {
					t.Errorf("%q should have no replacement: %s", command, out)
				}
				continue
			}
			var reply struct {
				Output struct {
					Event string `json:"hookEventName"`
					Input struct {
						Command     string `json:"command"`
						Description string `json:"description"`
						Timeout     int    `json:"timeout"`
					} `json:"updatedInput"`
				} `json:"hookSpecificOutput"`
			}
			if err := json.Unmarshal(out, &reply); err != nil {
				t.Fatal(err)
			}
			if reply.Output.Event != "PreToolUse" || reply.Output.Input.Command != "rtk git status" || reply.Output.Input.Description != "keep description" || reply.Output.Input.Timeout != 1000 {
				t.Errorf("native RTK reply lost rewrite or input fields: %s", out)
			}
		}
	})
}
