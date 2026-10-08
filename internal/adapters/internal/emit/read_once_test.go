package emit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFile_ReadsAnUnchangedFileOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.md")
	body := WithHeader("body\n", FormatMarkdown)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	reads := 0
	prev := readFile
	readFile = func(name string) ([]byte, error) {
		if name == path {
			reads++
		}
		return prev(name)
	}
	t.Cleanup(func() { readFile = prev })

	sess := NewSession()
	sess.BackUpEditsSince(map[string]string{path: ContentSum(body)})
	sess.StartDetailedRecording()
	if err := sess.WriteFile(path, body, false); err != nil {
		t.Fatal(err)
	}

	if reads != 1 {
		t.Errorf("read %s %d times, want 1", path, reads)
	}
	if got := sess.StopDetailedRecording(); len(got) != 1 || got[0].Action != "skip" {
		t.Errorf("recorded %v, want one skip", got)
	}
}
