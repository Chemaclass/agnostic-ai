package mailbox

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReadJSON_OpenFileDoesNotBlockAReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.json")
	if err := writeJSON(path, []auditRecord{}); err != nil {
		t.Fatal(err)
	}
	file, err := openShared(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if err := writeJSON(path, []auditRecord{{Guard: "format"}}); err != nil {
		t.Fatalf("replace while a reader holds the file: %v", err)
	}
	var audit []auditRecord
	if err := readJSON(path, &audit); err != nil || len(audit) != 1 {
		t.Fatalf("audit = %v, err = %v", audit, err)
	}
}

func TestStats_DoesNotBlockAWriterHoldingTheLock(t *testing.T) {
	s := statsFixture(t)
	entries, err := s.threadEntries()
	if err != nil || len(entries) == 0 {
		t.Fatalf("entries = %v, err = %v", entries, err)
	}
	id := strings.TrimSuffix(entries[0].Name(), ".json")
	path := s.threadPath(id)
	file, err := openShared(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	thread, err := s.readThread(id)
	if err != nil {
		t.Fatal(err)
	}
	release, err := s.lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := s.writeThread(thread); err != nil {
		t.Fatalf("write while stats reads the thread: %v", err)
	}
	if _, err := s.Stats(context.Background(), time.Time{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
