package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// linkedPage builds <root>/run with pages/claude/docs-1-hooks.body linking to
// <root>/<store>/<name>, the layout scripts/docfetch.sh writes for an audit run.
func linkedPage(t *testing.T, store, name string, body []byte) (run, path, blob string) {
	t.Helper()
	root := t.TempDir()
	run = filepath.Join(root, "run")
	if err := os.MkdirAll(filepath.Join(run, "pages", "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, store), 0o755); err != nil {
		t.Fatal(err)
	}
	if name == "" {
		name = digest(body)
	}
	blob = filepath.Join(root, store, name)
	if err := os.WriteFile(blob, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(blob, filepath.Join(run, "pages", "claude", "docs-1-hooks.body")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	return run, "pages/claude/docs-1-hooks.body", blob
}

func TestArtifactReadsAPageStoredOnceByHash(t *testing.T) {
	body := []byte("Hooks run before tools.")
	run, path, _ := linkedPage(t, filepath.Join("snapshots", "blobs"), "", body)
	a, err := getArtifact(run, path)
	if err != nil {
		t.Fatal(err)
	}
	if a.SHA256 != digest(body) || a.Path != path {
		t.Errorf("artifact = %+v", a)
	}
}

func TestArtifactRejectsASharedPageWhoseBytesMoved(t *testing.T) {
	run, path, _ := linkedPage(t, filepath.Join("snapshots", "blobs"), digest([]byte("old")), []byte("new"))
	if _, err := getArtifact(run, path); err == nil || !strings.Contains(err.Error(), "do not match their name") {
		t.Errorf("err = %v", err)
	}
}

func TestArtifactRejectsALinkIntoAnotherBlobsFolder(t *testing.T) {
	run, path, _ := linkedPage(t, filepath.Join("cache", "blobs"), "", []byte("cached"))
	if _, err := getArtifact(run, path); err == nil || !strings.Contains(err.Error(), "escapes run directory") {
		t.Errorf("err = %v", err)
	}
}

func TestArtifactRejectsAReportLinkedIntoTheBlobStore(t *testing.T) {
	body := []byte("# claude\n")
	run, _, blob := linkedPage(t, filepath.Join("snapshots", "blobs"), "", body)
	if err := os.Symlink(blob, filepath.Join(run, "claude.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := getArtifact(run, "claude.md"); err == nil || !strings.Contains(err.Error(), "escapes run directory") {
		t.Errorf("err = %v", err)
	}
}
