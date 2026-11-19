package emit

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFileAt_RecreatesAMissingParent(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "gone", "nested", "agent.md")
	if err := writeFileAt(path, "generated", 0o644, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "generated" {
		t.Errorf("content = %q, want %q", got, "generated")
	}
}

func TestWriteFileAt_EnforcesTheModeAfterRecreatingTheParent(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("Windows keeps no Unix permission bits")
	}
	path := filepath.Join(t.TempDir(), "gone", "hook.sh")
	const mode os.FileMode = 0o777
	if err := writeFileAt(path, "#!/bin/sh\n", mode, true); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != mode {
		t.Errorf("mode = %v, want %v", got, mode)
	}
}

func TestWriteFileAt_RefusesAPathThatIsAFolder(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	path := filepath.Join(parent, "agents")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAt(path, "generated", 0o644, false); err == nil {
		t.Fatal("writing over a folder succeeded")
	}
	if names := entryNames(t, parent); len(names) != 1 || names[0] != "agents" {
		t.Errorf("parent holds %v after the failed write, want only agents", names)
	}
	if names := entryNames(t, path); len(names) != 0 {
		t.Errorf("folder holds %v after the failed write, want nothing", names)
	}
}

func entryNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestWriteFileAt_ReturnsTheWriteErrorWhenTheParentCannotBeRecreated(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs a folder the user cannot write into")
	}
	locked := filepath.Join(t.TempDir(), "locked")
	if err := os.Mkdir(locked, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	path := filepath.Join(locked, "gone", "agent.md")
	err := writeFileAt(path, "generated", 0o644, false)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want the first write's not-exist error", err)
	}
	if names := entryNames(t, locked); len(names) != 0 {
		t.Errorf("locked folder holds %v, want nothing", names)
	}
}
