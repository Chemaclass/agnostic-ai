package atomicfile

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWrite_ReplacesFileWithoutTruncatingTheOldOne(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ignore")
	if err := os.WriteFile(path, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	held := filepath.Join(dir, "held")
	if err := os.Link(path, held); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	if err := Write(path, []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != "new\n" {
		t.Errorf("path = %q", got)
	}
	if got, _ := os.ReadFile(held); string(got) != "old\n" {
		t.Errorf("old file was rewritten in place: %q", got)
	}
}

func TestWrite_KeepsExistingModeAndUsesDefaultForNewFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits differ on Windows")
	}
	dir := t.TempDir()
	existing := filepath.Join(dir, "existing")
	if err := os.WriteFile(existing, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(existing, []byte("b"), 0o644); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(dir, "fresh")
	if err := Write(fresh, []byte("c"), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]os.FileMode{existing: 0o600, fresh: 0o644} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s mode = %v, want %v", filepath.Base(path), info.Mode().Perm(), want)
		}
	}
}

func TestWrite_FollowsSymlinkAndKeepsIt(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real")
	link := filepath.Join(dir, "link")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := Write(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was replaced: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "new" {
		t.Errorf("target = %q", got)
	}
}

func TestWrite_FailureLeavesOldFileAndNoTemp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ignore")
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new"), 0o644); err == nil {
		t.Fatal("want an error replacing a directory")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("leftover files: %v", entries)
	}
}

func TestWrite_CreatesTargetOfDanglingSymlink(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "link")
	if err := os.Symlink("real", link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := Write(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was replaced: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "real")); string(got) != "new" {
		t.Errorf("target = %q", got)
	}
}

func TestWrite_FollowsChainOfDanglingSymlinks(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("b", filepath.Join(dir, "a")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink("a", link); err != nil {
		t.Fatal(err)
	}
	if err := Write(link, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(filepath.Join(dir, "a")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("middle symlink was replaced: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "b")); string(got) != "new" {
		t.Errorf("target = %q", got)
	}
}

func TestWrite_RefusesReadOnlyFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits differ on Windows")
	}
	path := filepath.Join(t.TempDir(), "ignore")
	if err := os.WriteFile(path, []byte("old"), 0o444); err != nil {
		t.Fatal(err)
	}
	if err := Write(path, []byte("new"), 0o644); err == nil {
		t.Fatal("want a permission error")
	}
	if got, _ := os.ReadFile(path); string(got) != "old" {
		t.Errorf("file = %q", got)
	}
}

func linkedFolders(t *testing.T) (root, outside string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(base, "project")
	if err := os.MkdirAll(filepath.Join(root, "a", "b", "c"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.FromSlash("a/b/c"), filepath.Join(root, "sub")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	return root, base
}

func TestWrite_DotDotInLinkAppliesAfterLinkedFolder(t *testing.T) {
	root, outside := linkedFolders(t)
	// sub is a/b/c, so sub/../../real is a/real.
	inside := filepath.Join(root, "a", "real")
	if err := os.WriteFile(inside, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.FromSlash("sub/../../real"), filepath.Join(root, "ignore")); err != nil {
		t.Fatal(err)
	}
	if err := Write(filepath.Join(root, "ignore"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(inside); string(got) != "new" {
		t.Errorf("target = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(outside, "real")); err == nil {
		t.Error("wrote outside the project")
	}
}

func TestWrite_DotDotInDanglingLinkAppliesAfterLinkedFolder(t *testing.T) {
	root, outside := linkedFolders(t)
	if err := os.Symlink(filepath.FromSlash("sub/../../../real"), filepath.Join(root, "ignore")); err != nil {
		t.Fatal(err)
	}
	if err := Write(filepath.Join(root, "ignore"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(root, "real")); string(got) != "new" {
		t.Errorf("target = %q", got)
	}
	if _, err := os.Lstat(filepath.Join(outside, "real")); err == nil {
		t.Error("wrote outside the project")
	}
}
