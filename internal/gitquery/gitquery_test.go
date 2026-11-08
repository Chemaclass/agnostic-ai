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

func TestIgnoreMatches_MatchesOnlyThePatternsGiven(t *testing.T) {
	Reset()
	got, err := IgnoreMatches([]byte("/out/*\n!out/keep.md\n*.log\n"), []string{"out/a.md", "out/keep.md", "src/a.md", "src/x.log"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["out/a.md"]; !ok || len(got) != 2 {
		t.Errorf("IgnoreMatches = %v", got)
	}
	if _, ok := got["src/x.log"]; !ok {
		t.Errorf("IgnoreMatches = %v", got)
	}
	if got, err := IgnoreMatches(nil, []string{"a.md"}); err != nil || len(got) != 0 {
		t.Errorf("no patterns: %v, %v", got, err)
	}
}

func TestQueriesIgnoreHookRepositoryEnvironment(t *testing.T) {
	Reset()
	repo := t.TempDir()
	foreign := t.TempDir()
	git(t, repo, "init", "-q")
	git(t, foreign, "init", "-q")
	must(t, os.Mkdir(filepath.Join(repo, "sub"), 0o755))
	must(t, os.WriteFile(filepath.Join(repo, "sub", "tracked.txt"), []byte("x"), 0o644))
	git(t, repo, "add", "sub/tracked.txt")
	must(t, os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("ignored.local\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(foreign, ".git", "info", "exclude"), []byte("*.md\n"), 0o644))
	t.Setenv("GIT_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_WORK_TREE", foreign)
	t.Setenv("GIT_COMMON_DIR", filepath.Join(foreign, ".git"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(foreign, ".git", "index"))
	layout, err := LayoutAtFresh(filepath.Join(repo, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	actualDir, err := os.Stat(layout.GitDir)
	if err != nil {
		t.Fatal(err)
	}
	expectedDir, err := os.Stat(filepath.Join(repo, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	if layout.Prefix != "sub/" || !os.SameFile(actualDir, expectedDir) {
		t.Errorf("layout = %+v", layout)
	}
	tracked, err := Tracked(repo, "sub/tracked.txt")
	if err != nil || !tracked {
		t.Errorf("tracked = %v, %v", tracked, err)
	}
	if !Ignored(repo, "ignored.local") {
		t.Error("repository ignore not applied")
	}
	matched, err := IgnoreMatches([]byte("foo\n"), []string{"foo", "a.md"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := matched["foo"]; !ok || len(matched) != 1 {
		t.Errorf("matched = %v", matched)
	}
}

func TestTrackedAndIgnored_HandlePathsStartingWithDash(t *testing.T) {
	Reset()
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	name := "--tracked.txt"
	must(t, os.WriteFile(filepath.Join(dir, name), []byte("tracked"), 0o644))
	git(t, dir, "add", "--", name)
	tracked, err := Tracked(dir, name)
	if err != nil || !tracked {
		t.Errorf("tracked = %v, %v", tracked, err)
	}
	must(t, os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("--ignored.txt\n"), 0o644))
	if !Ignored(dir, "--ignored.txt") {
		t.Error("ignored path starting with dash was not matched")
	}
}

func TestOriginURL_ReadsTheOriginRemoteOrNothing(t *testing.T) {
	if got := OriginURL(t.TempDir()); got != "" {
		t.Errorf("outside a repository OriginURL = %q, want empty", got)
	}
	dir := t.TempDir()
	git(t, dir, "init", "-q")
	if got := OriginURL(dir); got != "" {
		t.Errorf("without an origin OriginURL = %q, want empty", got)
	}
	git(t, dir, "remote", "add", "origin", "git@github.com:octo/hello.git")
	must(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	if got := OriginURL(filepath.Join(dir, "sub")); got != "git@github.com:octo/hello.git" {
		t.Errorf("OriginURL = %q, want the origin remote", got)
	}
}

func TestIgnoreMatches_NamesThePatternAndLine(t *testing.T) {
	Reset()
	got, err := IgnoreMatches([]byte("# out\n/out/*\n!out/keep.md\n*.log\n"), []string{"out/a.md", "out/keep.md", "src/a.md", "src/x.log"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]IgnoreMatch{"out/a.md": {Line: 2, Pattern: "/out/*"}, "src/x.log": {Line: 4, Pattern: "*.log"}}
	if len(got) != len(want) {
		t.Errorf("IgnoreMatches = %v, want %v", got, want)
	}
	for path, m := range want {
		if got[path] != m {
			t.Errorf("%s: got %+v, want %+v", path, got[path], m)
		}
	}
}

// Git reports the last matching line, so a negation is skipped only when
// it is the deciding match.
func TestIgnoreMatches_ReportsTheDecidingPattern(t *testing.T) {
	Reset()
	for name, tc := range map[string]struct {
		patterns string
		want     IgnoreMatch
	}{
		"re-ignored after a negation":       {"/out/*\n!out/keep.md\n*.md\n", IgnoreMatch{Line: 3, Pattern: "*.md"}},
		"negation under an ignored folder":  {"out/\n!out/keep.md\n", IgnoreMatch{Line: 1, Pattern: "out/"}},
		"escaped bang is a literal pattern": {"\\!keep.md\nout/keep.md\n", IgnoreMatch{Line: 2, Pattern: "out/keep.md"}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := IgnoreMatches([]byte(tc.patterns), []string{"out/keep.md"})
			if err != nil {
				t.Fatal(err)
			}
			if got["out/keep.md"] != tc.want {
				t.Errorf("got %+v, want %+v", got["out/keep.md"], tc.want)
			}
		})
	}
}
