package cli

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// machineRootsIn lists the files under .agnostic-ai, the committed source,
// that name one of roots.
func machineRootsIn(t *testing.T, roots ...string) []string {
	t.Helper()
	var hits []string
	err := filepath.WalkDir(".agnostic-ai", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "local" {
			return filepath.SkipDir
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, root := range roots {
			if strings.Contains(filepath.ToSlash(string(data)), root) {
				hits = append(hits, path)
				break
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func TestImport_AfterSyncInRepoModeWritesNoMachinePath(t *testing.T) {
	parent := repoMemoryProject(t, true)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, windsurf, claude, qoder, opencode, cursor]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	store := repoStore(t, parent, readText(t, filepath.Join(".codex", "config.toml")))

	for _, source := range []string{"codex", "windsurf", "claude", "qoder", "opencode", "cursor"} {
		if err := importArgs([]string{source}); err != nil {
			t.Fatalf("import %s: %v", source, err)
		}
	}
	if hits := machineRootsIn(t, store); len(hits) != 0 {
		t.Errorf("import copied the repo memory store %s into %v", store, hits)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check after import: %v", err)
	}
}

// syncedCodexProject syncs a repo-mode project for Codex and returns the
// repo store sync granted.
func syncedCodexProject(t *testing.T) string {
	t.Helper()
	parent := repoMemoryProject(t, true)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	return repoStore(t, parent, readText(t, filepath.Join(".codex", "config.toml")))
}

func addCodexRoot(t *testing.T, store, root string) {
	t.Helper()
	configPath := filepath.Join(".codex", "config.toml")
	text := readText(t, configPath)
	edited := strings.Replace(text, "writable_roots = [\""+store+"\"]", "writable_roots = [\""+root+"\", \""+store+"\"]", 1)
	if edited == text {
		t.Fatalf("config.toml has no writable_roots line:\n%s", text)
	}
	writeFile(t, configPath, edited)
}

// runLogged runs a command and returns what it logged.
func runLogged(t *testing.T, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	prev := logOut
	logOut = &buf
	defer func() { logOut = prev }()
	root := NewRootCmd("test")
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	var err error
	printed := captureStdout(t, func() { err = root.Execute() })
	if err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return buf.String() + printed
}

func TestImport_DryRunDiffAfterSyncInRepoModeWritesNoMachinePath(t *testing.T) {
	store := syncedCodexProject(t)
	addCodexRoot(t, store, "/opt/shared")

	out := captureStdout(t, func() {
		root := NewRootCmd("test")
		root.SetArgs([]string{"import", "codex", "--dry-run", "--diff"})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(out, "/opt/shared") {
		t.Fatalf("the diff does not show the overlay:\n%s", out)
	}
	if strings.Contains(filepath.ToSlash(out), store) {
		t.Errorf("the diff shows the repo memory store %s:\n%s", store, out)
	}
	if _, err := os.Stat(filepath.Join(".agnostic-ai", "overlays", "codex.config.toml")); err == nil {
		t.Error("dry run wrote the codex overlay")
	}
}

func TestImport_CodexKeepsAnotherProjectsHandoffFolder(t *testing.T) {
	store := syncedCodexProject(t)
	other := "/elsewhere/project/.agnostic-ai/local/handoffs"
	addCodexRoot(t, store, other)

	if err := importArgs([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	overlay := readText(t, filepath.Join(".agnostic-ai", "overlays", "codex.config.toml"))
	if !strings.Contains(overlay, other) || strings.Contains(overlay, store) {
		t.Errorf("overlay:\n%s", overlay)
	}
}

func TestImport_CodexDropsTheStoreAfterLeavingRepoMode(t *testing.T) {
	store := syncedCodexProject(t)
	addCodexRoot(t, store, "/opt/shared")
	writeFile(t, "agnostic-ai.local.yaml", "memory:\n  personal: checkout\n")

	if err := importArgs([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	overlay := readText(t, filepath.Join(".agnostic-ai", "overlays", "codex.config.toml"))
	if strings.Contains(overlay, store) || !strings.Contains(overlay, "/opt/shared") {
		t.Errorf("overlay after leaving repo mode:\n%s", overlay)
	}
}

func TestImport_CodexCleansAnOverlayThatNamesTheStore(t *testing.T) {
	for _, edited := range []bool{true, false} {
		name := "config edited after sync"
		if !edited {
			name = "config as sync wrote it"
		}
		t.Run(name, func(t *testing.T) {
			store := syncedCodexProject(t)
			overlayPath := filepath.Join(".agnostic-ai", "overlays", "codex.config.toml")
			writeFile(t, overlayPath, "[sandbox_workspace_write]\nwritable_roots = [\""+store+"\"]\n")
			writeFile(t, filepath.Join(".agnostic-ai", "rules", "x.md"), "---\nname: x\ndescription: x\n---\nx\n")
			runLogged(t, "sync")

			configPath := filepath.Join(".codex", "config.toml")
			if edited {
				writeFile(t, configPath, readText(t, configPath)+"# edited\n")
			}
			runLogged(t, "import", "codex", "--overwrite")
			if overlay := readText(t, overlayPath); strings.Contains(overlay, store) {
				t.Errorf("overlay still names the store:\n%s", overlay)
			}
			runLogged(t, "sync")
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check: %v", err)
			}
		})
	}
}

func TestImport_CodexSaysWhenItRewritesTheOverlayWithoutComments(t *testing.T) {
	store := syncedCodexProject(t)
	configPath := filepath.Join(".codex", "config.toml")
	writeFile(t, configPath, "# my settings\nsandbox_workspace_write.writable_roots = [\"/opt/shared\", \""+store+"\"]\n")

	printed := runLogged(t, "import", "codex")
	overlay := readText(t, filepath.Join(".agnostic-ai", "overlays", "codex.config.toml"))
	if strings.Contains(overlay, store) || !strings.Contains(overlay, "/opt/shared") {
		t.Errorf("overlay:\n%s", overlay)
	}
	if !strings.Contains(printed, "lost its comments") {
		t.Errorf("no note about the lost comments:\n%s", printed)
	}
}

func TestImport_CodexKeepsUserWritableRootsAndSyncKeepsTheStore(t *testing.T) {
	parent := repoMemoryProject(t, true)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	store := repoStore(t, parent, readText(t, filepath.Join(".codex", "config.toml")))
	configPath := filepath.Join(".codex", "config.toml")
	edited := strings.Replace(readText(t, configPath), "writable_roots = [\""+store+"\"]",
		"# shared scratch\nwritable_roots = [\"/opt/shared\", \""+store+"\"]\nnetwork_access = true", 1)
	if edited == readText(t, configPath) {
		t.Fatalf("config.toml has no writable_roots line:\n%s", readText(t, configPath))
	}
	writeFile(t, configPath, edited)

	if err := importArgs([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	overlay := readText(t, filepath.Join(".agnostic-ai", "overlays", "codex.config.toml"))
	if strings.Contains(overlay, store) {
		t.Errorf("overlay names the repo memory store:\n%s", overlay)
	}
	for _, want := range []string{"/opt/shared", "network_access = true", "# shared scratch"} {
		if !strings.Contains(overlay, want) {
			t.Errorf("overlay lost %q:\n%s", want, overlay)
		}
	}

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	synced := readText(t, configPath)
	if !strings.Contains(synced, "\"/opt/shared\", \""+store+"\"") || strings.Count(synced, "[sandbox_workspace_write]") != 1 {
		t.Errorf("sync dropped a root or split the table:\n%s", synced)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}

func TestImport_CodexDropsTheSharedHandoffStore(t *testing.T) {
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", home)
	main, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "commit.gpgsign=false"}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git(main, "init", "-q")
	cfg := "version: 1\ntargets: [codex]\nbuiltins: [handoff]\ngitignore:\n  enabled: true\n"
	if err := os.WriteFile(filepath.Join(main, "agnostic-ai.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(main, ".gitignore"), []byte(".agnostic-ai/local/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(main, ".agnostic-ai", "local", "handoffs"), 0o700); err != nil {
		t.Fatal(err)
	}
	git(main, "add", "-A")
	git(main, "commit", "-q", "-m", "init")
	linked := main + "-linked"
	git(main, "worktree", "add", "-q", linked)
	linked, err = filepath.EvalSymlinks(linked)
	if err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, linked)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	store := filepath.ToSlash(filepath.Join(main, ".agnostic-ai", "local", "handoffs"))
	if !strings.Contains(readText(t, filepath.Join(".codex", "config.toml")), store) {
		t.Fatalf("sync did not grant the handoff store %s:\n%s", store, readText(t, filepath.Join(".codex", "config.toml")))
	}
	if err := importArgs([]string{"codex"}); err != nil {
		t.Fatal(err)
	}
	if hits := machineRootsIn(t, store); len(hits) != 0 {
		t.Errorf("import copied the handoff store into %v", hits)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check: %v", err)
	}
}
