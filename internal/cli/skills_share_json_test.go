package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncJSON_SharedSkillsReportsOnlyChangedLinks(t *testing.T) {
	dir := setupSharedSkillsFixture(t, "version: 1\ntargets: [claude, codex, cursor]\nsync:\n  shared-skills: true\n")
	probe := filepath.Join(dir, "link-probe")
	if err := os.Symlink(".agnostic-ai", probe); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := os.Remove(probe); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, dir)
	silence(t)
	captureLogOut(t)
	syncJSON := func() jsonOutput {
		t.Helper()
		var stdout, stderr bytes.Buffer
		root := NewRootCmd("test")
		root.SetOut(&stdout)
		root.SetErr(&stderr)
		root.SetArgs([]string{"sync", "--all", "--json"})
		if err := root.Execute(); err != nil {
			t.Fatalf("sync --json: %v\n%s", err, stderr.String())
		}
		var out jsonOutput
		if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
			t.Fatalf("decode sync JSON: %v\n%s", err, stdout.String())
		}
		return out
	}
	first := syncJSON()
	links := []string{filepath.ToSlash(filepath.Join(".claude", "skills", "greet")), filepath.ToSlash(filepath.Join(".cursor", "skills", "greet"))}
	for _, link := range links {
		if !slices.Contains(first.Writes, fileRecord{Target: "agnostic-ai", Path: link, Action: "link"}) {
			t.Errorf("first sync writes omit new link %s: %+v", link, first.Writes)
		}
	}
	unchanged := syncJSON()
	for _, link := range links {
		for _, write := range unchanged.Writes {
			if write.Path == link {
				t.Errorf("unchanged sync reports link %s as a write: %+v", link, write)
			}
		}
		if !slices.Contains(unchanged.Skipped, fileRecord{Target: "agnostic-ai", Path: link, Action: "skip"}) {
			t.Errorf("unchanged sync skips omit link %s: %+v", link, unchanged.Skipped)
		}
	}
	link := links[0]
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, ".agents", "skills", "greet"), link); err != nil {
		t.Fatal(err)
	}
	retargeted := syncJSON()
	if !slices.Contains(retargeted.Writes, fileRecord{Target: "agnostic-ai", Path: link, Action: "link"}) {
		t.Errorf("retargeted link missing from writes: %+v", retargeted.Writes)
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("shared-link ledger must remain clean: %v", err)
	}
}
