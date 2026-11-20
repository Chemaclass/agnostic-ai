package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestPacksAdd_SaysReinstalledForAnInstalledPack(t *testing.T) {
	root := testutil.TempCwd(t)
	src := filepath.Join(t.TempDir(), "team")
	mustWrite(t, filepath.Join(src, "rules", "x.md"), "---\nname: x\n---\nbody")

	var out bytes.Buffer
	for range 2 {
		if err := runPacksAdd(root, src, "", &out); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	if got := out.String(); got != "✓ added team\n✓ reinstalled team\n" {
		t.Errorf("add twice printed %q", got)
	}

	out.Reset()
	if err := runPacksList(root, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "@") {
		t.Errorf("a local pack has no Git ref to show: %q", out.String())
	}
}

func TestPacksAdd_AMissingFolderNamesTheErrorOnce(t *testing.T) {
	root := testutil.TempCwd(t)
	err := runPacksAdd(root, "./nope", "", &bytes.Buffer{})
	if err == nil || strings.Count(err.Error(), "stat") != 1 {
		t.Errorf("add ./nope: %v", err)
	}
}
