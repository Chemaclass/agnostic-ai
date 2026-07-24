package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/cli"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// goldenTargets lists every built-in adapter covered by snapshot golden tests.
var goldenTargets = []string{
	"aider",
	"amp",
	"antigravity",
	"cline",
	"claude",
	"codex",
	"continue",
	"copilot",
	"crush",
	"cursor",
	"gemini",
	"junie",
	"kiro",
	"opencode",
	"trae",
	"warp",
	"windsurf",
	"zed",
	"jules",
	"goose",
	"augment",
	"qoder",
	"openhands",
	"factory",
	"kilo",
}

// TestGolden runs a sync against a shared fixture for each built-in target
// and compares the emitted files against committed golden snapshots.
//
// Regenerate with: UPDATE_GOLDEN=1 go test ./tests/integration/ -run TestGolden
func TestGolden(t *testing.T) {
	for _, target := range goldenTargets {
		t.Run(target, func(t *testing.T) {
			runGolden(t, target)
		})
	}
}

func runGolden(t *testing.T, target string) {
	t.Helper()

	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}

	dir := setupGoldenFixture(t)
	before := snapFiles(t, dir)
	testutil.Chdir(t, dir)

	root := cli.NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", target, "--gitignore=off"})
	if err := root.Execute(); err != nil {
		t.Fatalf("sync -t %s: %v", target, err)
	}

	output := diffSnaps(before, snapFiles(t, dir))
	// snapFiles uses filepath.ToSlash so all keys use forward slashes.
	delete(output, ".agnostic-ai/.sync-state")

	expectedDir := filepath.Join(packageDir, "fixtures", "golden", target)

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		updateGolden(t, expectedDir, output)
		return
	}

	compareGolden(t, expectedDir, output, target)
}

// snapFiles returns a map of relative path → content for every regular file
// under dir.
func snapFiles(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}

// diffSnaps returns entries from after that are new or changed relative to
// before.
func diffSnaps(before, after map[string]string) map[string]string {
	out := map[string]string{}
	for path, content := range after {
		if old, ok := before[path]; !ok || old != content {
			out[path] = content
		}
	}
	return out
}

// updateGolden writes output files into expectedDir, replacing any prior
// snapshot.
func updateGolden(t *testing.T, expectedDir string, output map[string]string) {
	t.Helper()
	if err := os.RemoveAll(expectedDir); err != nil {
		t.Fatalf("clean %s: %v", expectedDir, err)
	}
	for rel, content := range output {
		dst := filepath.Join(expectedDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(dst), err)
		}
		if err := os.WriteFile(dst, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", dst, err)
		}
	}
	t.Logf("golden updated: %s (%d files)", expectedDir, len(output))
}

// compareGolden checks that output matches the committed snapshot in
// expectedDir. Reports missing, extra, and mismatched files.
func compareGolden(t *testing.T, expectedDir string, output map[string]string, target string) {
	t.Helper()

	expected := map[string]string{}
	if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
		t.Fatalf("no golden snapshot for %s; run: UPDATE_GOLDEN=1 go test ./tests/integration/ -run TestGolden/%s", target, target)
	}
	if err := filepath.WalkDir(expectedDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(expectedDir, path)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		expected[filepath.ToSlash(rel)] = string(data)
		return nil
	}); err != nil {
		t.Fatalf("walk %s: %v", expectedDir, err)
	}

	for rel, want := range expected {
		got, ok := output[rel]
		if !ok {
			t.Errorf("missing output file: %s", rel)
			continue
		}
		if got != want {
			t.Errorf("content mismatch: %s\n--- want ---\n%s\n--- got ---\n%s", rel, want, got)
		}
	}
	for rel := range output {
		if _, ok := expected[rel]; !ok {
			t.Errorf("unexpected output file: %s (run UPDATE_GOLDEN=1 to accept)", rel)
		}
	}
}

// setupGoldenFixture copies the committed fixture into a fresh temp dir and
// returns its path without changing the working directory.
func setupGoldenFixture(t *testing.T) string {
	t.Helper()

	src := filepath.Join("fixtures", "golden")
	dst := t.TempDir()

	if err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		// Skip the per-target expected snapshot directories.
		for _, tgt := range goldenTargets {
			if rel == tgt || len(rel) > len(tgt) && rel[:len(tgt)+1] == tgt+string(filepath.Separator) {
				return fs.SkipDir
			}
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}
