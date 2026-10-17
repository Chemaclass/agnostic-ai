package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestAgainstTreeExportLinkTargets_PreservesFilesDirectoriesAndChains(t *testing.T) {
	for _, ref := range []string{againstIndex, againstHEAD} {
		t.Run(ref, func(t *testing.T) {
			dir := setupGitRepo(t)
			testutil.Chdir(t, dir)
			git(t, dir, "config", "core.symlinks", "true")
			git(t, dir, "config", "core.autocrlf", "false")
			files := map[string]string{
				"assets/pack/SKILL.md":          "Committed skill.\n",
				"assets/pack/nested/example.md": "Nested asset.\n",
				"assets/pack-note.txt":          "Exact file target.\n",
				"assets/pack-notes.txt":         "Unrelated sibling file.\n",
				"assets/pack-other/ignored.md":  "Unrelated sibling directory.\n",
				"assets/pack.ext/ignored.md":    "Sibling sorting before the slash.\n",
			}
			for p, body := range files {
				mustWriteFile(t, filepath.Join(dir, filepath.FromSlash(p)), body)
			}
			links := map[string]string{
				"links/directory": "../assets/pack",
				"links/file":      "../assets/pack-note.txt",
				"links/chain":     "directory",
				"links/duplicate": "../assets/pack",
			}
			if err := os.MkdirAll(filepath.Join(dir, "links"), 0o755); err != nil {
				t.Fatal(err)
			}
			for p, dest := range links {
				if err := os.Symlink(filepath.FromSlash(dest), filepath.Join(dir, filepath.FromSlash(p))); err != nil {
					t.Skipf("native symlinks unavailable: %v", err)
				}
			}
			git(t, dir, "add", "-A")
			git(t, dir, "commit", "-q", "-m", "linked assets")
			mustWriteFile(t, filepath.Join(dir, "assets", "pack", "SKILL.md"), "Staged skill.\n")
			git(t, dir, "add", "assets/pack/SKILL.md")
			mustWriteFile(t, filepath.Join(dir, "assets", "pack", "SKILL.md"), "Unstaged skill.\n")
			beforeIndex := git(t, dir, "write-tree")
			beforeStatus := git(t, dir, "status", "--porcelain=v1")
			env := os.Environ()
			if ref == againstHEAD {
				env = append(env, "GIT_INDEX_FILE="+filepath.Join(t.TempDir(), "index"))
				if _, err := gitOutput(dir, env, "read-tree", "HEAD"); err != nil {
					t.Fatal(err)
				}
			}
			entries, err := gitOutput(dir, env, "ls-files", "--stage", "-z")
			if err != nil {
				t.Fatal(err)
			}
			tree := &againstTree{
				root: t.TempDir(), toplevel: dir, exportEnv: env,
				exported: map[string]bool{}, modes: map[string]string{}, blobs: map[string]string{},
			}
			if err := tree.exportInputs(entries); err != nil {
				t.Fatalf("export links: %v", err)
			}
			tracked := append([]string(nil), tree.tracked...)

			if err := tree.exportLinkTargets(); err != nil {
				t.Fatalf("export link targets: %v", err)
			}

			wantSkill := "Committed skill.\n"
			if ref == againstIndex {
				wantSkill = "Staged skill.\n"
			}
			for p, want := range map[string]string{
				"links/directory/SKILL.md":          wantSkill,
				"links/chain/SKILL.md":              wantSkill,
				"links/duplicate/nested/example.md": "Nested asset.\n",
				"links/file":                        "Exact file target.\n",
			} {
				got, err := os.ReadFile(filepath.Join(tree.root, filepath.FromSlash(p)))
				if err != nil || string(got) != want {
					t.Errorf("%s: got %q, %v; want %q", p, got, err, want)
				}
			}
			for _, p := range []string{"assets/pack-notes.txt", "assets/pack-other/ignored.md", "assets/pack.ext/ignored.md"} {
				if _, err := os.Lstat(filepath.Join(tree.root, filepath.FromSlash(p))); !os.IsNotExist(err) {
					t.Errorf("unrelated sibling %s was exported: %v", p, err)
				}
			}
			if !reflect.DeepEqual(tree.tracked, tracked) {
				t.Error("export changed the tracked path order")
			}
			if got := git(t, dir, "write-tree"); got != beforeIndex {
				t.Error("export changed the original index")
			}
			if got := git(t, dir, "status", "--porcelain=v1"); got != beforeStatus {
				t.Error("export changed the original working tree status")
			}
		})
	}
}
