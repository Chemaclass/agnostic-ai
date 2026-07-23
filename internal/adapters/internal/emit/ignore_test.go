package emit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestIgnoreBody_ConcatenatesTrimmed(t *testing.T) {
	t.Parallel()
	got := IgnoreBody([]spec.Entry{
		{Body: "*.env\n"},
		{Body: "  \n"}, // blank-only: skipped
		{Body: "secrets/"},
	})
	want := "*.env\n\nsecrets/"
	if got != want {
		t.Errorf("IgnoreBody = %q, want %q", got, want)
	}
}

func TestWriteIgnoreFile_WritesHeaderAndPatterns(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursorignore")
	if err := sess.WriteIgnoreFile([]spec.Entry{{Body: "*.env"}}, path, false); err != nil {
		t.Fatalf("write: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	body := string(data)
	if !strings.Contains(body, "*.env") {
		t.Errorf("missing pattern: %s", body)
	}
	if !strings.HasPrefix(body, "#") {
		t.Errorf("expected shell-style (#) provenance header: %s", body)
	}
}

func TestWriteIgnoreFile_NoOpWhenEmpty(t *testing.T) {
	t.Parallel()
	sess := NewSession()
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursorignore")
	if err := sess.WriteIgnoreFile(nil, path, false); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected no file written for empty ignore set, got err=%v", err)
	}
}
