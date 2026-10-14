package integration

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
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

	t.Run("antigravity-reports-expanded-memory-index-limit", func(t *testing.T) {
		dir := project(t, []byte("version: 1\ntargets: [antigravity]\nbuiltins: [memory]\n"))
		index := filepath.Join(dir, ".agnostic-ai", "memory", "MEMORY.md")
		if err := os.MkdirAll(filepath.Dir(index), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(index, []byte(strings.Repeat("p", 24000)), 0o644); err != nil {
			t.Fatal(err)
		}
		out := run(t, dir, "sync")
		if !strings.Contains(out, "Antigravity truncates a rules file over 24,000 bytes; split the rule") {
			t.Errorf("sync did not report the expanded memory rule size: %s", out)
		}
		body, err := os.ReadFile(filepath.Join(dir, ".agents", "rules", "antigravity-memory-index.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(body), "@[Project memory](") || len(body) >= 24000 {
			t.Errorf("sync must preserve the native include rather than inline the index: %s", body)
		}
	})

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

	hookFixture, err := os.ReadFile(filepath.Join(packageDir, "fixtures", "builtin-memory-hook", "agnostic-ai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	// subagentContext runs the Claude Code subagent hook and reports
	// whether it adds context.
	subagentContext := func(t *testing.T, dir string) bool {
		t.Helper()
		out := run(t, dir, "hook", "run", "memory-subagent-start", "--target", "claude", "--format", "json")
		if !strings.Contains(out, `"decision": "allow"`) {
			t.Errorf("the subagent hook must allow:\n%s", out)
		}
		return strings.Contains(out, `"adds_context": true`)
	}
	personalIndex := func(t *testing.T, dir string) {
		t.Helper()
		store := filepath.Join(dir, ".agnostic-ai", "local", "memory")
		if err := os.MkdirAll(store, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(store, "MEMORY.md"), []byte("- [Prefers tabs](tabs.md): the user indents with tabs\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("devin-native-memory-hook-adds-context", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			return
		}
		dir := project(t, []byte("version: 1\ntargets: [windsurf]\nbuiltins: [memory]\n"))
		t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
		run(t, dir, "sync", "--gitignore=off")
		store := filepath.Join(dir, ".agnostic-ai", "memory")
		if err := os.MkdirAll(store, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(store, "MEMORY.md"), []byte("- [Native recall](recall.md): the project uses native startup context\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		payload := filepath.Join(t.TempDir(), "session-start.json")
		if err := os.WriteFile(payload, []byte(`{"hook_event_name":"SessionStart","session_id":"native-memory-fixture"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		out := run(t, dir, "hook", "run", "memory-windsurf-session-start", "--target", "windsurf", "--payload", payload, "--format", "json")
		if !strings.Contains(out, `"adds_context": true`) || !strings.Contains(out, "native startup context") {
			t.Errorf("native memory hook did not add the project index: %s", out)
		}
	})

	t.Run("trae-native-memory-hook-adds-the-index-to-context", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Trae hook diagnostics model its Unix launcher")
		}
		dir := project(t, []byte("version: 1\ntargets: [trae]\nbuiltins: [memory]\n"))
		t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
		personalIndex(t, dir)
		run(t, dir, "sync", "--gitignore=off")
		data, err := os.ReadFile(filepath.Join(dir, ".trae", "hooks.json"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "SessionStart") || !strings.Contains(string(data), "hook memory --target trae") {
			t.Errorf("native Trae memory startup command must be emitted:\n%s", data)
		}
		out := run(t, dir, "hook", "run", "memory-trae-session-start", "--target", "trae", "--format", "json")
		if !strings.Contains(out, `"adds_context": true`) || !strings.Contains(out, "Prefers tabs") {
			t.Errorf("the unique native Trae hook must load the actual index:\n%s", out)
		}
	})

	clineSDKPayload := func(t *testing.T, dir string) string {
		t.Helper()
		path := filepath.Join(dir, "cline-sdk-task-start.json")
		if err := os.WriteFile(path, []byte(`{"hookName":"agent_start","taskId":"fixture-task","sessionContext":{"rootSessionId":"fixture-session"}}`), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}

	t.Run("cline-vscode-builtin-emits-and-adds-context", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("VS Code Windows launcher resolution is not modeled")
		}
		dir := project(t, []byte("version: 1\ntargets: [cline]\nbuiltins: [memory]\n"))
		t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
		personalIndex(t, dir)
		run(t, dir, "sync", "--gitignore=off")
		native := filepath.Join(dir, ".clinerules", "hooks", "TaskStart")
		body, err := os.ReadFile(native)
		if err != nil {
			t.Fatalf("memory builtin must emit native Cline TaskStart: %v", err)
		}
		if !strings.Contains(string(body), "hook memory --target cline") {
			t.Errorf("native TaskStart omits the builtin memory command:\n%s", body)
		}
		out := run(t, dir, "hook", "run", "memory-cline-task-start", "--target", "cline", "--runtime", "vscode", "--format", "json")
		if !strings.Contains(out, `"adds_context": true`) || !strings.Contains(out, "Prefers tabs") {
			t.Errorf("public builtin sync must load the actual index in the VS Code diagnostic:\n%s", out)
		}
		if out := run(t, dir, "hook", "run", "memory-cline-task-start", "--target", "cline", "--payload", clineSDKPayload(t, dir), "--format", "json"); strings.Contains(out, `"adds_context": true`) {
			t.Errorf("default SDK TaskStart must ignore its reply:\n%s", out)
		}
	})

	t.Run("augment-cli-runs-the-executable-memory-script-from-a-nested-folder", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Augment dispatches .sh scripts directly on Unix")
		}
		dir := project(t, []byte("version: 1\ntargets: [augment]\nbuiltins: [memory]\n"))
		t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
		run(t, dir, "sync", "--gitignore=off")
		var doc struct {
			Hooks map[string][]struct {
				Hooks []struct {
					Command string `json:"command"`
				} `json:"hooks"`
			} `json:"hooks"`
		}
		data, err := os.ReadFile(filepath.Join(dir, ".augment", "settings.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		groups := doc.Hooks["SessionStart"]
		if len(groups) != 1 || len(groups[0].Hooks) != 1 {
			t.Fatalf("Augment SessionStart does not have one script: %+v", groups)
		}
		command := groups[0].Hooks[0].Command
		if filepath.ToSlash(command) != ".augment/hooks/memory-augment-session-start.sh" {
			t.Fatalf("Augment command is not the emitted script path: %q", command)
		}
		handoffWrite(t, filepath.Join(dir, ".agnostic-ai", "memory", "MEMORY.md"), "- [Probe](probe.md): AUGMENT_MEMORY_PROBE_2106\n")
		personalIndex(t, dir)
		nested := filepath.Join(dir, "nested", "session directory")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(filepath.Join(dir, filepath.FromSlash(command)))
		cmd.Dir = nested
		cmd.Stdin = strings.NewReader(`{"hook_event_name":"SessionStart","workspace_roots":[]}`)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("direct Augment script: %v\n%s", err, out)
		}
		var reply struct {
			HookSpecificOutput struct {
				HookEventName     string `json:"hookEventName"`
				AdditionalContext string `json:"additionalContext"`
			} `json:"hookSpecificOutput"`
		}
		if err := json.Unmarshal(out, &reply); err != nil {
			t.Fatalf("Augment context is not JSON: %v\n%s", err, out)
		}
		if reply.HookSpecificOutput.HookEventName != "SessionStart" || !strings.Contains(reply.HookSpecificOutput.AdditionalContext, "AUGMENT_MEMORY_PROBE_2106") || !strings.Contains(reply.HookSpecificOutput.AdditionalContext, "Prefers tabs") {
			t.Errorf("Augment script lost project or personal memory: %+v", reply)
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
	})

	t.Run("antigravity-local-override-keeps-memory-routing", func(t *testing.T) {
		authoredBody := "Authored replacement.\n\n@[Project memory](../../.agnostic-ai/memory/MEMORY.md)\n\nPersonal memory, `.agnostic-ai/local/memory/MEMORY.md`:\n\n@[Personal memory](../../.agnostic-ai/local/memory/MEMORY.md)\n"
		for _, test := range []struct {
			name string
			body string
		}{
			{"metadata-only-inherits-native-memory", ""},
			{"authored-body-is-preserved", authoredBody},
		} {
			t.Run(test.name, func(t *testing.T) {
				dir := project(t, []byte("version: 1\ntargets: [antigravity]\nbuiltins: [memory]\noutputs:\n  antigravity:\n    rules-dir: custom/deep/rules\n"))
				t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
				t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "absent-gitconfig"))
				if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
					t.Fatalf("git init: %v\n%s", err, out)
				}
				if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.local.yaml"), []byte("memory:\n  personal: repo\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				override := filepath.Join(dir, ".agnostic-ai", "local", "rules", "antigravity-memory-index.md")
				if err := os.MkdirAll(filepath.Dir(override), 0o755); err != nil {
					t.Fatal(err)
				}
				source := "---\nname: antigravity-memory-index\ndescription: Personal wording\n---\n\n" + test.body
				if err := os.WriteFile(override, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
				run(t, dir, "sync", "--gitignore=off")
				path := filepath.Join(dir, "custom", "deep", "rules", "antigravity-memory-index.md")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				text := string(data)
				if test.body != "" {
					if !strings.Contains(text, test.body) {
						t.Errorf("authored replacement body must stay unchanged:\n%s", text)
					}
					return
				}
				if !strings.Contains(text, "](../../../.agnostic-ai/memory/MEMORY.md)") {
					t.Errorf("inherited project include must resolve from the configured output directory:\n%s", text)
				}
				if strings.Contains(text, ".agnostic-ai/local/memory/MEMORY.md") {
					t.Errorf("repo-personal mode must omit the inherited checkout include:\n%s", text)
				}
			})
		}
	})

	t.Run("antigravity-scoped-local-override", func(t *testing.T) {
		authoredBody := "Authored replacement.\n\n@[Project memory](../../.agnostic-ai/memory/MEMORY.md)\n"
		for _, test := range []struct {
			name string
			body string
		}{
			{"metadata-only-inherits-native-memory", ""},
			{"authored-body-is-preserved", authoredBody},
		} {
			t.Run(test.name, func(t *testing.T) {
				dir := project(t, []byte("version: 1\ntargets: [antigravity]\nbuiltins: [memory]\n"))
				if err := os.MkdirAll(filepath.Join(dir, "backend"), 0o755); err != nil {
					t.Fatal(err)
				}
				override := filepath.Join(dir, ".agnostic-ai", "local", "rules", "antigravity-memory-index.md")
				if err := os.MkdirAll(filepath.Dir(override), 0o755); err != nil {
					t.Fatal(err)
				}
				source := "---\nname: antigravity-memory-index\nscope: backend\n---\n\n" + test.body
				if err := os.WriteFile(override, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
				run(t, dir, "sync", "--gitignore=off")
				path := filepath.Join(dir, "backend", ".agents", "rules", "antigravity-memory-index.md")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				text := string(data)
				if test.body != "" {
					if !strings.Contains(text, test.body) {
						t.Errorf("authored scoped replacement body must stay unchanged:\n%s", text)
					}
					return
				}
				index := filepath.Join(dir, ".agnostic-ai", "memory", "MEMORY.md")
				ref, err := filepath.Rel(filepath.Dir(path), index)
				if err != nil {
					t.Fatal(err)
				}
				include := "@[Project memory](" + filepath.ToSlash(ref) + ")"
				if !strings.Contains(text, include) {
					t.Errorf("inherited include must resolve from the emitted scoped directory, want %q:\n%s", include, text)
				}
			})
		}
	})

	t.Run("session-start-hook-adds-the-index-to-context", func(t *testing.T) {
		dir := project(t, hookFixture)
		t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
		run(t, dir, "sync", "--gitignore=off")
		paths := []string{".claude/settings.json", ".codex/hooks.json", ".github/hooks/agnostic-ai.json", ".cursor/hooks.json", ".gemini/settings.json", ".qoder/settings.json", ".factory/hooks.json", ".augment/settings.json", ".augment/hooks/memory-augment-session-start.sh", ".clinerules/hooks/TaskStart", ".devin/hooks.v1.json", ".trae/hooks.json"}
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
		targets := []string{"codex", "copilot", "cursor", "gemini", "qoder", "factory", "augment", "windsurf", "trae"}
		devinPayload := filepath.Join(t.TempDir(), "session-start.json")
		if err := os.WriteFile(devinPayload, []byte(`{"hook_event_name":"SessionStart","session_id":"memory-fixture"}`), 0o600); err != nil {
			t.Fatal(err)
		}
		memoryHookResult := func(target string) string {
			name := "memory-session-start"
			switch target {
			case "windsurf":
				name = "memory-windsurf-session-start"
			case "trae":
				name = "memory-trae-session-start"
			}
			args := []string{"hook", "run", name, "--target", target, "--format", "json"}
			if target == "windsurf" {
				args = append(args, "--payload", devinPayload)
			}
			return run(t, dir, args...)
		}
		for _, target := range targets {
			if out := memoryHookResult(target); !strings.Contains(out, `"adds_context": false`) {
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
			if out := memoryHookResult(target); !strings.Contains(out, `"adds_context": true`) {
				t.Errorf("%s: the index does not reach the model:\n%s", target, out)
			}
		}
		if out := run(t, dir, "hook", "run", "memory-cline-task-start", "--target", "cline", "--runtime", "vscode", "--format", "json"); !strings.Contains(out, `"adds_context": true`) {
			t.Errorf("Cline VS Code diagnostic did not read the index:\n%s", out)
		}
		if out := run(t, dir, "hook", "run", "memory-cline-task-start", "--target", "cline", "--payload", clineSDKPayload(t, dir), "--format", "json"); strings.Contains(out, `"adds_context": true`) {
			t.Errorf("default Cline SDK diagnostic must remain detached:\n%s", out)
		}
		// CLAUDE.md gives Claude Code subagents the project index, and
		// Claude Code's auto memory loads the personal one, which never
		// reaches a subagent.
		if claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); strings.Contains(string(claude), "local/memory") {
			t.Fatalf("CLAUDE.md imports the personal index:\n%s", claude)
		}
		if subagentContext(t, dir) {
			t.Error("the subagent hook repeats the project index CLAUDE.md imports")
		}
		personalIndex(t, dir)
		if !subagentContext(t, dir) {
			t.Error("the personal index does not reach Claude Code subagents")
		}
		t.Setenv("PATH", filepath.Dir(sh))
		for _, target := range targets {
			if target == "trae" {
				continue
			}
			out := memoryHookResult(target)
			if !strings.Contains(out, `"adds_context": false`) || !strings.Contains(out, `"decision": "allow"`) {
				t.Errorf("%s: without agnostic-ai on PATH the hook must allow quietly:\n%s", target, out)
			}
		}
		traeRun := exec.Command(binary, "hook", "run", "memory-trae-session-start", "--target", "trae", "--format", "json")
		traeRun.Dir = dir
		traeOutput, traeErr := traeRun.CombinedOutput()
		if traeErr == nil || !strings.Contains(string(traeOutput), `"decision": "error"`) || !strings.Contains(string(traeOutput), `"adds_context": false`) {
			t.Errorf("Trae without agnostic-ai on PATH: %v\n%s", traeErr, traeOutput)
		}
		if subagentContext(t, dir) {
			t.Error("without agnostic-ai on PATH the subagent hook must allow quietly")
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
	})

	t.Run("claude-keeps-the-personal-import-when-auto-memory-is-off", func(t *testing.T) {
		t.Setenv("CLAUDE_CODE_DISABLE_AUTO_MEMORY", "1")
		dir := project(t, hookFixture)
		t.Setenv("PATH", filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
		run(t, dir, "sync", "--gitignore=off")
		if claude, _ := os.ReadFile(filepath.Join(dir, "CLAUDE.md")); !strings.Contains(string(claude), "\n@.agnostic-ai/local/memory/MEMORY.md\n") {
			t.Fatalf("CLAUDE.md lost the personal import:\n%s", claude)
		}
		personalIndex(t, dir)
		if runtime.GOOS != "windows" && subagentContext(t, dir) {
			t.Error("the subagent hook repeats the personal index CLAUDE.md imports")
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

	for _, c := range []struct {
		name, targets string
		localConfig   []byte
		// The steering copy of the shared policy stays unless the
		// shared AGENTS.md carries the same rule.
		policyInSteering bool
	}{
		{"kiro", "kiro", nil, true},
		{"kiro-in-repo-mode", "kiro", repoMode, true},
		{"kiro-with-codex", "kiro, codex", nil, false},
	} {
		t.Run(c.name+"-includes-the-indexes-in-a-steering-file", func(t *testing.T) {
			dir := project(t, []byte("version: 1\ntargets: ["+c.targets+"]\nbuiltins: [memory]\n"))
			if c.localConfig != nil {
				if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
					t.Fatalf("git init: %v\n%s", err, out)
				}
				if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.local.yaml"), c.localConfig, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			run(t, dir, "sync", "--gitignore=off")
			index, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(kiroMemoryIndexRule)))
			if err != nil {
				t.Fatal(err)
			}
			expectedName := "builtin-memory-kiro"
			if c.localConfig != nil {
				expectedName += "-repo"
			}
			expectedDir := filepath.Join(packageDir, "fixtures", "golden", expectedName)
			output := map[string]string{kiroMemoryIndexRule: string(index)}
			if c.localConfig != nil {
				if !strings.Contains(string(index), "#[[file:.agnostic-ai/memory/MEMORY.md]]") {
					t.Errorf("repo mode lacks project include:\n%s", index)
				}
				if strings.Contains(string(index), ".agnostic-ai/local/memory/MEMORY.md") {
					t.Errorf("repo mode names the checkout personal store:\n%s", index)
				}
			}
			if os.Getenv("UPDATE_GOLDEN") == "1" && (c.name == "kiro" || c.localConfig != nil) {
				updateGolden(t, expectedDir, output)
			} else {
				compareGolden(t, expectedDir, output, expectedName)
			}
			if got := filesNaming(t, dir, "#[[file:"); !slices.Equal(got, []string{kiroMemoryIndexRule}) {
				t.Errorf("files with a Kiro include = %v, want only %s", got, kiroMemoryIndexRule)
			}
			_, err = os.Stat(filepath.Join(dir, ".kiro", "steering", "shared-memory-policy.md"))
			if (err == nil) != c.policyInSteering {
				t.Errorf("policy steering file exists = %t, want %t", err == nil, c.policyInSteering)
			}
			if !c.policyInSteering {
				agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
				if err != nil || !strings.Contains(string(agents), "### shared-memory-policy") {
					t.Errorf("AGENTS.md should carry the shared policy instead: %v\n%s", err, agents)
				}
			}
			run(t, dir, "sync", "--check", "--gitignore=off")
		})
	}

	t.Run("kiro-repo-mode-leaves-a-retained-checkout-index-out", func(t *testing.T) {
		dir := project(t, []byte("version: 1\ntargets: [kiro]\nbuiltins: [memory]\n"))
		if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, out)
		}
		stale := filepath.Join(dir, ".agnostic-ai", "local", "memory", "MEMORY.md")
		if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(stale, []byte("- [Checkout fact](checkout.md): retained checkout fact\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "sync", "--gitignore=off")
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.local.yaml"), repoMode, 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "sync", "--gitignore=off")
		rule, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(kiroMemoryIndexRule)))
		if err != nil {
			t.Fatal(err)
		}
		rendered := run(t, dir, "render", "builtin:kiro-memory-index", "--target", "kiro")
		for _, output := range []string{string(rule), rendered} {
			if !strings.Contains(output, "#[[file:.agnostic-ai/memory/MEMORY.md]]") {
				t.Errorf("repo mode lacks project include:\n%s", output)
			}
			if strings.Contains(output, ".agnostic-ai/local/memory/MEMORY.md") {
				t.Errorf("repo mode still loads retained checkout memory:\n%s", output)
			}
		}
		if _, err := os.Stat(stale); err != nil {
			t.Errorf("switch removed retained checkout index: %v", err)
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
	})

	t.Run("kiro-repo-mode-keeps-a-project-owned-memory-rule", func(t *testing.T) {
		dir := project(t, []byte("version: 1\ntargets: [kiro]\nbuiltins: [memory]\n"))
		if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, out)
		}
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.local.yaml"), repoMode, 0o644); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, ".agnostic-ai", "rules", "kiro-memory-index.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "A project-owned include.\n\nPersonal memory, `.agnostic-ai/local/memory/MEMORY.md`:\n\n#[[file:.agnostic-ai/local/memory/MEMORY.md]]\n"
		if err := os.WriteFile(path, []byte("---\nname: kiro-memory-index\nalwaysApply: true\ntargets: [kiro]\n---\n\n"+body), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "sync", "--gitignore=off")
		written, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(kiroMemoryIndexRule)))
		if err != nil {
			t.Fatal(err)
		}
		rendered := run(t, dir, "render", ".agnostic-ai/rules/kiro-memory-index.md", "--target", "kiro")
		for _, output := range []string{string(written), rendered} {
			if !strings.Contains(output, body) {
				t.Errorf("project-owned memory rule was changed:\n%s", output)
			}
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
	})

	t.Run("kiro-include-needs-no-index-and-leaves-with-the-built-in", func(t *testing.T) {
		dir := project(t, []byte("version: 1\ntargets: [kiro]\nbuiltins: [memory]\n"))
		run(t, dir, "sync", "--gitignore=off")
		// CI has no personal store and a laptop may, so --check must pass
		// with and without the indexes.
		run(t, dir, "sync", "--check", "--gitignore=off")
		for _, index := range []string{".agnostic-ai/memory/MEMORY.md", ".agnostic-ai/local/memory/MEMORY.md"} {
			path := filepath.Join(dir, filepath.FromSlash(index))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("- [CI is Ubuntu only](ci-ubuntu.md): PR CI runs on Ubuntu alone\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [kiro]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "sync", "--gitignore=off")
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(kiroMemoryIndexRule))); !os.IsNotExist(err) {
			t.Errorf("dropping the built-in should remove %s: %v", kiroMemoryIndexRule, err)
		}
	})

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
		if got := filesNaming(t, dir, "#[[file:"); !slices.Equal(got, []string{kiroMemoryIndexRule}) {
			t.Errorf("files with a Kiro include = %v, want only %s", got, kiroMemoryIndexRule)
		}
		if got := filesNaming(t, dir, "gemini-memory-tiers"); !slices.Equal(got, []string{"GEMINI.md"}) {
			t.Errorf("the Gemini CLI memory tiers rule should reach GEMINI.md alone, found it in %v", got)
		}
	})
}

const kiroMemoryIndexRule = ".kiro/steering/kiro-memory-index.md"

// filesNaming lists, sorted and relative to dir, the files sync wrote
// that contain text. The source folder and links are skipped.
func filesNaming(t *testing.T, dir, text string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".agnostic-ai" || entry.Name() == ".git") {
			return filepath.SkipDir
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), text) {
			rel, err := filepath.Rel(dir, path)
			if err != nil {
				return err
			}
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(found)
	return found
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
