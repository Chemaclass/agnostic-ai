package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// windowsCheckout commits a synced claude project in a repository whose
// Git settings convert line endings and write links as plain files, as
// Git for Windows does by default. Git honors these settings on every OS.
func windowsCheckout(t *testing.T, settings map[string]string, attributes string) string {
	t.Helper()
	dir := setupFixture(t)
	isolateGit(t)
	git(t, dir, "init", "-q")
	git(t, dir, "config", "core.symlinks", "false")
	for k, v := range settings {
		git(t, dir, "config", k, v)
	}
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), againstConfig("instructions"))
	if attributes != "" {
		mustWriteFile(t, filepath.Join(dir, ".gitattributes"), attributes)
	}
	testutil.Chdir(t, dir)
	silence(t)
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	return dir
}

// checkWorkingTree runs `sync --check` and returns stdout, stderr and the
// error.
func checkWorkingTree(t *testing.T, extra ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	log := captureLogOut(t)
	root := NewRootCmd("test")
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(append([]string{"sync", "--check"}, extra...))
	err := root.Execute()
	return log.String() + stdout.String(), stderr.String(), err
}

func TestSyncCheckAgainst_PassesOnACleanCommitWhenGitConvertsLineEndings(t *testing.T) {
	for name, tc := range map[string]struct {
		settings   map[string]string
		attributes string
	}{
		"core.autocrlf": {settings: map[string]string{"core.autocrlf": "true"}},
		"core.eol":      {settings: map[string]string{"core.autocrlf": "false", "core.eol": "crlf"}, attributes: "* text=auto\n"},
		"eol attribute": {settings: map[string]string{"core.autocrlf": "true"}, attributes: ".agnostic-ai/rules/*.md text eol=crlf\n"},
	} {
		t.Run(name, func(t *testing.T) {
			windowsCheckout(t, tc.settings, tc.attributes)
			for _, ref := range []string{"HEAD", "index"} {
				if stdout, stderr, err := checkAgainst(t, ref); err != nil {
					t.Errorf("--against %s on a clean commit: %v\n%s\n%s", ref, err, stdout, stderr)
				}
			}
		})
	}
}

func TestSyncCheck_NamesLineEndingsWhenACheckoutConvertedThem(t *testing.T) {
	dir := windowsCheckout(t, map[string]string{"core.autocrlf": "true"}, "")
	if err := os.Remove(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "checkout", "--", "CLAUDE.md")
	data, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil || !strings.Contains(string(data), "\r\n") {
		t.Fatalf("fixture checkout kept LF: %v", err)
	}
	for _, format := range []string{"human", "github"} {
		t.Run(format, func(t *testing.T) {
			stdout, stderr, err := checkWorkingTree(t, "--format", format)
			if err == nil {
				t.Fatal("bytes that differ by line endings are still drift")
			}
			out := stdout + stderr
			for _, want := range []string{"CLAUDE.md", "line endings", "core.autocrlf false", ".gitattributes"} {
				if !strings.Contains(out, want) {
					t.Errorf("report lacks %q:\n%s", want, out)
				}
			}
			if strings.Contains(out, "edited") {
				t.Errorf("a converted checkout is not a hand edit:\n%s", out)
			}
		})
	}
}

func TestSyncCheck_NamesCoreSymlinksForASpecLinkCheckedOutAsAPlainFile(t *testing.T) {
	dir := windowsCheckout(t, map[string]string{"core.autocrlf": "false", "core.symlinks": "true"}, "")
	entry := filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md")
	data, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, "shared", "instructions.md"), string(data)+"\nShared line.\n")
	if err := os.Remove(entry); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "shared", "instructions.md"), entry); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := runSyncArgs(t); err != nil {
		t.Fatalf("sync: %v", err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "linked instructions")
	git(t, dir, "config", "core.symlinks", "false")
	if err := os.Remove(entry); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "checkout", "--", ".agnostic-ai/AGNOSTIC_AI.md")
	if info, err := os.Lstat(entry); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("fixture checkout kept the link: %v", err)
	}

	link := ".agnostic-ai/AGNOSTIC_AI.md"
	check := map[string]func() (string, string, error){
		"working tree": func() (string, string, error) { return checkWorkingTree(t) },
		"HEAD":         func() (string, string, error) { return checkAgainst(t, "HEAD") },
	}
	for name, run := range check {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, err := run()
			if err == nil {
				t.Fatal("a spec link read as its link text must fail")
			}
			out := stdout + stderr + err.Error()
			for _, want := range []string{link, "plain file", "core.symlinks", "git config --local core.symlinks true"} {
				if !strings.Contains(out, want) {
					t.Errorf("report lacks %q:\n%s", want, out)
				}
			}
		})
	}
}
