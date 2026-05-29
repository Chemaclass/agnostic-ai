package codex

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKitSink_GoldenSnapshot pins the byte-exact emit footprint for a
// codex sync over the canonical kit-sink bundle. Every supported spec
// kind is exercised with 3+ specimens, hooks span multiple lifecycle
// events, MCPs cover stdio + http + disabled, and the legacy rules
// file is opted in via outputs.codex.rules-file so all six supported
// kinds land at observable paths.
//
// Diff regressions (frontmatter key reorder, hook ordering churn,
// MCP table drift, exec-policy rendering changes) trip the test and
// must be acknowledged by either fixing the adapter or regenerating
// the snapshot:
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/codex/ -run TestKitSink_GoldenSnapshot
func TestKitSink_GoldenSnapshot(t *testing.T) {
	// Resolve the package-relative testdata path before TempCwd
	// switches the working directory; otherwise the relative path
	// would target the temp dir.
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	expectedDir := filepath.Join(origCwd, "testdata", "kitsink")

	dir := testutil.TempCwd(t)
	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"codex": {RulesFile: "AGENTS-rules.md"},
		},
	}
	if err := New().Emit(kitSinkBundle(), cfg, false); err != nil {
		t.Fatalf("emit: %v", err)
	}

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.RemoveAll(expectedDir); err != nil {
			t.Fatalf("clean expected dir: %v", err)
		}
		if err := copyEmittedTree(dir, expectedDir); err != nil {
			t.Fatalf("copy: %v", err)
		}
		t.Logf("kit-sink golden updated: %s", expectedDir)
		return
	}

	got := walkRel(t, dir)
	want, err := loadKitSinkExpected(expectedDir)
	if err != nil {
		t.Fatalf("load expected: %v", err)
	}

	gotSet := map[string]string{}
	for _, rel := range got {
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		gotSet[rel] = string(data)
	}

	for rel, wantBody := range want {
		gotBody, ok := gotSet[rel]
		if !ok {
			t.Errorf("missing expected output %s (run UPDATE_GOLDEN=1 to accept)", rel)
			continue
		}
		if gotBody != wantBody {
			t.Errorf("content mismatch: %s\n--- want ---\n%s\n--- got ---\n%s",
				rel, wantBody, gotBody)
		}
	}
	for rel := range gotSet {
		if _, ok := want[rel]; !ok {
			t.Errorf("unexpected output file: %s (run UPDATE_GOLDEN=1 to accept)", rel)
		}
	}
}

func loadKitSinkExpected(root string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = string(data)
		return nil
	})
	return out, err
}

func copyEmittedTree(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		// Skip files that belong to the agnostic-ai source tree or the
		// project-root AGENTS.md (owned by sync, not the adapter).
		relSlash := filepath.ToSlash(rel)
		if relSlash == "AGENTS.md" || strings.HasPrefix(relSlash, ".agnostic-ai/") {
			return nil
		}
		dst := filepath.Join(dstDir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dst, data, 0o644)
	})
}
