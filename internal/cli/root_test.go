package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestRoot_VersionFlag(t *testing.T) {
	root := NewRootCmd("9.9.9")
	root.SetArgs([]string{"--version"})
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.Bytes(), []byte("9.9.9")) {
		t.Errorf("expected version 9.9.9 in output, got %s", buf.Bytes())
	}
}

func TestSync_Validate_List_OnFixture(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)

	// suppress noisy stderr/stdout from cobra during this test
	silence(t)

	for _, args := range [][]string{
		{"validate"},
		{"list"},
		{"sync", "-t", "claude"},
	} {
		root := NewRootCmd("test")
		root.SetArgs(args)
		if err := root.Execute(); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Error("expected CLAUDE.md after sync -t claude")
	}
}

func TestSync_DryRunDoesNotWrite(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "claude", "--dry-run"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("dry-run should not have produced CLAUDE.md")
	}
}

func TestSync_UnknownTargetIsSkipped(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "no-such-target"})
	if err := root.Execute(); err != nil {
		t.Errorf("unknown target should not be a fatal error: %v", err)
	}
}

func setupFixture(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "agnostic.config.yaml"),
		[]byte("version: 1\n"), 0o644))
	must(os.MkdirAll(filepath.Join(dir, "rules"), 0o755))
	must(os.WriteFile(filepath.Join(dir, "rules", "r1.md"),
		[]byte("---\nname: r1\n---\nrule body"), 0o644))
	return dir
}

func silence(t *testing.T) {
	t.Helper()
	stdout, stderr := os.Stdout, os.Stderr
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	os.Stderr = devnull
	t.Cleanup(func() {
		os.Stdout = stdout
		os.Stderr = stderr
		_ = devnull.Close()
	})
	_ = io.Discard
}
