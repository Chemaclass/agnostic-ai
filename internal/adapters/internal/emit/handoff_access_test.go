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
