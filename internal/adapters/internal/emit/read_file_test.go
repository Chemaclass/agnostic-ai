package emit

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestReadFileWaitsForSharedWrite(t *testing.T) {
	t.Run("same path", func(t *testing.T) { testReadFileWaitsForSharedWrite(t, "same") })
	t.Run("relative and absolute paths", func(t *testing.T) { testReadFileWaitsForSharedWrite(t, "relative") })
	t.Run("symlinked directory", func(t *testing.T) { testReadFileWaitsForSharedWrite(t, "symlink") })
	t.Run("case-insensitive directory", func(t *testing.T) { testReadFileWaitsForSharedWrite(t, "case") })
	t.Run("unicode case aliases", func(t *testing.T) { testReadFileWaitsForSharedWrite(t, "unicode-case") })
	t.Run("unicode normalization aliases", func(t *testing.T) { testReadFileWaitsForSharedWrite(t, "unicode-normalization") })
	t.Run("dangling skill symlink", func(t *testing.T) { testReadFileWaitsForSharedWrite(t, "dangling") })
	t.Run("skill symlink to another output", func(t *testing.T) { testReadFileWaitsForSharedWrite(t, "other-output") })
	t.Run("hard-linked output", func(t *testing.T) { testReadFileWaitsForSharedWrite(t, "hardlink") })
	t.Run("hard link created during write", func(t *testing.T) { testReadFileWaitsForSharedWrite(t, "hardlink-new") })
	t.Run("windows extended path", func(t *testing.T) { testReadFileWaitsForSharedWrite(t, "windows-extended") })
}

func testReadFileWaitsForSharedWrite(t *testing.T, alias string) {
	root := t.TempDir()
	if alias == "relative" || alias == "other-output" {
		testutil.Chdir(t, root)
	}
	path := filepath.Join(root, "nested", "SKILL.md")
	writePath := path
	switch alias {
	case "relative":
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		writePath, err = filepath.Rel(cwd, path)
		if err != nil {
			t.Fatal(err)
		}
	case "symlink":
		link := filepath.Join(t.TempDir(), "skills")
		if err := os.Symlink(root, link); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		writePath = filepath.Join(link, "nested", "SKILL.md")
	case "case":
		lowerRoot := filepath.Join(root, "casefolder")
		if err := os.Mkdir(lowerRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		upperRoot := filepath.Join(root, strings.ToUpper(filepath.Base(lowerRoot)))
		original, err := os.Stat(lowerRoot)
		if err != nil {
			t.Fatal(err)
		}
		alternate, err := os.Stat(upperRoot)
		if err != nil || !os.SameFile(original, alternate) {
			t.Skip("filesystem is case-sensitive")
		}
		path = filepath.Join(lowerRoot, "nested", "SKILL.md")
		writePath = filepath.Join(upperRoot, "NESTED", "SKILL.md")
	case "windows-extended":
		if runtime.GOOS != "windows" {
			t.Skip("Windows path syntax")
		}
		if strings.HasPrefix(path, `\\`) {
			writePath = `\\?\UNC\` + strings.TrimPrefix(path, `\\`)
		} else {
			writePath = `\\?\` + path
		}
	case "hardlink", "hardlink-new":
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		writePath = filepath.Join(root, "other.md")
		if alias == "hardlink" {
			if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Link(path, writePath); err != nil {
				t.Skipf("hard links unavailable: %v", err)
			}
		}
	case "dangling", "other-output":
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..", "target.md"), path); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		if alias == "other-output" {
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			writePath, err = filepath.Rel(cwd, filepath.Join(root, "target.md"))
			if err != nil {
				t.Fatal(err)
			}
		}
	case "unicode-case", "unicode-normalization":
		first, second := "\U000003A3", "\U000003C2"
		if alias == "unicode-normalization" {
			first, second = "\U000000E9", "e\U00000301"
		}
		firstDir, secondDir := filepath.Join(root, first), filepath.Join(root, second)
		if err := os.Mkdir(firstDir, 0o755); err != nil {
			t.Fatal(err)
		}
		firstInfo, err := os.Stat(firstDir)
		if err != nil {
			t.Fatal(err)
		}
		secondInfo, err := os.Stat(secondDir)
		if err != nil || !os.SameFile(firstInfo, secondInfo) {
			t.Skip("filesystem does not identify these names")
		}
		path = filepath.Join(firstDir, "nested", "SKILL.md")
		writePath = filepath.Join(secondDir, "nested", "SKILL.md")
	}
	unlockPath := lockPath(writePath)
	ownershipReads.RLock()
	unlock := func() { ownershipReads.RUnlock(); unlockPath() }
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		unlock()
		t.Fatal(err)
	}
	partialPath := path
	if alias == "hardlink-new" {
		partialPath = writePath
	}
	if err := os.WriteFile(partialPath, []byte("---\nname: deploy\n"), 0o644); err != nil {
		unlock()
		t.Fatal(err)
	}
	if alias == "hardlink-new" {
		if err := os.Link(writePath, path); err != nil {
			unlock()
			t.Skipf("hard links unavailable: %v", err)
		}
	}
	type result struct {
		data []byte
		err  error
	}
	started := make(chan struct{})
	read := make(chan result, 1)
	go func() {
		close(started)
		data, err := ReadFile(path)
		read <- result{data, err}
	}()
	<-started
	select {
	case got := <-read:
		unlock()
		t.Fatalf("read during write: %q, %v", got.data, got.err)
	case <-time.After(100 * time.Millisecond):
	}
	const complete = "---\nname: deploy\n---\n<!-- Generated by agnostic-ai -->\n"
	if err := os.WriteFile(path, []byte(complete), 0o644); err != nil {
		unlock()
		t.Fatal(err)
	}
	unlock()
	got := <-read
	if got.err != nil || string(got.data) != complete || !header.Has(string(got.data)) {
		t.Errorf("read after write = %q, %v; want generated skill", got.data, got.err)
	}
}

func TestOutputMutationsWaitForOwnershipRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	sess := NewSession()
	sess.StartTransaction()
	operations := []struct {
		name string
		run  func() error
	}{
		{"write", func() error { return writeFileAt(path, "generated", 0o644, false) }},
		{"backup", func() error { return writeBackup(path+".bak", []byte("old"), 0o644) }},
		{"remove", func() error { _, err := sess.remove(path, "", []byte("generated"), false, false); return err }},
		{"rollback", sess.Rollback},
	}
	for _, op := range operations {
		t.Run(op.name, func(t *testing.T) {
			ownershipReads.Lock()
			started, done := make(chan struct{}), make(chan error, 1)
			go func() { close(started); done <- op.run() }()
			<-started
			select {
			case err := <-done:
				ownershipReads.Unlock()
				t.Fatalf("mutation completed during ownership read: %v", err)
			case <-time.After(100 * time.Millisecond):
			}
			ownershipReads.Unlock()
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("mutation did not finish")
			}
		})
	}
}

func TestMutationsCanRunTogether(t *testing.T) {
	ownershipReads.RLock()
	defer ownershipReads.RUnlock()
	path := filepath.Join(t.TempDir(), "other.md")
	done := make(chan error, 1)
	go func() { done <- writeFileAt(path, "data", 0o644, false) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unrelated mutation waited for another mutation")
	}
}
