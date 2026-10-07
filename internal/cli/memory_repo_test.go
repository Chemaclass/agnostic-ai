package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// repoMemoryProject sets up a Git project whose agnostic-ai.local.yaml
// sets memory.personal: repo, and returns the repo store it should use.
func repoMemoryProject(t *testing.T, gitignore bool) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("AGNOSTIC_AI_HOME", home)
	testutil.TempCwd(t)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cfg := "version: 1\ntargets: [claude, codex, gemini, opencode, kilo]\nbuiltins: [memory]\n"
	if gitignore {
		cfg += "gitignore:\n  enabled: true\n"
	}
	writeFile(t, "agnostic-ai.yaml", cfg)
	writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: repo\n")
	silence(t)
	entries, _ := os.ReadDir(filepath.Join(home, "local", "memory"))
	if len(entries) != 0 {
		t.Fatalf("store exists before sync: %v", entries)
	}
	return filepath.Join(home, "local", "memory")
}

// repoStore returns the one folder sync and the hook name under parent.
func repoStore(t *testing.T, parent, text string) string {
	t.Helper()
	i := strings.Index(text, filepath.ToSlash(parent)+"/")
	if i < 0 {
		t.Fatalf("no path under %s in:\n%s", parent, text)
	}
	rest := text[i+len(filepath.ToSlash(parent))+1:]
	return filepath.ToSlash(parent) + "/" + rest[:strings.IndexAny(rest, "/\"\n")]
}

func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestSync_RepoPersonalMemoryReachesEveryTarget(t *testing.T) {
	parent := repoMemoryProject(t, true)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	claude := readText(t, "CLAUDE.md")
	store := repoStore(t, parent, claude)
	if !strings.Contains(claude, "\n@"+store+"/MEMORY.md\n") || !strings.Contains(claude, "\n@.agnostic-ai/memory/MEMORY.md\n") {
		t.Errorf("CLAUDE.md imports:\n%s", claude)
	}
	var local map[string]any
	if err := json.Unmarshal([]byte(readText(t, filepath.Join(".claude", "settings.local.json"))), &local); err != nil || local["autoMemoryDirectory"] != store {
		t.Errorf("autoMemoryDirectory = %v, want %s (%v)", local["autoMemoryDirectory"], store, err)
	}
	if codex := readText(t, filepath.Join(".codex", "config.toml")); !strings.Contains(codex, "[sandbox_workspace_write]\nwritable_roots = [\""+store+"\"]") {
		t.Errorf("codex config.toml:\n%s", codex)
	}
	var gemini struct {
		Context struct {
			IncludeDirectories []string `json:"includeDirectories"`
		} `json:"context"`
	}
	if err := json.Unmarshal([]byte(readText(t, filepath.Join(".gemini", "settings.json"))), &gemini); err != nil || len(gemini.Context.IncludeDirectories) != 1 || gemini.Context.IncludeDirectories[0] != store {
		t.Errorf("gemini includeDirectories = %v (%v)", gemini.Context.IncludeDirectories, err)
	}
	for _, file := range []string{"opencode.json", "kilo.jsonc"} {
		if text := readText(t, file); !strings.Contains(text, `"`+store+`/MEMORY.md"`) || strings.Contains(text, ".agnostic-ai/local/memory") {
			t.Errorf("%s instructions:\n%s", file, text)
		}
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check after sync: %v", err)
	}
}

// With no managed .gitignore block the outputs may be committed, so they
// keep the checkout store and never carry an absolute path.
func TestSync_RepoPersonalMemoryStaysOutOfCommittableFiles(t *testing.T) {
	parent := repoMemoryProject(t, false)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	for _, file := range []string{"CLAUDE.md", "opencode.json", "kilo.jsonc", filepath.Join(".gemini", "settings.json"), filepath.Join(".codex", "config.toml")} {
		data, err := os.ReadFile(file)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), filepath.ToSlash(parent)) {
			t.Errorf("%s names the repo store:\n%s", file, data)
		}
	}
	if claude := readText(t, "CLAUDE.md"); !strings.Contains(claude, "\n@.agnostic-ai/local/memory/MEMORY.md\n") {
		t.Errorf("CLAUDE.md lost the checkout import:\n%s", claude)
	}
	if local := readText(t, filepath.Join(".claude", "settings.local.json")); !strings.Contains(local, filepath.ToSlash(parent)) {
		t.Errorf("settings.local.json is personal and should name the repo store:\n%s", local)
	}
}

func TestHookMemory_ReadsTheRepoStore(t *testing.T) {
	parent := repoMemoryProject(t, true)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	store := filepath.FromSlash(repoStore(t, parent, readText(t, "CLAUDE.md")))
	writeFile(t, filepath.Join(store, "MEMORY.md"), "- [Tabs](tabs.md): the user wants tabs\n")
	writeFile(t, filepath.Join(".agnostic-ai", "local", "memory", "MEMORY.md"), "- [Stale](stale.md): checkout store\n")

	got := runHookMemory(t, "--target", "codex")
	if !strings.Contains(got, "the user wants tabs") || strings.Contains(got, "checkout store") || !strings.Contains(got, filepath.ToSlash(store)) {
		t.Errorf("hook output:\n%s", got)
	}
}

func TestMemoryList_ReadsTheRepoStore(t *testing.T) {
	parent := repoMemoryProject(t, true)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	store := filepath.FromSlash(repoStore(t, parent, readText(t, "CLAUDE.md")))
	writeFile(t, filepath.Join(store, "tabs.md"), "---\nname: Tabs\ndescription: tabs\nmetadata:\n  type: user\n---\nTabs.\n")
	writeFile(t, filepath.Join(store, "MEMORY.md"), "- [Tabs](tabs.md): the user wants tabs\n")

	var out strings.Builder
	root := NewRootCmd("test")
	root.SetOut(&out)
	root.SetArgs([]string{"memory", "list"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "personal") || !strings.Contains(out.String(), "Tabs") {
		t.Errorf("memory list:\n%s", out.String())
	}
}
