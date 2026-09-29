package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncCheckAgainst_ChecksAProjectBelowTheRepositoryRoot(t *testing.T) {
	isolateGit(t)
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	app := filepath.Join(repo, "apps", "web")
	if err := os.MkdirAll(filepath.Join(app, ".agnostic-ai", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, "agnostic-ai.yaml"), []byte(againstConfig("instructions")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app, ".agnostic-ai", "rules", "r1.md"), []byte("---\nname: r1\n---\nrule body"), 0o644); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, app)
	silence(t)
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	git(t, repo, "add", "-A")
	if _, _, err := checkAgainst(t, "index"); err != nil {
		t.Fatalf("a synced index should pass: %v", err)
	}
	editRuleSpec(t, app, "Staged rule.\n")
	git(t, repo, "add", "-A")

	stdout, _, err := checkAgainst(t, "index")

	if err == nil || !strings.Contains(stdout, ".claude/rules/r1.md") {
		t.Errorf("a stale staged rule output should fail and be named, got err=%v\n%s", err, stdout)
	}
	if wd, _ := os.Getwd(); !strings.HasSuffix(filepath.ToSlash(wd), "apps/web") {
		t.Errorf("the check should leave the working directory at the project, got %s", wd)
	}
}
