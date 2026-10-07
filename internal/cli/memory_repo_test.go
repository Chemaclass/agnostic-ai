package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// repoMemoryProject sets up a Git project whose agnostic-ai.local.yaml
// sets memory.personal: repo, and returns the repo store it should use.
func repoMemoryProject(t *testing.T, gitignore bool) string {
	t.Helper()
	// Sync names the store by its resolved path; macOS temp dirs sit
	// behind the /var link.
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", home)
	testutil.TempCwd(t)
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	cfg := "version: 1\ntargets: [claude, codex, cursor, gemini, opencode, kilo, qoder]\nbuiltins: [memory]\n"
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
	if text := readText(t, "opencode.json"); !strings.Contains(text, `"`+store+`/MEMORY.md"`) || strings.Contains(text, ".agnostic-ai/local/memory") {
		t.Errorf("opencode.json instructions:\n%s", text)
	}
	// Kilo drops project-declared files outside the root, so the store index stays out.
	if text := readText(t, "kilo.jsonc"); strings.Contains(text, filepath.ToSlash(parent)) || !strings.Contains(text, `".agnostic-ai/memory/MEMORY.md"`) {
		t.Errorf("kilo.jsonc instructions:\n%s", text)
	}
	var qoder struct {
		Permissions struct {
			AdditionalDirectories []string `json:"additionalDirectories"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(readText(t, filepath.Join(".qoder", "settings.json"))), &qoder); err != nil || len(qoder.Permissions.AdditionalDirectories) != 1 || qoder.Permissions.AdditionalDirectories[0] != store {
		t.Errorf("qoder additionalDirectories = %v (%v)", qoder.Permissions.AdditionalDirectories, err)
	}
	if text := readText(t, filepath.Join(".cursor", "cli.json")); !strings.Contains(text, `"Write(`+store+`/**)"`) {
		t.Errorf("cursor cli.json:\n%s", text)
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

	for _, file := range []string{"CLAUDE.md", "opencode.json", "kilo.jsonc", filepath.Join(".gemini", "settings.json"), filepath.Join(".codex", "config.toml"), filepath.Join(".qoder", "settings.json")} {
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

func TestSync_LeavingRepoModeDropsTheRepoIndexFromInstructions(t *testing.T) {
	parent := repoMemoryProject(t, true)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	for _, file := range []string{"opencode.json", "kilo.jsonc"} {
		text := readText(t, file)
		if strings.Contains(text, filepath.ToSlash(parent)) || !strings.Contains(text, `".agnostic-ai/local/memory/MEMORY.md"`) {
			t.Errorf("%s instructions after leaving repo mode:\n%s", file, text)
		}
	}
}

func TestSync_MovingTheHomeDropsTheOldRepoIndexFromInstructions(t *testing.T) {
	oldStores := repoMemoryProject(t, true)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Instructions []string `json:"instructions"`
	}
	if err := json.Unmarshal([]byte(readText(t, "opencode.json")), &doc); err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(doc.Instructions, func(s string) bool { return strings.HasPrefix(s, filepath.ToSlash(oldStores)) })
	if i < 0 {
		t.Fatalf("first sync instructions: %v", doc.Instructions)
	}
	oldIndex := doc.Instructions[i]
	// A path the user listed in the old home is theirs, not sync's.
	userIndex := filepath.ToSlash(filepath.Join(oldStores, "notes", "MEMORY.md"))
	doc.Instructions = append(doc.Instructions, userIndex)
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, "opencode.json", string(raw))

	newHome, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", newHome)
	for range 2 {
		if err := runSync(t); err != nil {
			t.Fatal(err)
		}
	}
	if err := runSync(t, "--check"); err != nil {
		t.Fatalf("sync --check after moving the home: %v", err)
	}

	doc.Instructions = nil
	if err := json.Unmarshal([]byte(readText(t, "opencode.json")), &doc); err != nil {
		t.Fatal(err)
	}
	newStores := filepath.ToSlash(filepath.Join(newHome, "local", "memory"))
	if slices.Contains(doc.Instructions, oldIndex) || !slices.Contains(doc.Instructions, userIndex) ||
		!slices.ContainsFunc(doc.Instructions, func(s string) bool { return strings.HasPrefix(s, newStores) }) {
		t.Errorf("instructions after moving the home: %v", doc.Instructions)
	}
}

// opencodeInstructions reads the instructions list of opencode.json.
func opencodeInstructions(t *testing.T) []string {
	t.Helper()
	var doc struct {
		Instructions []string `json:"instructions"`
	}
	if err := json.Unmarshal([]byte(readText(t, "opencode.json")), &doc); err != nil {
		t.Fatal(err)
	}
	return doc.Instructions
}

func TestSync_MovingTheHomeDropsAnUnclaimedIndexOfThisRepository(t *testing.T) {
	stores := repoMemoryProject(t, true)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	list := opencodeInstructions(t)
	i := slices.IndexFunc(list, func(s string) bool { return strings.HasPrefix(s, filepath.ToSlash(stores)) })
	if i < 0 {
		t.Fatalf("first sync instructions: %v", list)
	}
	// An older release left this repository's index from a former home
	// unclaimed after the move.
	slug := filepath.Base(filepath.Dir(list[i]))
	stale := filepath.ToSlash(filepath.Join(t.TempDir(), "local", "memory", slug, "MEMORY.md"))
	raw, err := json.Marshal(map[string]any{"instructions": append(list, stale)})
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, "opencode.json", string(raw))
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	if got := opencodeInstructions(t); slices.Contains(got, stale) {
		t.Errorf("instructions keep the former home's index: %v", got)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Fatalf("sync --check: %v", err)
	}
}

func TestSync_TurningMemoryOffAfterMovingTheHomeLeavesNoIndex(t *testing.T) {
	repoMemoryProject(t, true)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	newHome, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", newHome)
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\ngitignore:\n  enabled: true\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	if text, err := os.ReadFile("opencode.json"); err == nil && strings.Contains(string(text), "MEMORY.md") {
		t.Errorf("opencode.json keeps a memory index after memory is off:\n%s", text)
	}
}

func TestSync_CreatesThePrivateRepoStore(t *testing.T) {
	parent := repoMemoryProject(t, true)
	if err := runSync(t, "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(parent); len(entries) != 0 {
		t.Fatalf("dry run created %v", entries)
	}
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	store := filepath.FromSlash(repoStore(t, parent, readText(t, "CLAUDE.md")))
	info, err := os.Stat(store)
	if err != nil || !info.IsDir() {
		t.Fatalf("store %s not created: %v", store, err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Errorf("store mode = %v, want 0700", info.Mode().Perm())
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

func TestSync_RepoPersonalMemoryHonorsGitignoreOverride(t *testing.T) {
	for _, format := range []string{"text", "json", "preview"} {
		t.Run(format, func(t *testing.T) {
			parent := repoMemoryProject(t, true)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
			args := []string{"sync", "--gitignore", "off"}
			if format == "json" {
				args = append(args, "--json")
			}
			var out strings.Builder
			cmd := NewRootCmd("test")
			cmd.SetOut(&out)
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			if body := readText(t, "CLAUDE.md"); strings.Contains(body, filepath.ToSlash(parent)) {
				t.Errorf("committable CLAUDE.md exposes the personal store: %s", body)
			}
			if format == "preview" {
				out.Reset()
				cmd = NewRootCmd("test")
				cmd.SetOut(&out)
				cmd.SetArgs([]string{"sync", "--dry-run", "--json", "--gitignore", "off"})
				if err := cmd.Execute(); err != nil {
					t.Fatal(err)
				}
				var got syncJSONOutput
				if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
					t.Fatal(err)
				}
				if len(got.Writes) > 0 {
					t.Errorf("preview differs from sync with the same override: %s", out.String())
				}
			}
			if err := runSync(t, "--check", "--gitignore", "off"); err != nil {
				t.Errorf("sync --check differs from sync with the same override: %v", err)
			}
		})
	}
}

func TestSync_RepoPersonalMemoryHonorsAllowedOutputPaths(t *testing.T) {
	for _, path := range []string{"CLAUDE.md", ".codex/config.toml", ".gemini/settings.json", "opencode.json", "kilo.jsonc"} {
		t.Run(path, func(t *testing.T) {
			parent := repoMemoryProject(t, true)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, gemini, opencode, kilo]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n  allow: ["+path+"]\n")
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.FromSlash(path))
			if err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			if strings.Contains(string(data), filepath.ToSlash(parent)) {
				t.Errorf("allowed output %s exposes the personal store: %s", path, data)
			}
			if err := exec.Command("git", "check-ignore", "-q", path).Run(); err == nil {
				t.Errorf("allowed output %s is ignored", path)
			}
			if local := readText(t, ".claude/settings.local.json"); !strings.Contains(local, filepath.ToSlash(parent)) {
				t.Errorf("personal settings lost the repo store: %s", local)
			}
			if path != "CLAUDE.md" && !strings.Contains(readText(t, "CLAUDE.md"), filepath.ToSlash(parent)) {
				t.Error("an unrelated allow disabled repo memory in ignored CLAUDE.md")
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check after sync: %v", err)
			}
		})
	}
}

// A directory the user adds by hand after sync stopped managing it stays,
// even when it is the path an earlier sync wrote.
func TestSync_ReaddedStoreDirectorySurvivesAfterLeavingRepoMode(t *testing.T) {
	for _, tc := range []struct{ target, file, object, key string }{
		{"qoder", filepath.Join(".qoder", "settings.json"), "permissions", "additionalDirectories"},
		{"gemini", filepath.Join(".gemini", "settings.json"), "context", "includeDirectories"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			parent := repoMemoryProject(t, true)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
			writeFile(t, tc.file, `{"`+tc.object+`":{"`+tc.key+`":["native"]}}`)
			list := func() []string {
				t.Helper()
				var doc map[string]map[string]any
				if err := json.Unmarshal([]byte(readText(t, tc.file)), &doc); err != nil {
					t.Fatal(err)
				}
				var out []string
				for _, v := range doc[tc.object][tc.key].([]any) {
					out = append(out, v.(string))
				}
				return out
			}
			addByHand := func(dir string) {
				t.Helper()
				var doc map[string]any
				if err := json.Unmarshal([]byte(readText(t, tc.file)), &doc); err != nil {
					t.Fatal(err)
				}
				object := doc[tc.object].(map[string]any)
				object[tc.key] = append(object[tc.key].([]any), dir)
				data, err := json.Marshal(doc)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, tc.file, string(data))
			}

			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			got := list()
			if len(got) != 2 || !strings.HasPrefix(got[1], filepath.ToSlash(parent)+"/") {
				t.Fatalf("repo mode list = %v", got)
			}
			store := got[1]

			writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			if got := list(); len(got) != 1 || got[0] != "native" {
				t.Fatalf("after leaving repo mode = %v", got)
			}

			addByHand(store)
			for range 2 {
				if err := runSync(t); err != nil {
					t.Fatal(err)
				}
				if got := list(); len(got) != 2 || got[1] != store {
					t.Fatalf("hand-added directory lost: %v", got)
				}
			}
		})
	}
}

// memoryPaths runs `memory path` in the working directory and returns
// each store's folder by scope.
func memoryPaths(t *testing.T) map[string]string {
	t.Helper()
	out, err := runRoot(t, "memory", "path")
	if err != nil {
		t.Fatalf("memory path: %v\n%s", err, out)
	}
	paths := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		scope, dir, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("line %q in:\n%s", line, out)
		}
		paths[scope] = strings.TrimSpace(dir)
	}
	return paths
}

func TestMemoryPath_CheckoutFoldersFromASubdirectory(t *testing.T) {
	root := t.TempDir()
	testutil.Chdir(t, root)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\nbuiltins: [memory]\n")
	if err := os.MkdirAll(filepath.Join(root, "pkg", "deep"), 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, filepath.Join(root, "pkg", "deep"))

	got := memoryPaths(t)

	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.ToSlash(filepath.Join(real, ".agnostic-ai", "memory")); got["project"] != want {
		t.Errorf("project = %q, want %q", got["project"], want)
	}
	if want := filepath.ToSlash(filepath.Join(real, ".agnostic-ai", "local", "memory")); got["personal"] != want {
		t.Errorf("personal = %q, want %q", got["personal"], want)
	}
	if _, err := os.Stat(got["personal"]); err == nil {
		t.Errorf("memory path created %s", got["personal"])
	}
}

func TestMemoryPath_RepoModeSharesOneFolderAcrossWorktrees(t *testing.T) {
	parent := repoMemoryProject(t, true)
	project, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	writeFile(t, ".gitignore", "agnostic-ai.local.yaml\n")
	git(project, "add", "agnostic-ai.yaml", ".gitignore")
	git(project, "commit", "-q", "-m", "init")
	linked := filepath.Join(t.TempDir(), "linked")
	git(project, "worktree", "add", "-q", linked)
	writeFile(t, filepath.Join(linked, "agnostic-ai.local.yaml"), "memory:\n  personal: repo\n")
	if err := os.MkdirAll(filepath.Join(linked, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	main := memoryPaths(t)
	testutil.Chdir(t, filepath.Join(linked, "sub"))
	other := memoryPaths(t)

	if !strings.HasPrefix(main["personal"], filepath.ToSlash(parent)+"/") {
		t.Errorf("personal = %q, want a folder under %s", main["personal"], parent)
	}
	if other["personal"] != main["personal"] {
		t.Errorf("worktrees differ: %q and %q", main["personal"], other["personal"])
	}
	if other["project"] == main["project"] || !strings.HasSuffix(other["project"], "linked/.agnostic-ai/memory") {
		t.Errorf("project folder should follow the worktree: %q", other["project"])
	}
	if _, err := os.Stat(main["personal"]); err == nil {
		t.Errorf("memory path created %s", main["personal"])
	}
}

func TestMemoryPath_FailsOutsideAProject(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	if out, err := runRoot(t, "memory", "path"); err == nil {
		t.Errorf("want an error, got:\n%s", out)
	}
}

// The user's global source root is often a Git checkout. It is never a
// project, whichever path reaches it.
func TestMemoryProjectRoot_RejectsTheGlobalSourceRootCheckout(t *testing.T) {
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "home-link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", link)
	if out, err := exec.Command("git", "-C", real, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	writeFile(t, filepath.Join(real, "sub", "keep"), "")
	for _, dir := range []string{real, filepath.Join(real, "sub"), link, filepath.Join(link, "sub")} {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			testutil.Chdir(t, dir)
			if out, err := runRoot(t, "memory", "path"); err == nil {
				t.Errorf("memory path treated the global root as a project:\n%s", out)
			}
			if got := runHookMemory(t); got != "" {
				t.Errorf("hook memory printed for the global root:\n%s", got)
			}
		})
	}
}

// A repo-mode project whose config does not load must not fall back to
// the checkout folder: tools would save to a store that disappears once the
// config is fixed.
func TestMemoryPath_InvalidConfigInRepoModeIsAnError(t *testing.T) {
	for name, local := range map[string]string{
		"malformed":   "memory: [personal\n",
		"unknown key": "memory:\n  personal: repo\nnot-a-key: 1\n",
	} {
		t.Run(name, func(t *testing.T) {
			repoMemoryProject(t, true)
			writeFile(t, "agnostic-ai.local.yaml", local)

			if out, err := runRoot(t, "memory", "path"); err == nil {
				t.Errorf("want an error, got:\n%s", out)
			}
			if _, err := runRoot(t, "memory", "list"); err == nil {
				t.Error("memory list used the checkout store")
			}
			got := runHookMemory(t, "--target", "codex")
			if strings.Contains(got, ".agnostic-ai/local/memory") || !strings.Contains(got, "agnostic-ai memory path") {
				t.Errorf("hook should not name the checkout folder, and should point to memory path:\n%s", got)
			}
		})
	}
}

func TestMemoryPath_NoConfigUsesTheCheckoutFolders(t *testing.T) {
	testutil.TempCwd(t)
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	got := memoryPaths(t)
	if !strings.HasSuffix(got["personal"], "/.agnostic-ai/local/memory") {
		t.Errorf("personal = %q", got["personal"])
	}
}

// On a case-insensitive filesystem, a global root spelled with another
// case is still the same folder as Git's top level.
func TestMemoryProjectRoot_RejectsTheGlobalRootSpelledInAnotherCase(t *testing.T) {
	real := filepath.Join(t.TempDir(), "Global-Home")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(filepath.Dir(real), "global-home")
	a, errA := os.Stat(real)
	b, errB := os.Stat(other)
	if errA != nil || errB != nil || !os.SameFile(a, b) {
		t.Skip("the temp filesystem is case-sensitive")
	}
	t.Setenv("AGNOSTIC_AI_HOME", other)
	if out, err := exec.Command("git", "-C", real, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	writeFile(t, filepath.Join(real, "agnostic-ai.yaml"), "version: 1\ntargets: [codex]\n")
	writeFile(t, filepath.Join(real, "sub", "keep"), "")
	for _, dir := range []string{real, filepath.Join(real, "sub")} {
		testutil.Chdir(t, dir)
		if out, err := runRoot(t, "memory", "path"); err == nil {
			t.Errorf("memory path treated the global root as a project from %s:\n%s", dir, out)
		}
	}
}

// cli.json holds permissions alone, so a committed Cursor kind that never
// writes it keeps the repo store's Write rule, and committed settings drop it.
func TestSync_CursorRepoMemoryRuleFollowsTheCommittedKinds(t *testing.T) {
	for _, tc := range []struct {
		commit string
		want   bool
	}{
		{"cursor:reviews", true},
		{"cursor:settings", false},
	} {
		t.Run(tc.commit, func(t *testing.T) {
			parent := repoMemoryProject(t, true)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n  commit: ["+tc.commit+"]\n")
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			text, _ := os.ReadFile(filepath.Join(".cursor", "cli.json"))
			if got := strings.Contains(string(text), "Write("+filepath.ToSlash(parent)); got != tc.want {
				t.Errorf("Write rule for the store = %v, want %v:\n%s", got, tc.want, text)
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check after sync: %v", err)
			}
		})
	}
}

// Cursor CLI refuses a cli.json without both lists, and the file sync
// created leaves again with the repo store.
func TestSync_CursorCLIConfigHasBothListsAndLeavesWithRepoMode(t *testing.T) {
	repoMemoryProject(t, true)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Permissions map[string][]string `json:"permissions"`
	}
	if err := json.Unmarshal([]byte(readText(t, filepath.Join(".cursor", "cli.json"))), &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc.Permissions["deny"]; !ok || len(doc.Permissions["allow"]) != 1 {
		t.Errorf("permissions = %v, want the Write rule and a deny list", doc.Permissions)
	}
	// A second sync finds the empty list on disk and keeps claiming it.
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(".cursor", "cli.json")); !os.IsNotExist(err) {
		text, _ := os.ReadFile(filepath.Join(".cursor", "cli.json"))
		t.Errorf("cli.json stayed after repo mode: %s", text)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}
