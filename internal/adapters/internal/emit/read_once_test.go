package emit

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// countReads counts reads of path through readFile for the test.
func countReads(t *testing.T, path string) *int {
	t.Helper()
	reads := 0
	prev := readFile
	readFile = func(name string) ([]byte, error) {
		if name == path {
			reads++
		}
		return prev(name)
	}
	t.Cleanup(func() { readFile = prev })
	return &reads
}

func TestWriteFile_ReadsAnUnchangedFileOnce(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	path := "out.md"
	body := WithHeader("body\n", FormatMarkdown)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	reads := countReads(t, path)

	sess := NewSession()
	sess.BackUpEditsSince(map[string]string{path: ContentSum(body)})
	sess.StartDetailedRecording()
	if err := sess.WriteFile(path, body, false); err != nil {
		t.Fatal(err)
	}

	if *reads != 1 {
		t.Errorf("read %s %d times, want 1", path, *reads)
	}
	if got := sess.StopDetailedRecording(); len(got) != 1 || got[0].Action != "skip" {
		t.Errorf("recorded %v, want one skip", got)
	}
}

func TestWriteFile_ReadsOnceForRollbackAndBackup(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	path := "out.md"
	if err := os.WriteFile(path, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	reads := countReads(t, path)

	sess := NewSession()
	sess.StartTransaction()
	sess.SetBackup(true)
	if err := sess.WriteFile(path, "new\n", false); err != nil {
		t.Fatal(err)
	}

	if *reads != 1 {
		t.Errorf("read %s %d times, want 1", path, *reads)
	}
	if got, err := os.ReadFile(path + ".bak"); err != nil || string(got) != "old\n" {
		t.Errorf(".bak = %q, %v; want the old bytes", got, err)
	}
}

// countParentMakes counts the folders writes create for the test.
func countParentMakes(t *testing.T) *int {
	t.Helper()
	calls := 0
	prev := makeParent
	makeParent = func(path string) error {
		calls++
		return prev(path)
	}
	t.Cleanup(func() { makeParent = prev })
	return &calls
}

func TestWriteFile_UnchangedFileMakesNoFolder(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	path := filepath.Join("dir", "out.md")
	body := WithHeader("body\n", FormatMarkdown)
	if err := os.MkdirAll("dir", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	makes := countParentMakes(t)

	sess := NewSession()
	sess.BackUpEditsSince(map[string]string{path: ContentSum(body)})
	sess.StartTransaction()
	sess.StartDetailedRecording()
	if err := sess.WriteFile(path, body, false); err != nil {
		t.Fatal(err)
	}

	if *makes != 0 {
		t.Errorf("made the folder %d times for an unchanged file, want 0", *makes)
	}
	if got := sess.StopDetailedRecording(); len(got) != 1 || got[0].Action != "skip" {
		t.Errorf("recorded %v, want one skip", got)
	}
}

func TestWriteFile_NewFileMakesItsFolder(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	path := filepath.Join("a", "b", "out.md")

	for _, detailed := range []bool{false, true} {
		sess := NewSession()
		sess.StartTransaction()
		if detailed {
			sess.StartDetailedRecording()
		}
		if err := sess.WriteFile(path, "body\n", false); err != nil {
			t.Fatalf("detailed=%v: %v", detailed, err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != "body\n" {
			t.Fatalf("detailed=%v: read back %q, %v", detailed, got, err)
		}
		if err := os.RemoveAll("a"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestWriteFile_ParentIsAFileNamesTheFolder(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	if err := os.WriteFile("a", []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess := NewSession()
	sess.StartDetailedRecording()
	err := sess.WriteFile(filepath.Join("a", "out.md"), "body\n", false)

	if err == nil || !strings.HasPrefix(err.Error(), "mkdir a:") {
		t.Errorf("err = %v, want it to name the folder it could not make", err)
	}
}

// With enforceMode, a write through a link compares the mode of the file
// the link points to, as os.Stat did.
func TestWriteFile_ThroughALinkComparesTheTargetMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	testutil.Chdir(t, t.TempDir())
	body := "#!/bin/sh\n"
	// 0700 differs from the mode a link itself reports (0755 on macOS,
	// 0777 on Linux).
	if err := os.WriteFile("target.sh", []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("target.sh", "link.sh"); err != nil {
		t.Fatal(err)
	}

	sess := NewSession()
	sess.StartDetailedRecording()
	if err := sess.writeFileWithMode("link.sh", body, 0o700, true, false); err != nil {
		t.Fatal(err)
	}

	if got := sess.StopDetailedRecording(); len(got) != 1 || got[0].Action != "skip" {
		t.Errorf("recorded %v, want a skip: the target already has mode 0700", got)
	}
}
