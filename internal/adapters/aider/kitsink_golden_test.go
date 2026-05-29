package aider

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestKitSink_GoldenSnapshot pins the byte-exact emit footprint for an
// aider sync over the canonical kit-sink bundle (3 rules + 3 agents +
// 3 skills) with both legacy outputs opted in: rules-file at
// CONVENTIONS.md and conf-file at .aider.conf.yml.
//
// Diff regressions (frontmatter key reorder, agent section prefix
// drift, YAML key sort changes, conf-file header churn) trip this
// test and must be acknowledged by either fixing the adapter or
// regenerating the snapshot:
//
//	UPDATE_GOLDEN=1 go test ./internal/adapters/aider/ -run TestKitSink_GoldenSnapshot
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
			"aider": {
				RulesFile: "CONVENTIONS.md",
				ConfFile:  ".aider.conf.yml",
				Model:     "gpt-4o",
				WeakModel: "gpt-4o-mini",
			},
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
		// Skip files that belong to the agnostic-ai source tree.
		relSlash := filepath.ToSlash(rel)
		if strings.HasPrefix(relSlash, ".agnostic-ai/") {
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
