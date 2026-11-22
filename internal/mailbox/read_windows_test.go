package mailbox

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadJSON_OpenFileDoesNotBlockAReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.json")
	if err := writeJSON(path, []auditRecord{}); err != nil {
		t.Fatal(err)
	}

	plain, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	blocked := writeJSON(path, []auditRecord{{Guard: "format"}})
	_ = plain.Close()
	if blocked == nil {
		t.Fatal("control: a replace over a file held by os.Open succeeded, so this test proves nothing")
	}

	shared, err := openShared(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = shared.Close() }()
	if err := writeJSON(path, []auditRecord{{Guard: "format"}}); err != nil {
		t.Fatalf("replace while a reader holds the file: %v", err)
	}
	var audit []auditRecord
	if err := readJSON(path, &audit); err != nil || len(audit) != 1 {
		t.Fatalf("audit = %v, err = %v", audit, err)
	}
}
