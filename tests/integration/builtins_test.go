package integration

import (
	"bytes"
	"encoding/json"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/lsp"
)

func TestBuiltinHandoff(t *testing.T) {
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
	build := exec.Command("go", "build", "-ldflags", "-X main.version=0.80.0 -X github.com/chemaclass/agnostic-ai/internal/cli.candidateVersion=0.80.0", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	fixture, err := os.ReadFile(filepath.Join(packageDir, "fixtures", "builtin-handoff", "agnostic-ai.yaml"))
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
	project := func(t *testing.T) string {
		t.Helper()
		dir := t.TempDir()
		t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), fixture, 0o644); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	unusableCache := func(t *testing.T) {
		t.Helper()
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
		t.Setenv("LocalAppData", filepath.Join(home, "cache"))
		cache, err := os.UserCacheDir()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(cache), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cache, []byte("not a directory\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	paths := []string{".claude/skills/handoff/SKILL.md", ".agents/skills/handoff/SKILL.md"}

	t.Run("learning-proposal-contract", func(t *testing.T) {
		dir := project(t)
		run(t, dir, "sync", "--gitignore=off")
		for _, path := range paths {
			data, err := os.ReadFile(filepath.Join(dir, path))
			if err != nil {
				t.Fatal(err)
			}
			_, body, found := strings.Cut(string(data), "# handoff\n")
			if !found {
				t.Fatalf("%s: missing handoff skill body", path)
			}
			for _, want := range []string{
				".agnostic-ai/rules/learnings.md",
				".agnostic-ai/local/rules/learnings-local.md",
				"~/.agnostic-ai/rules/learnings.md",
				"`agnostic-ai sync`",
				"`agnostic-ai sync --global`",
				"sources.rules",
				"AGNOSTIC_AI_HOME",
			} {
				if !strings.Contains(body, want) {
					t.Errorf("%s: handoff skill body missing %q", path, want)
				}
			}
		}
	})

	t.Run("project-and-personal-learnings", func(t *testing.T) {
		dir := project(t)
		must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(string(fixture)+"outputs:\n  claude:\n    rules-file: CLAUDE.md\n"), 0o644))
		projectRule := filepath.Join(dir, ".agnostic-ai", "rules", "learnings.md")
		personalRule := filepath.Join(dir, ".agnostic-ai", "local", "rules", "learnings-local.md")
		must(t, os.MkdirAll(filepath.Dir(projectRule), 0o755))
		must(t, os.MkdirAll(filepath.Dir(personalRule), 0o755))
		must(t, os.WriteFile(projectRule, []byte("---\nname: learnings\ndescription: Project learnings.\nalwaysApply: true\n---\n\nKeep the shared project learning.\n"), 0o644))
		must(t, os.WriteFile(personalRule, []byte("---\nname: learnings-local\ndescription: Personal learnings for this project.\nalwaysApply: true\n---\n\nKeep the personal project learning.\n"), 0o644))
		run(t, dir, "sync", "--gitignore=off")
		assertContains(t, filepath.Join(dir, "CLAUDE.md"),
			"Keep the shared project learning.",
			"Keep the personal project learning.",
		)
		run(t, dir, "sync", "--check", "--gitignore=off")
	})

	t.Run("configured-learning-destinations", func(t *testing.T) {
		dir := project(t)
		home := t.TempDir()
		globalSource := filepath.Join(home, "portable")
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("AGNOSTIC_AI_HOME", globalSource)
		cfg := string(fixture) + "sources:\n  rules: guidance/rules\noutputs:\n  claude:\n    rules-file: CLAUDE.md\n"
		must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644))
		for _, rule := range []struct {
			path string
			name string
			body string
		}{
			{filepath.Join(dir, "guidance", "rules", "learnings.md"), "learnings", "Keep the configured project learning."},
			{filepath.Join(dir, ".agnostic-ai", "local", "rules", "learnings-local.md"), "learnings-local", "Keep the fixed personal learning."},
			{filepath.Join(globalSource, "rules", "learnings.md"), "learnings", "Keep the configured global learning."},
		} {
			must(t, os.MkdirAll(filepath.Dir(rule.path), 0o755))
			must(t, os.WriteFile(rule.path, []byte("---\nname: "+rule.name+"\nalwaysApply: true\n---\n\n"+rule.body+"\n"), 0o644))
		}
		must(t, os.WriteFile(filepath.Join(globalSource, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude]\n"), 0o644))
		run(t, dir, "sync", "--gitignore=off")
		assertContains(t, filepath.Join(dir, "CLAUDE.md"),
			"Keep the configured project learning.",
			"Keep the fixed personal learning.",
		)
		run(t, dir, "sync", "--check", "--gitignore=off")
		run(t, dir, "sync", "--global")
		assertContains(t, filepath.Join(home, ".claude", "CLAUDE.md"), "Keep the configured global learning.")
		run(t, dir, "sync", "--global", "--check")
	})

	t.Run("project-dogfood-requires-builtins-release", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(repoRoot, "agnostic-ai.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		var cfg config.Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(cfg.Builtins, "handoff") {
			t.Error("project dogfood does not enable handoff")
		}
		minimum, err := config.ParseRequirement(">=0.80.0")
		if err != nil {
			t.Fatal(err)
		}
		if allowed, _ := minimum.Allows(cfg.Requires); !allowed {
			t.Errorf("project requires = %q, want an exact pin at least 0.80.0", cfg.Requires)
		}
	})

	t.Run("golden-and-orphan-sweep", func(t *testing.T) {
		dir := project(t)
		run(t, dir, "sync", "--gitignore=off")
		output := map[string]string{}
		for _, path := range paths {
			data, err := os.ReadFile(filepath.Join(dir, path))
			if err != nil {
				t.Fatal(err)
			}
			output[path] = string(data)
		}
		expectedDir := filepath.Join(packageDir, "fixtures", "golden", "builtin-handoff")
		if os.Getenv("UPDATE_GOLDEN") == "1" {
			updateGolden(t, expectedDir, output)
		} else {
			compareGolden(t, expectedDir, output, "builtin-handoff")
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude, codex]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "sync", "--gitignore=off")
		for _, path := range paths {
			if _, err := os.Stat(filepath.Join(dir, path)); !os.IsNotExist(err) {
				t.Errorf("orphan %s remains: %v", path, err)
			}
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
	})

	t.Run("project-override", func(t *testing.T) {
		dir := project(t)
		path := filepath.Join(dir, ".agnostic-ai", "skills", "handoff.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("---\nname: handoff\ndescription: My handoff\n---\nProject handoff.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "sync", "--gitignore=off")
		for _, path := range paths {
			data, err := os.ReadFile(filepath.Join(dir, path))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), "Project handoff.") || strings.Contains(string(data), "## Next steps") {
				t.Errorf("override did not win: %s", data)
			}
		}
		if out := run(t, dir, "list"); !strings.Contains(out, "skill\thandoff\tproject") {
			t.Errorf("list did not report project: %s", out)
		}
	})

	for _, target := range []string{"claude", "codex", "gemini", "opencode"} {
		t.Run("native-import-keeps-builtin-"+target, func(t *testing.T) {
			dir := project(t)
			cfg := filepath.Join(dir, "agnostic-ai.yaml")
			if err := os.WriteFile(cfg, []byte("version: 1\ntargets: ["+target+"]\nbuiltins: [handoff]\noutputs:\n  "+target+":\n    emit-skills-as-commands: true\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			run(t, dir, "sync", "--gitignore=off")
			run(t, dir, "import", target)
			if out := run(t, dir, "list"); !strings.Contains(out, "skill\thandoff\tbuiltin") {
				t.Errorf("native import replaced builtin provenance: %s", out)
			}
			for _, path := range []string{"handoff.md", "handoff/SKILL.md"} {
				if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "skills", path)); !os.IsNotExist(err) {
					t.Errorf("native import copied builtin %s: %v", path, err)
				}
			}
			if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "commands", "skill-handoff.md")); !os.IsNotExist(err) {
				t.Errorf("native import copied builtin command mirror: %v", err)
			}
			if err := os.WriteFile(cfg, []byte("version: 1\ntargets: ["+target+"]\nbuiltins: []\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			run(t, dir, "sync", "--gitignore=off")
			nativeDir := map[string]string{"claude": ".claude/skills", "codex": ".agents/skills", "gemini": ".gemini/skills", "opencode": ".opencode/skills"}[target]
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(nativeDir), "handoff", "SKILL.md")); !os.IsNotExist(err) {
				t.Errorf("imported builtin survived opt-out: %v", err)
			}
			run(t, dir, "sync", "--check", "--gitignore=off")
		})
	}

	t.Run("aider-survives-temporary-cache-cleanup", func(t *testing.T) {
		dir := project(t)
		unusableCache(t)
		cfg := "version: 1\ntargets: [aider]\nbuiltins: [handoff]\noutputs:\n  aider:\n    rules-file: CONVENTIONS.md\n    conf-file: .aider.conf.yml\n"
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "sync", "--gitignore=off")
		output := map[string]string{}
		for _, name := range []string{"CONVENTIONS.md", ".aider.conf.yml"} {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			output[name] = string(data)
		}
		body := output["CONVENTIONS.md"]
		if !strings.Contains(body, "source: builtin:handoff") || !strings.Contains(body, ".agnostic-ai/local/HANDOFF.md") || strings.Contains(body, "agnostic-ai-builtin-") {
			t.Errorf("Aider handoff depends on temporary source: %s", body)
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
		run(t, dir, "sync", "--gitignore=off")
		run(t, dir, "sync", "--check", "--gitignore=off")
		data, err := os.ReadFile(filepath.Join(dir, "CONVENTIONS.md"))
		if err != nil || string(data) != body {
			t.Errorf("Aider changed after cache cleanup: %v", err)
		}
		expectedDir := filepath.Join(packageDir, "fixtures", "golden", "builtin-handoff-aider")
		if os.Getenv("UPDATE_GOLDEN") == "1" {
			updateGolden(t, expectedDir, output)
		} else {
			compareGolden(t, expectedDir, output, "builtin-handoff-aider")
		}
	})

	t.Run("invalid-name", func(t *testing.T) {
		dir := project(t)
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\nbuiltins: [nope]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "sync")
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatal("unknown builtin accepted")
		}
		for _, want := range []string{"AAI-004", "builtins", "nope", "valid names", "handoff"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("error missing %q: %s", want, out)
			}
		}
	})

	t.Run("aider-import-keeps-builtin-regenerable", func(t *testing.T) {
		dir := project(t)
		unusableCache(t)
		cfg := "version: 1\ntargets: [aider]\nbuiltins: [handoff]\noutputs:\n  aider:\n    rules-file: CONVENTIONS.md\n"
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		rule := filepath.Join(dir, ".agnostic-ai", "rules", "style.md")
		if err := os.MkdirAll(filepath.Dir(rule), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(rule, []byte("---\nname: style\n---\nKeep existing user instructions.\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "sync", "--gitignore=off")
		run(t, dir, "import", "aider")
		if _, err := os.Stat(filepath.Join(filepath.Dir(rule), "handoff.md")); !os.IsNotExist(err) {
			t.Errorf("import copied builtin into a project rule: %v", err)
		}
		shared, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(shared, []byte(".agnostic-ai/local/HANDOFF.md")) || bytes.Contains(shared, []byte("builtin:handoff")) {
			t.Errorf("import copied builtin into shared instructions: %s", shared)
		}
		cfg = strings.Replace(cfg, "builtins: [handoff]", "builtins: []", 1)
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "sync", "--gitignore=off")
		data, err := os.ReadFile(filepath.Join(dir, "CONVENTIONS.md"))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte(".agnostic-ai/local/HANDOFF.md")) || !bytes.Contains(data, []byte("Keep existing user instructions.")) {
			t.Errorf("opt-out kept builtin or lost user instructions: %s", data)
		}
		run(t, dir, "sync", "--check", "--gitignore=off")
	})

	t.Run("aider-import-skips-builtin-only-document", func(t *testing.T) {
		dir := project(t)
		cfg := "version: 1\ntargets: [aider]\nbuiltins: [handoff]\noutputs:\n  aider:\n    rules-file: CONVENTIONS.md\n"
		if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		run(t, dir, "sync", "--gitignore=off")
		run(t, dir, "import", "aider")
		rules, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai", "rules"))
		if err != nil {
			t.Fatal(err)
		}
		if len(rules) != 0 {
			t.Errorf("generated builtin document became project rules: %v", rules)
		}
	})

	t.Run("provenance-and-inputs", func(t *testing.T) {
		dir := project(t)
		run(t, dir, "sync", "--gitignore=off")
		for _, args := range [][]string{{"list"}, {"explain", "builtin:handoff"}, {"why", paths[0]}, {"status"}} {
			out := run(t, dir, args...)
			if !strings.Contains(out, "builtin:handoff (agnostic-ai v0.80.0)") {
				t.Errorf("%v missing provenance: %s", args, out)
			}
			out = run(t, dir, append(args, "--json")...)
			if !json.Valid([]byte(out)) || !strings.Contains(out, `"builtin"`) || !strings.Contains(out, `"version": "0.80.0"`) || !strings.Contains(out, `"path": ""`) || !strings.Contains(out, `"layer": "builtin"`) {
				t.Errorf("%v missing JSON provenance: %s", args, out)
			}
			if strings.Contains(out, "agnostic-ai/builtins/") || strings.Contains(out, "agnostic-ai-builtin-") {
				t.Errorf("%v leaked cache path: %s", args, out)
			}
		}
		if out := run(t, dir, "explain", "--inputs"); !regexp.MustCompile(`(?m)^builtin:handoff@[a-f0-9]{64}$`).MatchString(out) {
			t.Errorf("inputs missing builtin hash: %s", out)
		}
		if out := run(t, dir, "migrate", "--dry-run"); !strings.Contains(out, "no migrations apply") {
			t.Errorf("migrate reported builtin: %s", out)
		}
		out := run(t, dir, "lint", "--json")
		var lint struct {
			Findings []json.RawMessage `json:"findings"`
		}
		if err := json.Unmarshal([]byte(out), &lint); err != nil || len(lint.Findings) != 0 {
			t.Errorf("builtin lint findings: %v\n%s", err, out)
		}
	})

	t.Run("global-handoff", func(t *testing.T) {
		home := t.TempDir()
		source := filepath.Join(home, ".agnostic-ai")
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("AGNOSTIC_AI_HOME", source)
		t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))
		if err := os.MkdirAll(source, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "agnostic-ai.yaml"), []byte("version: 1\ntargets: [claude]\nbuiltins: [handoff]\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		dir := t.TempDir()
		run(t, dir, "sync", "--global")
		if _, err := os.Stat(filepath.Join(home, paths[0])); err != nil {
			t.Errorf("global skill missing: %v", err)
		}
		for _, args := range [][]string{{"list", "--global"}, {"explain", "builtin:handoff", "--global"}} {
			if out := run(t, dir, args...); !strings.Contains(out, "builtin:handoff (agnostic-ai v0.80.0)") {
				t.Errorf("%v missing global provenance: %s", args, out)
			}
			if out := run(t, dir, append(args, "--json")...); !json.Valid([]byte(out)) || !strings.Contains(out, `"builtin"`) || !strings.Contains(out, `"path": ""`) {
				t.Errorf("%v missing global JSON provenance: %s", args, out)
			}
		}
	})

	t.Run("init-default", func(t *testing.T) {
		dir := t.TempDir()
		run(t, dir, "init", "--all", "--gitignore=off")
		cfg, err := os.ReadFile(filepath.Join(dir, "agnostic-ai.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(cfg), "builtins: [handoff]") {
			t.Errorf("init omitted builtins: %s", cfg)
		}
	})

	t.Run("lsp-exit-cleans-temporary-builtin", func(t *testing.T) {
		dir := project(t)
		unusableCache(t)
		temp := t.TempDir()
		t.Setenv("TMPDIR", temp)
		t.Setenv("TEMP", temp)
		t.Setenv("TMP", temp)
		uri := (&url.URL{Scheme: "file", Path: "/" + strings.TrimPrefix(filepath.ToSlash(dir), "/")}).String()
		var in bytes.Buffer
		writer := lsp.NewWriter(&in)
		for _, msg := range []map[string]any{
			{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"rootUri": uri}},
			{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{"textDocument": map[string]any{"uri": uri + "/agnostic-ai.yaml"}}},
			{"jsonrpc": "2.0", "id": 2, "method": "shutdown"},
			{"jsonrpc": "2.0", "method": "exit"},
		} {
			if err := writer.Send(msg); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command(binary, "lsp")
		cmd.Dir = dir
		cmd.Stdin = &in
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("lsp: %v\n%s", err, out)
		}
		if !bytes.Contains(out, []byte("publishDiagnostics")) {
			t.Errorf("LSP did not load project: %s", out)
		}
		entries, err := os.ReadDir(temp)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "agnostic-ai-builtin-") {
				t.Errorf("LSP exit left temporary builtin: %s", entry.Name())
			}
		}
	})
}
