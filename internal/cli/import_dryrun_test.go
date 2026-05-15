package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImport_DryRunListsPathsNotContents asserts the new planning shape
// for `import --dry-run`: one line per file path that would be written,
// no file bodies. Closes the issue where dry-run dumped every body.
func TestImport_DryRunListsPathsNotContents(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeFile(t, filepath.Join(dir, "CLAUDE.md"), "## sliced\n\nbody\n")
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\nsources:\n  rules: rules\n  agents: agents\n  skills: skills\n  hooks: hooks\n  mcps: mcps\n  commands: commands\ntargets: [claude]\n")

	stdout := captureStdout(t, func() {
		root := NewRootCmd("test")
		root.SetArgs([]string{"import", "claude", "--dry-run"})
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})

	if !strings.Contains(stdout, "would write ") {
		t.Errorf("expected 'would write <path>' lines, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "dry-run:") {
		t.Errorf("expected dry-run summary line, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "--- ") {
		t.Errorf("dry-run should not dump file contents anymore, got:\n%s", stdout)
	}
	// The rule body must not leak into stdout.
	if strings.Contains(stdout, "body") {
		t.Errorf("file content leaked into dry-run output:\n%s", stdout)
	}
	// No file should actually be written.
	if _, err := os.Stat(filepath.Join(dir, "rules", "sliced.md")); !os.IsNotExist(err) {
		t.Errorf("dry-run wrote a file: %v", err)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	done := make(chan struct{})
	var buf bytes.Buffer
	go func() {
		_, _ = io.Copy(&buf, r)
		close(done)
	}()
	fn()
	_ = w.Close()
	<-done
	return buf.String()
}
