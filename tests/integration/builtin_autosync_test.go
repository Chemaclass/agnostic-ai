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

// autosyncProject syncs a project with the autosync built-in and returns
// its directory, the built binary and a runner for the CLI in it.
func autosyncProject(t *testing.T) (string, func(t *testing.T, args ...string) string) {
	t.Helper()
	binary := builtBinary(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("AppData", filepath.Join(home, "AppData"))
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	t.Setenv("CLAUDE_CODE_REMOTE", "")
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
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude, codex, cursor, copilot]\nbuiltins: [autosync]\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), []byte("---\nname: style\ndescription: Style.\n---\nWrite short functions.\n"), 0o644))
	run(t, "sync", "--gitignore=off")
	return dir, run
}

// claudeAutosyncCommand returns the command .claude/settings.json runs at
// session start.
func claudeAutosyncCommand(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatal(err)
	}
	for _, group := range settings.Hooks["SessionStart"] {
		for _, hook := range group.Hooks {
			if strings.Contains(hook.Command, "hook autosync") {
				return hook.Command
			}
		}
	}
	t.Fatalf("no autosync SessionStart hook in .claude/settings.json:\n%s", data)
	return ""
}

// runSessionStart runs command as Claude Code does, from dir, and returns
// its stdout and stderr.
func runSessionStart(t *testing.T, dir, command string) (string, string) {
	t.Helper()
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "AGNOSTIC_AI_TARGET=claude")
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("session-start hook failed: %v\n%s", err, stderr.String())
	}
	return stdout.String(), stderr.String()
}

func TestBuiltinAutosync_EmitsSessionStartHooks(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir, run := autosyncProject(t)
	output := map[string]string{}
	for _, path := range []string{".claude/settings.json", ".codex/hooks.json", ".cursor/hooks.json", ".github/hooks/agnostic-ai.json"} {
		data, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Fatal(err)
		}
		output[path] = string(data)
	}
	expectedDir := filepath.Join(packageDir, "fixtures", "golden", "builtin-autosync")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		updateGolden(t, expectedDir, output)
	} else {
		compareGolden(t, expectedDir, output, "builtin-autosync")
	}
	var copilot struct {
		Hooks map[string][]struct {
			PowerShell string `json:"powershell"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(output[".github/hooks/agnostic-ai.json"]), &copilot); err != nil {
		t.Fatal(err)
	}
	if starts := copilot.Hooks["SessionStart"]; len(starts) != 1 || !strings.Contains(starts[0].PowerShell, "hook autosync --target copilot > $null; exit 0") {
		t.Errorf("Copilot hook has no PowerShell form:\n%s", output[".github/hooks/agnostic-ai.json"])
	}
	if !strings.Contains(output[".codex/hooks.json"], "hook autosync --target codex > $null; exit 0") {
		t.Errorf("Codex hook has no commandWindows:\n%s", output[".codex/hooks.json"])
	}
	run(t, "sync", "--check", "--gitignore=off")
	run(t, "validate")
	run(t, "lint")
}

func TestBuiltinAutosync_SessionStartRestoresMissingFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the emitted POSIX command is what runs here")
	}
	dir, _ := autosyncProject(t)
	command := claudeAutosyncCommand(t, dir)
	must(t, os.Remove(filepath.Join(dir, "AGENTS.md")))
	must(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))

	stdout, stderr := runSessionStart(t, filepath.Join(dir, "sub"), command)
	if stdout != "" || stderr != "" {
		t.Errorf("a clean sync printed stdout %q stderr %q, want nothing", stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Errorf("AGENTS.md not restored: %v", err)
	}

	must(t, os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("hand edit\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai", "rules", "style.md"), []byte("---\nname: style\ndescription: Style.\n---\nWrite shorter functions.\n"), 0o644))
	stdout, stderr = runSessionStart(t, dir, command)
	if stdout != "" || !strings.Contains(stderr, "~ kept AGENTS.md") {
		t.Errorf("stdout %q stderr %q, want the kept file on stderr only", stdout, stderr)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md")); string(data) != "hand edit\n" {
		t.Errorf("the hand edit was overwritten: %q", data)
	}
	if data, _ := os.ReadFile(filepath.Join(dir, ".claude", "rules", "style.md")); !strings.Contains(string(data), "shorter") {
		t.Errorf("the other outputs were not synced:\n%s", data)
	}
}

func TestBuiltinAutosync_SessionStartIsSilentOutsideAProject(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the emitted POSIX command is what runs here")
	}
	dir, _ := autosyncProject(t)
	command := claudeAutosyncCommand(t, dir)
	outside := t.TempDir()

	stdout, stderr := runSessionStart(t, outside, command)
	if stdout != "" || stderr != "" {
		t.Errorf("outside a project: stdout %q stderr %q, want nothing", stdout, stderr)
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("wrote %v outside a project", entries)
	}
}

func TestBuiltinAutosync_SessionStartIsSilentWithoutTheBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the emitted POSIX command is what runs here")
	}
	dir, _ := autosyncProject(t)
	command := claudeAutosyncCommand(t, dir)
	// A PATH with the shell tools the command uses and no agnostic-ai.
	tools := t.TempDir()
	for _, name := range []string{"sh", "dirname"} {
		path, err := exec.LookPath(name)
		if err != nil {
			t.Fatal(err)
		}
		must(t, os.Symlink(path, filepath.Join(tools, name)))
	}
	cmd := exec.Command(filepath.Join(tools, "sh"), "-c", command)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=" + tools, "HOME=" + t.TempDir()}
	out, err := cmd.CombinedOutput()
	if err != nil || len(out) != 0 {
		t.Errorf("without the binary: %v %q, want exit 0 and no output", err, out)
	}
}

func TestBuiltinAutosync_UserScopeSkipsIt(t *testing.T) {
	binary := builtBinary(t)
	home := t.TempDir()
	source := filepath.Join(home, ".agnostic-ai")
	must(t, os.MkdirAll(source, 0o755))
	must(t, os.WriteFile(filepath.Join(source, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude]\nbuiltins: [autosync]\n"), 0o644))
	cmd := exec.Command(binary, "sync", "--scope", "user")
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home, "AGNOSTIC_AI_HOME="+source, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sync --scope user: %v\n%s", err, out)
	}
	if data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json")); err == nil && strings.Contains(string(data), "hook autosync") {
		t.Errorf("user settings run autosync:\n%s", data)
	}
}
