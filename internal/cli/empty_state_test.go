package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// emptyProjectDir scaffolds a project with a config but zero specs in any
// source directory. This is the state right after `agnostic-ai init`.
func emptyProjectDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestList_EmptyHintsToStderr(t *testing.T) {
	dir := emptyProjectDir(t)
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"list"})
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errBuf)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 {
		t.Errorf("list stdout should be empty when no specs, got %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "no specs found") {
		t.Errorf("expected hint on stderr, got %q", errBuf.String())
	}
}

func TestValidate_EmptyHintsToStderr(t *testing.T) {
	dir := emptyProjectDir(t)
	testutil.Chdir(t, dir)

	root := NewRootCmd("test")
	root.SetArgs([]string{"validate"})
	out := &bytes.Buffer{}
	errBuf := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(errBuf)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "loaded 0 entries") {
		t.Errorf("expected entry count on stdout, got %q", out.String())
	}
	if !strings.Contains(errBuf.String(), "no specs found") {
		t.Errorf("expected hint on stderr, got %q", errBuf.String())
	}
}
