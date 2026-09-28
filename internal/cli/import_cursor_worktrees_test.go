package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// A hand-written .cursor/worktrees.json becomes an environment spec whose
// sync writes the same commands back (#1339).
func TestImportFromCursor_ReadsWorktreeSetup(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "worktrees.json"), `{
  "setup-worktree-unix": ["bash scripts/setup-worktree.bash"],
  "setup-worktree-windows": ["npm ci", "copy .env.example .env"]
}`)
	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFileString(t, filepath.Join(dir, "environments", "worktree.yaml"))
	want := "name: worktree\nsetup: bash scripts/setup-worktree.bash\nsetup-windows:\n    - npm ci\n    - copy .env.example .env\n"
	if got != want {
		t.Errorf("environments/worktree.yaml:\n%s\nwant:\n%s", got, want)
	}
}

// Cursor's `setup-worktree` runs on every OS, so its command list fills
// whichever OS key the file leaves unset.
func TestImportFromCursor_WorktreeSetupForEveryOS(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "worktrees.json"), `{
  "setup-worktree": ["npm ci"],
  "setup-worktree-windows": ["npm ci --no-audit"]
}`)
	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFileString(t, filepath.Join(dir, "environments", "worktree.yaml"))
	want := "name: worktree\nsetup: npm ci\nsetup-windows: npm ci --no-audit\n"
	if got != want {
		t.Errorf("environments/worktree.yaml:\n%s\nwant:\n%s", got, want)
	}
}

// A string is a script path Cursor resolves from .cursor/, not a command,
// so it stays a Cursor key under x-cursor.
func TestImportFromCursor_KeepsWorktreeScriptPathsForCursor(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "worktrees.json"), `{
  "setup-worktree-unix": "setup-worktree-unix.sh",
  "setup-worktree-windows": ["npm ci"]
}`)
	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFileString(t, filepath.Join(dir, "environments", "worktree.yaml"))
	want := "name: worktree\nsetup-windows: npm ci\nx-cursor:\n    setup-worktree-unix: setup-worktree-unix.sh\n"
	if got != want {
		t.Errorf("environments/worktree.yaml:\n%s\nwant:\n%s", got, want)
	}
}

// A worktrees.json sync wrote, or an existing spec, is left alone.
func TestImportFromCursor_SkipsSyncedWorktreesAndExistingSpec(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "worktrees.json"), `{"setup-worktree-unix": ["make setup"]}`)
	own := "name: worktree\nsetup: make other\n"
	writeFile(t, filepath.Join(dir, "environments", "worktree.yaml"), own)
	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	if got := readFileString(t, filepath.Join(dir, "environments", "worktree.yaml")); got != own {
		t.Errorf("existing spec changed:\n%s", got)
	}

	synced := t.TempDir()
	writeFile(t, filepath.Join(synced, ".cursor", "worktrees.json"), `{"setup-worktree-unix": ["make setup"]}`)
	writeFile(t, filepath.Join(synced, ".agnostic-ai", ".sync-state"), `{"outputs":[".cursor/worktrees.json"]}`)
	if err := importFromCursor(synced, rootSources()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(synced, "environments", "worktree.yaml")); !os.IsNotExist(err) {
		t.Errorf("a synced worktrees.json was imported: %v", err)
	}
}

// A unix-only list stays a Cursor key: as `setup` it would also start
// running on Windows.
func TestImportFromCursor_KeepsAUnixOnlyListForCursor(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".cursor", "worktrees.json"), `{"setup-worktree-unix": ["make setup"]}`)
	if err := importFromCursor(dir, rootSources()); err != nil {
		t.Fatal(err)
	}
	got := readFileString(t, filepath.Join(dir, "environments", "worktree.yaml"))
	want := "name: worktree\nx-cursor:\n    setup-worktree-unix:\n        - make setup\n"
	if got != want {
		t.Errorf("environments/worktree.yaml:\n%s\nwant:\n%s", got, want)
	}
}
