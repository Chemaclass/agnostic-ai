package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

type reviewRefsReply struct {
	Worktree    string   `json:"worktree"`
	Commit      string   `json:"commit"`
	Base        string   `json:"base"`
	Branch      string   `json:"branch"`
	Paths       []string `json:"paths"`
	Uncommitted int      `json:"uncommitted"`
}

func makeReviewRepo(t *testing.T, dir string) (base string, commit string) {
	t.Helper()
	isolateGit(t)
	gitInit(t, dir)
	git(t, dir, "config", "user.name", "Mailbox Test")
	git(t, dir, "config", "user.email", "mailbox@example.test")
	writeFile(t, filepath.Join(dir, "src", "parser.go"), "original\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	base = git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "update-ref", "refs/remotes/origin/main", base)
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	writeFile(t, filepath.Join(dir, "src", "parser.go"), "changed\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "update")
	commit = git(t, dir, "rev-parse", "HEAD")
	return base, commit
}

func parseReviewRefsReply(t *testing.T, raw []byte) reviewRefsReply {
	t.Helper()
	var refs reviewRefsReply
	if err := json.Unmarshal(raw, &refs); err != nil {
		t.Fatalf("review refs = %s: %v", raw, err)
	}
	return refs
}

func TestMailboxRefsCommand_UsesTheCurrentWorktreeAndOptionalCommit(t *testing.T) {
	dir := realDir(t)
	base, commit := makeReviewRepo(t, dir)
	testutil.Chdir(t, filepath.Join(dir, "src"))
	out, err := runRoot(t, "mailbox", "refs")
	if err != nil {
		t.Fatal(err)
	}
	refs := parseReviewRefsReply(t, []byte(out))
	if !sameRealDir(dir, refs.Worktree) || refs.Base != base || refs.Commit != commit ||
		len(refs.Paths) != 1 || refs.Paths[0] != "src/parser.go" {
		t.Errorf("review refs = %#v", refs)
	}
	out, err = runRoot(t, "mailbox", "refs", "--base", base, "--commit", base)
	if err != nil {
		t.Fatal(err)
	}
	refs = parseReviewRefsReply(t, []byte(out))
	if refs.Commit != base || refs.Base != base || len(refs.Paths) != 0 {
		t.Errorf("selected earlier commit = %#v", refs)
	}
}

func TestMailboxRefsCommand_ReportsWhenCurrentFolderIsOutsideGit(t *testing.T) {
	dir := realDir(t)
	testutil.Chdir(t, dir)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	out, err := runRoot(t, "mailbox", "refs")
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "worktree") {
		t.Errorf("mailbox refs outside Git = %q, %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mailbox")); !os.IsNotExist(err) {
		t.Errorf("read-only command created a mailbox: %v", err)
	}
}
