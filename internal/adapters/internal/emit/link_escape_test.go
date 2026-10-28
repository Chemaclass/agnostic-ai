package emit

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// linkProject returns a project folder, as the working directory, and a
// folder outside it.
func linkProject(t *testing.T) (project, outside string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("symlinks need privileges on Windows")
	}
	project, outside = t.TempDir(), t.TempDir()
	testutil.Chdir(t, project)
	return project, outside
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
}

func assertEmptyDir(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) > 0 {
		t.Errorf("%s must stay empty, holds %v", dir, entries)
	}
}

func wantLinkRefusal(t *testing.T, err error, link, reason string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), link+" is a link,") || !strings.Contains(err.Error(), reason) {
		t.Errorf("want a refusal naming the link %s (%s), got %v", link, reason, err)
	}
}

func TestWriteFile_RefusesAFolderLinkedOutsideTheProject(t *testing.T) {
	_, outside := linkProject(t)
	symlinkOrSkip(t, outside, ".claude")

	err := NewSession().WriteFile(filepath.Join(".claude", "settings.json"), "{}\n", false)

	wantLinkRefusal(t, err, ".claude", "outside the project")
	assertEmptyDir(t, outside)
}

func TestWriteFile_RefusesADanglingFileLinkOutsideTheProject(t *testing.T) {
	_, outside := linkProject(t)
	symlinkOrSkip(t, filepath.Join(outside, "profile"), "CLAUDE.md")

	err := NewSession().WriteFile("CLAUDE.md", "hi\n", false)

	wantLinkRefusal(t, err, "CLAUDE.md", "outside the project")
	assertEmptyDir(t, outside)
}

func TestWriteExecutableFile_RefusesALinkIntoTheGitFolder(t *testing.T) {
	project, _ := linkProject(t)
	if err := os.MkdirAll(filepath.Join(project, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, filepath.Join("..", ".git", "hooks"), filepath.Join(".claude", "hooks"))

	err := NewSession().WriteExecutableFile(filepath.Join(".claude", "hooks", "post-commit"), "#!/bin/sh\n", false)

	wantLinkRefusal(t, err, filepath.Join(".claude", "hooks"), "Git folder")
	assertEmptyDir(t, filepath.Join(project, ".git", "hooks"))
}

func TestWriteFile_RefusesALinkIntoTheGitCommonDirOfALinkedCheckout(t *testing.T) {
	project, _ := linkProject(t)
	common := filepath.Join(project, "store")
	gitDir := filepath.Join(common, "worktrees", "wt")
	if err := os.MkdirAll(filepath.Join(common, "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(gitDir, "commondir"), "../..\n")
	writeTestFile(t, ".git", "gitdir: "+gitDir+"\n")
	symlinkOrSkip(t, filepath.Join("..", "store", "hooks"), filepath.Join(".claude", "hooks"))

	err := NewSession().WriteFile(filepath.Join(".claude", "hooks", "post-commit"), "#!/bin/sh\n", false)

	wantLinkRefusal(t, err, filepath.Join(".claude", "hooks"), "Git folder")
	assertEmptyDir(t, filepath.Join(common, "hooks"))
}

func TestWriteFile_RefusesAPathInsideTheGitFolder(t *testing.T) {
	project, _ := linkProject(t)
	if err := os.MkdirAll(filepath.Join(project, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}

	err := NewSession().WriteFile(filepath.Join(".git", "hooks", "post-commit"), "#!/bin/sh\n", false)

	if err == nil || !strings.Contains(err.Error(), "Git folder") {
		t.Errorf("want a refusal to write into the Git folder, got %v", err)
	}
	assertEmptyDir(t, filepath.Join(project, ".git", "hooks"))
}

func TestWriteFile_WritesThroughLinksThatStayInsideTheProject(t *testing.T) {
	project, _ := linkProject(t)
	symlinkOrSkip(t, "AGENTS.md", "CLAUDE.md")
	if err := os.MkdirAll(filepath.Join(project, "tools", "claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, filepath.Join("tools", "claude"), ".claude")
	sess := NewSession()

	if err := sess.WriteFile("CLAUDE.md", "entry\n", false); err != nil {
		t.Fatalf("write through the file link: %v", err)
	}
	if err := sess.WriteFile(filepath.Join(".claude", "rules", "a.md"), "rule\n", false); err != nil {
		t.Fatalf("write through the folder link: %v", err)
	}

	if got, _ := os.ReadFile(filepath.Join(project, "AGENTS.md")); string(got) != "entry\n" {
		t.Errorf("AGENTS.md = %q, want the entry written through CLAUDE.md", got)
	}
	if got, _ := os.ReadFile(filepath.Join(project, "tools", "claude", "rules", "a.md")); string(got) != "rule\n" {
		t.Errorf("tools/claude/rules/a.md = %q, want the rule", got)
	}
}

func TestWriteFile_LeavesAbsolutePathsOutsideTheProjectAlone(t *testing.T) {
	_, outside := linkProject(t)
	real := t.TempDir()
	symlinkOrSkip(t, real, filepath.Join(outside, "linked"))

	if err := NewSession().WriteFile(filepath.Join(outside, "linked", "a.md"), "a\n", false); err != nil {
		t.Fatalf("a write outside the project is the caller's choice: %v", err)
	}
	if _, err := os.Stat(filepath.Join(real, "a.md")); err != nil {
		t.Errorf("a.md must land in the linked folder: %v", err)
	}
}

func TestWriteFile_BackupRefusesALinkedBackupOutsideTheProject(t *testing.T) {
	_, outside := linkProject(t)
	writeTestFile(t, "CLAUDE.md", "old\n")
	symlinkOrSkip(t, filepath.Join(outside, "profile"), "CLAUDE.md.bak")
	sess := NewSession()
	sess.SetBackup(true)

	err := sess.WriteFile("CLAUDE.md", "new\n", false)

	wantLinkRefusal(t, err, "CLAUDE.md.bak", "outside the project")
	assertEmptyDir(t, outside)
}

func TestCopyTree_RefusesASkillFolderLinkedOutsideTheProject(t *testing.T) {
	project, outside := linkProject(t)
	src := filepath.Join(project, ".agnostic-ai", "skills", "greet")
	writeTestFile(t, filepath.Join(src, "SKILL.md"), "---\nname: greet\n---\nSay hi.\n")
	writeTestFile(t, filepath.Join(src, "run.sh"), "#!/bin/sh\n")
	symlinkOrSkip(t, outside, filepath.Join(".claude", "skills"))

	err := NewSession().CopyTree(filepath.Join(".agnostic-ai", "skills", "greet"), filepath.Join(".claude", "skills", "greet"), nil, false)

	wantLinkRefusal(t, err, filepath.Join(".claude", "skills"), "outside the project")
	assertEmptyDir(t, outside)
}

func TestRemoveOwned_RefusesAFileUnderAFolderLinkedOutsideTheProject(t *testing.T) {
	_, outside := linkProject(t)
	body := "rule\n"
	writeTestFile(t, filepath.Join(outside, "a.md"), body)
	symlinkOrSkip(t, outside, filepath.Join(".claude", "rules"))

	removed, err := NewSession().RemoveOwned(filepath.Join(".claude", "rules", "a.md"), ContentSum(body), false)

	wantLinkRefusal(t, err, filepath.Join(".claude", "rules"), "outside the project")
	if removed {
		t.Error("removed = true, want false")
	}
	if _, err := os.Stat(filepath.Join(outside, "a.md")); err != nil {
		t.Errorf("the file outside the project must stay: %v", err)
	}
}

func TestRemoveLink_RemovesALinkThatPointsOutsideTheProject(t *testing.T) {
	_, outside := linkProject(t)
	writeTestFile(t, filepath.Join(outside, "a.md"), "keep\n")
	symlinkOrSkip(t, filepath.Join(outside, "a.md"), "a.md")

	removed, err := NewSession().RemoveLink("a.md", false)

	if err != nil || !removed {
		t.Fatalf("RemoveLink = %v, %v; want the link itself removed", removed, err)
	}
	if _, err := os.Lstat("a.md"); !os.IsNotExist(err) {
		t.Errorf("the link must be gone: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "a.md")); err != nil {
		t.Errorf("the link's target must stay: %v", err)
	}
}

func TestWriteExecutableFile_RefusesADanglingLinkThatClimbsFromAnotherLinksTarget(t *testing.T) {
	project, _ := linkProject(t)
	if err := os.MkdirAll(filepath.Join(project, ".git", "refs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".git", "hooks"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, filepath.Join(".git", "refs"), "r")
	symlinkOrSkip(t, "../../r/../hooks/post-commit", filepath.Join(".claude", "hooks", "run.sh"))

	err := NewSession().WriteExecutableFile(filepath.Join(".claude", "hooks", "run.sh"), "#!/bin/sh\n", false)

	wantLinkRefusal(t, err, filepath.Join(".claude", "hooks", "run.sh"), "Git folder")
	assertEmptyDir(t, filepath.Join(project, ".git", "hooks"))
}

func TestWriteFile_RefusesAnUncleanPathThatClimbsFromALinksTarget(t *testing.T) {
	_, outside := linkProject(t)
	if err := os.MkdirAll(filepath.Join(outside, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, filepath.Join(outside, "sub"), "s")
	symlinkOrSkip(t, "s/../pwned.md", "CLAUDE.md")

	err := NewSession().WriteFile("CLAUDE.md", "hi\n", false)

	wantLinkRefusal(t, err, "CLAUDE.md", "outside the project")
	if _, statErr := os.Lstat(filepath.Join(outside, "pwned.md")); !os.IsNotExist(statErr) {
		t.Errorf("pwned.md must not exist outside the project: %v", statErr)
	}
}

func TestWriteFile_RefusesALinkIntoANestedGitFolder(t *testing.T) {
	project, _ := linkProject(t)
	hooks := filepath.Join(project, "vendor", "lib", ".git", "hooks")
	if err := os.MkdirAll(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, filepath.Join("..", "vendor", "lib", ".git", "hooks"), filepath.Join(".claude", "hooks"))

	err := NewSession().WriteFile(filepath.Join(".claude", "hooks", "post-commit"), "#!/bin/sh\n", false)

	wantLinkRefusal(t, err, filepath.Join(".claude", "hooks"), "Git folder")
	assertEmptyDir(t, hooks)
}

func TestCheckProjectPath_AllowsALinkedSpecFolderOnlyForPathsUnderIt(t *testing.T) {
	project, outside := linkProject(t)
	shared := filepath.Join(outside, "shared-specs")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, shared, filepath.Join(project, ".agnostic-ai"))
	ledger := filepath.Join(project, ".agnostic-ai", ".sync-state")

	if err := CheckBookkeepingPath(project, ledger); err == nil {
		t.Error("the ledger in a linked folder that holds no specs must be refused")
	}
	if err := os.WriteFile(filepath.Join(shared, "AGNOSTIC_AI.md"), []byte("Be brief.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CheckBookkeepingPath(project, ledger); err != nil {
		t.Errorf("the ledger in a linked spec folder must stay writable: %v", err)
	}
	if err := CheckProjectPath(project, filepath.Join(project, ".agnostic-ai", "rules", "r1.md")); err == nil {
		t.Error("an output written through the linked spec folder must be refused")
	}
	symlinkOrSkip(t, ".agnostic-ai", filepath.Join(project, ".claude"))
	if err := CheckProjectPath(project, filepath.Join(project, ".claude", "settings.json")); err == nil {
		t.Error("an output folder linked into the linked spec folder must be refused")
	}
	symlinkOrSkip(t, filepath.Join(outside, "profile"), filepath.Join(shared, ".sync-state"))
	err := CheckBookkeepingPath(project, ledger)
	if err == nil || !strings.Contains(err.Error(), "is a link to "+filepath.Join(outside, "profile")) {
		t.Errorf("a ledger linked elsewhere must be refused, got %v", err)
	}
}

func TestWriteFile_RefusesAFolderNamedLikeGitUnderAnOutputFolder(t *testing.T) {
	project, _ := linkProject(t)

	for _, name := range []string{".git", ".GIT", ".git."} {
		err := NewSession().WriteFile(filepath.Join(".claude", "skills", name, "HEAD"), "ref: x\n", false)

		if err == nil || !strings.Contains(err.Error(), "Git folder") {
			t.Errorf("%s: want a refusal to write into a Git folder, got %v", name, err)
		}
	}
	if _, err := os.Lstat(filepath.Join(project, ".claude")); !os.IsNotExist(err) {
		t.Errorf(".claude must not exist: %v", err)
	}
}

func TestGitFolderName(t *testing.T) {
	cases := []struct {
		seg, goos string
		want      bool
	}{
		{".git", "linux", true},
		{".Git", "darwin", true},
		{".git ", "linux", true},
		{".git..", "windows", true},
		{"GIT~1", "windows", true},
		{".git::$INDEX_ALLOCATION", "windows", true},
		{"GIT~1", "linux", false},
		{".github", "linux", false},
		{"git", "windows", false},
		{".gitignore", "windows", false},
	}
	for _, c := range cases {
		if got := gitFolderName(c.seg, c.goos); got != c.want {
			t.Errorf("gitFolderName(%q, %s) = %v, want %v", c.seg, c.goos, got, c.want)
		}
	}
}
