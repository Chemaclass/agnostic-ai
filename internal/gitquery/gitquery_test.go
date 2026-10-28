package gitquery

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func TestLayoutAt_WorkTreeAndSubfolder(t *testing.T) {
	Reset()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	must(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	l, err := LayoutAt(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if !l.HasToplevel || l.Prefix != "sub/" || filepath.Base(l.CommonDir) != ".git" {
		t.Errorf("layout = %+v", l)
	}
}

func TestLayoutAt_BareRepositoryHasNoToplevel(t *testing.T) {
	Reset()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "--bare")
	l, err := LayoutAt(dir)
	if err != nil {
		t.Fatal(err)
	}
	if l.HasToplevel || l.CommonDir == "" {
		t.Errorf("layout = %+v", l)
	}
	if _, err := Toplevel(dir); err == nil {
		t.Error("Toplevel of a bare repository succeeded")
	}
}

func TestLayoutAt_OutsideGitFails(t *testing.T) {
	Reset()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	if _, err := LayoutAt(t.TempDir()); err == nil {
		t.Error("LayoutAt outside Git succeeded")
	}
}

func TestTracked_AnswerHoldsUntilReset(t *testing.T) {
	Reset()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	must(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0o644))
	if got, _ := Tracked(dir, "a.txt"); got {
		t.Fatal("untracked file reported tracked")
	}
	git(t, dir, "add", "a.txt")
	if got, _ := Tracked(dir, "a.txt"); got {
		t.Error("cached answer changed before Reset")
	}
	Reset()
	if got, err := Tracked(dir, "a.txt"); err != nil || !got {
		t.Errorf("after Reset: tracked=%v err=%v", got, err)
	}
}

func TestIgnored(t *testing.T) {
	Reset()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	must(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("out/\n"), 0o644))
	if !Ignored(dir, "out/") || Ignored(dir, "src/") {
		t.Error("Ignored disagrees with .gitignore")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestIgnoredBy_MatchesOnlyThePatternsGiven(t *testing.T) {
	Reset()
	got, err := IgnoredBy([]byte("/out/*\n!out/keep.md\n*.log\n"), []string{"out/a.md", "out/keep.md", "src/a.md", "src/x.log"})
	if err != nil {
		t.Fatal(err)
	}
	if !got["out/a.md"] || got["out/keep.md"] || got["src/a.md"] || !got["src/x.log"] {
		t.Errorf("IgnoredBy = %v", got)
	}
	if got, err := IgnoredBy(nil, []string{"a.md"}); err != nil || len(got) != 0 {
		t.Errorf("no patterns: %v, %v", got, err)
	}
}
