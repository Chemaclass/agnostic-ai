package emit

import (
	"os"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestCurrentContent_ByMode(t *testing.T) {
	testutil.TempCwd(t)
	if err := os.WriteFile("a.json", []byte("disk"), 0o644); err != nil {
		t.Fatal(err)
	}

	sess := NewSession()
	if got, ok := sess.CurrentContent("a.json"); !ok || string(got) != "disk" {
		t.Errorf("sync reads the file: %q, %t", got, ok)
	}
	if _, ok := sess.CurrentContent("missing.json"); ok {
		t.Error("a missing file reports false")
	}

	sess.StartCapture()
	if got, ok := sess.CurrentContent("a.json"); !ok || string(got) != "disk" {
		t.Errorf("a check with no write yet reads the file: %q, %t", got, ok)
	}
	if err := sess.WriteFile("a.json", "first", false); err != nil {
		t.Fatal(err)
	}
	if err := sess.WriteFile("a.json", "second", false); err != nil {
		t.Fatal(err)
	}
	if got, ok := sess.CurrentContent("a.json"); !ok || string(got) != "second\n" {
		t.Errorf("a check reads its last captured write: %q, %t", got, ok)
	}
	sess.StopCapture()
	if data, _ := os.ReadFile("a.json"); string(data) != "disk" {
		t.Errorf("capture touched the file: %q", data)
	}
}
