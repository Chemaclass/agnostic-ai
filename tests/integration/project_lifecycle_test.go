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

func TestProjectLifecycle_BootstrapsOnceAndUsesLocalHooks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX package-manager and old-binary fixtures")
	}
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "agnostic-ai")
	build := exec.Command("go", "build", "-ldflags", "-X main.version=0.82.0 -X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion=0.82.0", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	dir := filepath.Join(t.TempDir(), "project with spaces")
	if err := os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "memory"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(path, text string, mode os.FileMode) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, path), []byte(text), mode); err != nil {
			t.Fatal(err)
		}
	}
	write("agnostic-ai.yaml", "version: 1\nrequires: '0.82.0'\ntargets: [codex, cursor]\nbuiltins: [memory]\n", 0o644)
	write("package.json", `{"devDependencies":{"agnostic-ai":"0.82.0"},"packageManager":"pnpm@10.0.0","scripts":{"postinstall":"agnostic-ai project --bootstrap"}}`, 0o644)
	write("pnpm-lock.yaml", "lockfileVersion: '9.0'\n", 0o644)
	write(".agnostic-ai/AGNOSTIC_AI.md", "Project guidance.\n", 0o644)
	write(".agnostic-ai/memory/MEMORY.md", "Local-memory-marker.\n", 0o644)
	bin := t.TempDir()
	old := filepath.Join(bin, "agnostic-ai")
	if err := os.WriteFile(old, []byte("#!/bin/sh\necho 'older global must not run' >&2\nexit 91\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	installer := "#!/bin/sh\nprintf '%s marker=%s\\n' \"$*\" \"$AGNOSTIC_AI_PROJECT_BOOTSTRAP\" >> installs.log\nmkdir -p node_modules/.bin\ncp \"$PROJECT_TEST_BINARY\" node_modules/.bin/agnostic-ai\n# Simulate the normal postinstall script, with bootstrap requested again.\nnode_modules/.bin/agnostic-ai project --bootstrap\nprintf 'normal scripts ran\\n' >> scripts.log\n"
	if err := os.WriteFile(filepath.Join(bin, "pnpm"), []byte(installer), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PROJECT_TEST_BINARY", binary)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	run := func(args ...string) (string, error) {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	if out, err := run("project", "--check"); err == nil || !strings.Contains(out, "missing") {
		t.Errorf("fresh check: %v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "installs.log")); !os.IsNotExist(err) {
		t.Error("check installed packages")
	}
	for i := 0; i < 2; i++ {
		if out, err := run("project", "--bootstrap"); err != nil {
			t.Fatalf("bootstrap: %v %s", err, out)
		}
	}
	installs, err := os.ReadFile(filepath.Join(dir, "installs.log"))
	if err != nil {
		t.Fatal(err)
	}
	if string(installs) != "install --frozen-lockfile marker=1\n" {
		t.Errorf("install calls: %s", installs)
	}
	if _, err := os.Stat(filepath.Join(dir, "scripts.log")); err != nil {
		t.Errorf("normal scripts did not run: %v", err)
	}
	if out, err := run("project", "--check"); err != nil {
		t.Errorf("fresh synchronized output: %v %s", err, out)
	}
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	git("init", "-q")
	git("add", ".")
	if out, err := run("install-hook"); err != nil {
		t.Fatalf("install hook: %v %s", err, out)
	}
	hook := exec.Command("sh", filepath.Join(dir, ".git", "hooks", "pre-commit"))
	hook.Dir = dir
	if out, err := hook.CombinedOutput(); err != nil {
		t.Errorf("local Git check: %v %s", err, out)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".codex", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var hooks struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &hooks); err != nil {
		t.Fatal(err)
	}
	command := hooks.Hooks["SessionStart"][0].Hooks[0].Command
	memory := exec.Command("sh", "-c", command)
	memory.Dir = filepath.Join(dir, ".agnostic-ai", "memory")
	if out, err := memory.CombinedOutput(); err != nil || !strings.Contains(string(out), "Local-memory-marker") {
		t.Errorf("local memory hook: %v %s", err, out)
	}
	cursorData, err := os.ReadFile(filepath.Join(dir, ".cursor", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cursorHooks struct {
		Hooks map[string][]struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(cursorData, &cursorHooks); err != nil {
		t.Fatal(err)
	}
	cursorCommand := ""
	for _, handler := range cursorHooks.Hooks["sessionStart"] {
		if strings.Contains(handler.Command, "project -- hook memory") {
			cursorCommand = handler.Command
		}
	}
	if cursorCommand == "" {
		t.Fatal("generated Cursor memory handler is missing")
	}
	t.Setenv("CURSOR_PROJECT_DIR", dir)
	t.Setenv("AGNOSTIC_AI_HOOK_TARGET", "")
	unrelated := t.TempDir()
	cursorMemory := exec.Command("sh", "-c", cursorCommand)
	cursorMemory.Dir = unrelated
	if out, err := cursorMemory.CombinedOutput(); err != nil || !strings.Contains(string(out), "Local-memory-marker") {
		t.Errorf("native host memory from unrelated directory: %v %s", err, out)
	}
	agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	write("AGENTS.md", string(agents)+"\nManual-edit-marker.\n", 0o644)
	if out, err := run("project"); err != nil {
		t.Fatalf("preserving sync: %v %s", err, out)
	}
	kept, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil || !strings.Contains(string(kept), "Manual-edit-marker") {
		t.Errorf("manual edit was not preserved: %v %s", err, kept)
	}
	write("agnostic-ai.yaml", "version: 1\nrequires: '0.83.0'\ntargets: [codex, cursor]\nbuiltins: [memory]\n", 0o644)
	memory = exec.Command("sh", "-c", command)
	memory.Dir = dir
	if out, err := memory.CombinedOutput(); err == nil || !strings.Contains(string(out), "requires 0.83.0") || !strings.Contains(string(out), "project --bootstrap") {
		t.Errorf("memory version contract: %v %s", err, out)
	}
	cursorMemory = exec.Command("sh", "-c", cursorCommand)
	cursorMemory.Dir = unrelated
	if out, err := cursorMemory.CombinedOutput(); err == nil || !strings.Contains(string(out), "requires 0.83.0") {
		t.Errorf("native host version contract: %v %s", err, out)
	}
	if err := os.Remove(filepath.Join(dir, "agnostic-ai.yaml")); err != nil {
		t.Fatal(err)
	}
	memory = exec.Command("sh", "-c", command)
	memory.Dir = filepath.Join(dir, ".agnostic-ai", "memory")
	if out, err := memory.CombinedOutput(); err != nil || !strings.Contains(string(out), "Local-memory-marker") {
		t.Errorf("configless nested local memory: %v %s", err, out)
	}
}
