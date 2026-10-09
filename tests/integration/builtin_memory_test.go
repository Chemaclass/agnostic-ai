package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
)

func TestBuiltinMemory(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Clean(filepath.Join(packageDir, "..", ".."))
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	fixture, err := os.ReadFile(filepath.Join(packageDir, "fixtures", "builtin-memory", "agnostic-ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	run := func(t *testing.T, dir string, args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	project := func(t *testing.T, config []byte) string {
		t.Helper()
		dir := t.TempDir()
		t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), config, 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}

	repoMode, err := os.ReadFile(filepath.Join(packageDir, "fixtures", "builtin-memory-repo", "agnostic-ai.local.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// agnostic-ai.local.yaml stays out of Git, so files a team commits
	// must come out the same whether or not one user turns repo mode on.
	for _, mode := range []struct {
		name        string
		localConfig []byte
	}{
		{"golden", nil},
		{"golden-in-repo-mode", repoMode},
	} {
		t.Run(mode.name, func(t *testing.T) {
			dir := project(t, fixture)
			if mode.localConfig != nil {
				if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
					t.Fatalf("git init: %v\n%s", err, out)
				}
				if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.local.yaml"), mode.localConfig, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			run(t, dir, "sync", "--gitignore=off")
			paths := []string{
				"CLAUDE.md",
				"AGENTS.md",
				"GEMINI.md",
				".claude/rules/shared-memory-policy.md",
				".claude/skills/shared-memory/SKILL.md",
				".agents/skills/shared-memory/SKILL.md",
				".cursor/rules/shared-memory-policy.mdc",
			}
			output := map[string]string{}
			for _, path := range paths {
				data, err := os.ReadFile(filepath.Join(dir, path))
				if err != nil {
					t.Fatal(err)
				}
				output[path] = string(data)
			}
			expectedDir := filepath.Join(packageDir, "fixtures", "golden", "builtin-memory")
			if os.Getenv("UPDATE_GOLDEN") == "1" && mode.localConfig == nil {
				updateGolden(t, expectedDir, output)
			} else {
				compareGolden(t, expectedDir, output, "builtin-memory")
			}
			run(t, dir, "sync", "--check", "--gitignore=off")
		})
	}

	t.Run("saving-a-fact-needs-no-sync", func(t *testing.T) {
		dir := project(t, fixture)
		run(t, dir, "sync", "--gitignore=off")
		store := filepath.Join(dir, ".agnostic-ai", "memory")
		if err := os.MkdirAll(store, 0o755); err != nil {
			t.Fatal(err)
		}
		files := map[string]string{
			"MEMORY.md":    "- [CI is Ubuntu only](ci-ubuntu.md): PR CI runs on Ubuntu alone\n",
			"ci-ubuntu.md": "---\nname: ci-ubuntu\ndescription: PR CI runs on Ubuntu alone\nmetadata:\n  type: project\n---\n\nPR CI runs on Ubuntu alone.\n",
		}
		for name, body := range files {
			if err := os.WriteFile(filepath.Join(store, name), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
	})

	t.Run("import-leaves-the-memory-block-out", func(t *testing.T) {
		dir := project(t, fixture)
		run(t, dir, "sync", "--gitignore=off")
		run(t, dir, "import", "claude")
		body, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "agnostic-ai:memory") || strings.Contains(string(body), "MEMORY.md") {
			t.Errorf("AGNOSTIC_AI.md picked up the memory block:\n%s", body)
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
	})

	t.Run("session-start-hook-adds-the-index-to-context", func(t *testing.T) {
		hookFixture, err := os.ReadFile(filepath.Join(packageDir, "fixtures", "builtin-memory-hook", "agnostic-ai.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		dir := project(t, hookFixture)
		t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
		run(t, dir, "sync", "--gitignore=off")
		paths := []string{".codex/hooks.json", ".github/hooks/agnostic-ai.json", ".cursor/hooks.json", ".gemini/settings.json", ".qoder/settings.json", ".factory/hooks.json"}
		output := map[string]string{}
		for _, path := range paths {
			data, err := os.ReadFile(filepath.Join(dir, path))
			if err != nil {
				t.Fatal(err)
			}
			output[path] = string(data)
		}
		expectedDir := filepath.Join(packageDir, "fixtures", "golden", "builtin-memory-hook")
		if os.Getenv("UPDATE_GOLDEN") == "1" {
			updateGolden(t, expectedDir, output)
		} else {
			compareGolden(t, expectedDir, output, "builtin-memory-hook")
		}

		if runtime.GOOS == "windows" {
			// hook run lists Cursor, Qoder, and Factory hooks as not run on Windows.
			return
		}
		sh, err := exec.LookPath("sh")
		if err != nil {
			t.Fatal(err)
		}
		targets := []string{"codex", "copilot", "cursor", "gemini", "qoder", "factory"}
		for _, target := range targets {
			if out := run(t, dir, "hook", "run", "memory-session-start", "--target", target, "--format", "json"); !strings.Contains(out, `"adds_context": false`) {
				t.Errorf("%s: an empty store adds context:\n%s", target, out)
			}
		}
		store := filepath.Join(dir, ".agnostic-ai", "memory")
		if err := os.MkdirAll(store, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(store, "MEMORY.md"), []byte("- [CI is Ubuntu only](ci-ubuntu.md): PR CI runs on Ubuntu alone\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, target := range targets {
			if out := run(t, dir, "hook", "run", "memory-session-start", "--target", target, "--format", "json"); !strings.Contains(out, `"adds_context": true`) {
				t.Errorf("%s: the index does not reach the model:\n%s", target, out)
			}
		}
		t.Setenv("PATH", filepath.Dir(sh))
		for _, target := range targets {
			out := run(t, dir, "hook", "run", "memory-session-start", "--target", target, "--format", "json")
			if !strings.Contains(out, `"adds_context": false`) || !strings.Contains(out, `"decision": "allow"`) {
				t.Errorf("%s: without agnostic-ai on PATH the hook must allow quietly:\n%s", target, out)
			}
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
	})

	for _, c := range []struct{ target, file, targets string }{
		{"opencode", "opencode.json", "opencode"},
		{"kilo", "kilo.jsonc", "kilo"},
		{"kilo-with-codex", "kilo.jsonc", "kilo, codex"},
	} {
		t.Run(c.target+"-lists-the-indexes-and-gives-them-back", func(t *testing.T) {
			testContextListMemory(t, project, run, c.file, c.targets)
		})
	}

	t.Run("kilo-gives-the-indexes-back-after-rules-move-into-agents-md", func(t *testing.T) {
		dir := project(t, []byte("version: 1\ntargets: [kilo]\nbuiltins: [memory]\nsync:\n  unmanaged: [AGENTS.md]\n"))
		run(t, dir, "sync", "--gitignore=off")
		for _, config := range []string{"version: 1\ntargets: [kilo]\nbuiltins: [memory]\n", "version: 1\ntargets: [kilo]\n"} {
			if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(config), 0o644); err != nil {
				t.Fatal(err)
			}
			run(t, dir, "sync", "--gitignore=off")
		}
		if data, _ := os.ReadFile(filepath.Join(dir, "kilo.jsonc")); strings.Contains(string(data), "MEMORY.md") {
			t.Errorf("memory indexes stuck in kilo.jsonc:\n%s", data)
		}
	})

	t.Run("every-target-syncs", func(t *testing.T) {
		config := "version: 1\nbuiltins: [memory]\ntargets: [" + strings.Join(adapters.Names(), ", ") + "]\n"
		dir := project(t, []byte(config))
		run(t, dir, "sync", "--gitignore=off")
		run(t, dir, "sync", "--check", "--gitignore=off")
	})
}

func testContextListMemory(t *testing.T, project func(*testing.T, []byte) string, run func(*testing.T, string, ...string) string, file, targets string) {
	dir := project(t, []byte("version: 1\ntargets: ["+targets+"]\nbuiltins: [memory]\n"))
	config := filepath.Join(dir, file)
	if err := os.WriteFile(config, []byte(`{"instructions": ["CONTRIBUTING.md"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "sync", "--gitignore=off")
	run(t, dir, "sync", "--gitignore=off")
	data, _ := os.ReadFile(config)
	for _, want := range []string{"CONTRIBUTING.md", ".agnostic-ai/local/memory/MEMORY.md", ".agnostic-ai/memory/MEMORY.md"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("%s lacks %s:\n%s", file, want, data)
		}
	}
	run(t, dir, "sync", "--check", "--gitignore=off")
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: ["+targets+"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "sync", "--gitignore=off")
	data, _ = os.ReadFile(config)
	if strings.Contains(string(data), "MEMORY.md") || !strings.Contains(string(data), "CONTRIBUTING.md") {
		t.Errorf("dropping the built-in should keep only the user's entry in %s:\n%s", file, data)
	}
}
