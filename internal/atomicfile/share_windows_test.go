package atomicfile

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestReplace_SucceedsWhileASharedReaderHoldsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.json")
	if err := Replace(path, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	plain, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	blocked := Replace(path, []byte("new"), 0o600)
	_ = plain.Close()
	if blocked == nil {
		t.Fatal("control: a replace over a file held by os.Open succeeded, so this test proves nothing")
	}

	shared, err := OpenShared(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shared.Close() }()
	if err := Replace(path, []byte("new"), 0o600); err != nil {
		t.Fatalf("replace while a shared reader holds the file: %v", err)
	}
	if err := Write(path, []byte("newer"), 0o600); err != nil {
		t.Fatalf("write while a shared reader holds the file: %v", err)
	}
	if got, _ := io.ReadAll(shared); string(got) != "old" {
		t.Errorf("open reader = %q, want the old content", got)
	}
	if got, _ := os.ReadFile(path); string(got) != "newer" {
		t.Errorf("path = %q", got)
	}
}
