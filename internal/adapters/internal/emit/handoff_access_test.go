package emit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSharedHandoffWriteRoot_RequiresPrivateMainStore(t *testing.T) {
	modes := []string{"ignored", "unignored", "tracked", "main checkout", "disabled", "missing subproject", "regular file"}
	if runtime.GOOS != "windows" {
		modes = append(modes, "symlink")
	}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			main, linked := t.TempDir(), filepath.Join(t.TempDir(), "linked")
			gitIn(t, main, "init", "-q")
			gitIn(t, main, "config", "maintenance.auto", "false")
			gitIn(t, main, "-c", "user.email=t@t", "-c", "user.name=t", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", "init")
			gitIn(t, main, "worktree", "add", "-q", "-b", "handoff", linked)
			ignore := "**/.agnostic-ai/local/\n"
			if mode == "unignored" {
				ignore = "!.agnostic-ai/\n!.agnostic-ai/local/\n!.agnostic-ai/local/handoffs/\n"
			}
			if err := os.WriteFile(filepath.Join(main, ".gitignore"), []byte(ignore), 0o600); err != nil {
				t.Fatal(err)
			}
			store := filepath.Join(main, ".agnostic-ai", "local", "handoffs")
			if mode == "regular file" {
				if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(store, []byte("keep this file"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "tracked" {
				if err := os.MkdirAll(store, 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(store, "tracked.md"), []byte("tracked"), 0o600); err != nil {
					t.Fatal(err)
				}
				gitIn(t, main, "add", "-f", ".agnostic-ai/local/handoffs/tracked.md")
			}
			if mode == "symlink" {
				if err := os.MkdirAll(filepath.Dir(store), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(t.TempDir(), store); err != nil {
					t.Fatal(err)
				}
			}
			cwd := linked
			if mode == "main checkout" {
				cwd = main
			}
			if mode == "missing subproject" {
				cwd = filepath.Join(linked, "subproject")
				if err := os.Mkdir(cwd, 0o700); err != nil {
					t.Fatal(err)
				}
				store = filepath.Join(main, "subproject", ".agnostic-ai/local/handoffs")
			}
			testutil.Chdir(t, cwd)
			cfg := &config.Config{Builtins: []string{"handoff"}}
			if mode == "disabled" {
				cfg.Builtins = nil
			}
			got := SharedHandoffWriteRoot(cfg)
			wantRoot := mode == "ignored" || mode == "missing subproject"
			if wantRoot && !SamePath(got, store) || !wantRoot && got != "" {
				t.Errorf("shared handoff root = %q, want root: %v (%s)", got, wantRoot, store)
			}
		})
	}
}

func TestCreateHandoffStore_PreviewsDoNotCreateDirectories(t *testing.T) {
	for _, mode := range []string{"write", "dry run", "capture"} {
		t.Run(mode, func(t *testing.T) {
			store := filepath.Join(t.TempDir(), "handoffs")
			sess := NewSession()
			if mode == "capture" {
				sess.StartCapture()
			}
			if err := sess.CreateHandoffStore(store, mode == "dry run"); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(store)
			if mode != "write" {
				if !os.IsNotExist(err) {
					t.Errorf("preview created store: %v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			} else if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
				t.Errorf("store permissions = %o, want 700", info.Mode().Perm())
			}
		})
	}
}

func TestPreserveHandoffConfig_MissingOwnedFileCanBeGenerated(t *testing.T) {
	for _, target := range []string{"codex", "qoder", "claude"} {
		t.Run(target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			path := filepath.Join("native", "settings")
			root := "/project/.agnostic-ai/local/handoffs"
			oldSum, oldKeys := PriorOutputSum, PriorMergedKeys
			t.Cleanup(func() {
				PriorOutputSum, PriorMergedKeys = oldSum, oldKeys
			})
			PriorOutputSum = func(string) string { return ContentSum(root) }
			PriorMergedKeys = func(string) []MergedKey {
				return []MergedKey{{Path: []string{"permissions", "additionalDirectories"}, Items: []string{ContentSum(root)}}}
			}
			sess := NewSession()
			preserved, err := sess.PreserveHandoffConfig(&config.Config{}, path, target)
			if err != nil || preserved {
				t.Errorf("missing native file should allow generation: preserved=%v, err=%v", preserved, err)
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Errorf("preservation created a missing file: %v", err)
			}
		})
	}
}

func TestPreserveHandoffConfig_MissingFileUnderLinkedParentStaysPrivate(t *testing.T) {
	for _, mode := range []string{"inside", "outside", "dangling", "checkout root"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			testutil.Chdir(t, root)
			destination := t.TempDir()
			switch mode {
			case "inside":
				destination = filepath.Join(root, "private")
				if err := os.Mkdir(destination, 0o700); err != nil {
					t.Fatal(err)
				}
			case "dangling":
				destination = filepath.Join(destination, "missing")
			case "checkout root":
				destination = root
			}
			if err := os.Symlink(destination, "native"); err != nil {
				if runtime.GOOS == "windows" {
					t.Skipf("symlink unavailable: %v", err)
				}
				t.Fatal(err)
			}
			oldSum, oldKeys := PriorOutputSum, PriorMergedKeys
			t.Cleanup(func() {
				PriorOutputSum, PriorMergedKeys = oldSum, oldKeys
			})
			store := "/project/.agnostic-ai/local/handoffs"
			PriorOutputSum = func(string) string { return ContentSum(store) }
			PriorMergedKeys = func(string) []MergedKey {
				return []MergedKey{{Path: []string{"permissions", "additionalDirectories"}, Items: []string{ContentSum(store)}}}
			}
			for _, target := range []string{"codex", "qoder", "claude"} {
				path := filepath.Join("native", "settings")
				sess := NewSession()
				sess.StartCapture()
				sess.StartDetailedRecording()
				preserved, err := sess.PreserveHandoffConfig(&config.Config{}, path, target)
				if err != nil || !preserved {
					t.Errorf("%s missing config under linked parent was not preserved: %v, %v", target, preserved, err)
				}
				writes := sess.StopDetailedRecording()
				if len(writes) != 1 || writes[0].Path != path || writes[0].Action != "skip" || len(writes[0].Keys) == 0 {
					t.Errorf("%s prior output lost its unchanged ownership: %+v", target, writes)
				}
				if captured := sess.StopCapture(); len(captured) != 0 {
					t.Errorf("%s planned a write through the linked parent: %+v", target, captured)
				}
			}
			if _, err := os.Lstat(filepath.Join(destination, "settings")); !os.IsNotExist(err) {
				t.Errorf("missing destination changed: %v", err)
			}
		})
	}
}

func TestHandoffConfigWritable_RejectsNativeParentLinkedToCheckoutRoot(t *testing.T) {
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	testutil.Chdir(t, root)
	if err := os.Symlink(root, "native"); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	if got := handoffConfigPathWritable(filepath.Join("native", "settings"), "codex"); got {
		t.Error("native parent linked to checkout root became writable")
	}
	if _, err := os.Lstat(filepath.Join(root, "settings")); !os.IsNotExist(err) {
		t.Errorf("writability check changed root settings: %v", err)
	}
}

func TestPreserveHandoffConfig_MissingFileInAliasedCheckoutCanBeGenerated(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(t.TempDir(), "checkout alias")
	if err := os.Symlink(root, alias); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	testutil.Chdir(t, alias)
	t.Setenv("PWD", alias)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if cwd != alias {
		if runtime.GOOS == "windows" {
			t.Skip("Windows Getwd does not use the PWD alias")
		}
		t.Fatalf("Getwd = %q, want PWD alias %q", cwd, alias)
	}
	oldSum := PriorOutputSum
	t.Cleanup(func() { PriorOutputSum = oldSum })
	PriorOutputSum = func(string) string { return ContentSum("owned native config") }
	path := filepath.Join(".codex", "config.toml")
	preserved, err := NewSession().PreserveHandoffConfig(&config.Config{}, path, "codex")
	if err != nil || preserved {
		t.Errorf("missing config in aliased checkout should regenerate: preserved=%v, err=%v", preserved, err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("preservation created a native config: %v", err)
	}
	gitIn(t, root, "init", "-q")
	cfg := &config.Config{Builtins: []string{"handoff"}}
	cfg.Gitignore.Enabled = true
	if got := NewSession().HandoffConfigWritable(cfg, path, "codex"); !got {
		t.Error("Git checkout root alias blocked native settings generation")
	}
}

func TestPreserveHandoffConfig_DirectoryReachedThroughParentLinkStaysPrivate(t *testing.T) {
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	testutil.Chdir(t, root)
	if err := os.Symlink(filepath.Dir(root), "native"); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	path := filepath.Join("native", filepath.Base(root), "settings")
	oldSum := PriorOutputSum
	t.Cleanup(func() { PriorOutputSum = oldSum })
	PriorOutputSum = func(string) string { return ContentSum("owned native config") }
	preserved, err := NewSession().PreserveHandoffConfig(&config.Config{}, path, "codex")
	if err != nil || !preserved {
		t.Errorf("directory reached through linked parent was not preserved: %v, %v", preserved, err)
	}
	cfg := &config.Config{Builtins: []string{"handoff"}}
	cfg.Gitignore.Enabled = true
	if got := NewSession().HandoffConfigWritable(cfg, path, "codex"); got {
		t.Error("canonical checkout equality skipped a linked ancestor")
	}
	if _, err := os.Lstat(filepath.Join(root, "settings")); !os.IsNotExist(err) {
		t.Errorf("linked parent changed root settings: %v", err)
	}
}

func TestHandoffConfigWritable_AliasedCheckoutSubprojectCanBeGenerated(t *testing.T) {
	root := t.TempDir()
	gitIn(t, root, "init", "-q")
	if err := os.Mkdir(filepath.Join(root, "subproject"), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "checkout alias")
	if err := os.Symlink(root, alias); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	cwd := filepath.Join(alias, "subproject")
	testutil.Chdir(t, cwd)
	t.Setenv("PWD", cwd)
	if got, err := os.Getwd(); err != nil || got != cwd {
		if runtime.GOOS == "windows" {
			t.Skip("Windows Getwd does not use the PWD alias")
		}
		t.Fatalf("Getwd = %q, %v, want PWD alias %q", got, err, cwd)
	}
	cfg := &config.Config{Builtins: []string{"handoff"}}
	cfg.Gitignore.Enabled = true
	path := filepath.Join(".codex", "config.toml")
	if got := NewSession().HandoffConfigWritable(cfg, path, "codex"); !got {
		t.Error("checkout alias above the subproject blocked native generation")
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Errorf("writability check created native settings: %v", err)
	}
}
