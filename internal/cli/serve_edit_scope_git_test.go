package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// gitScopeProject is scopeProject committed to a fresh Git repository
// that has never synced, so nothing ignores the personal files yet.
func gitScopeProject(t *testing.T) string {
	t.Helper()
	isolateGit(t)
	project := scopeProject(t)
	mustWrite(t, ".agnostic-ai/settings/native.yaml", "name: native\nx-claude:\n  verbose: false\n")
	git(t, project, "init", "-q")
	git(t, project, "add", ".")
	git(t, project, "commit", "-qm", "base")
	return project
}

func assertGitHides(t *testing.T, repo, file string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repo, file)); err != nil {
		t.Fatalf("%s was not written: %v", file, err)
	}
	if got := git(t, repo, "status", "--porcelain", "--untracked-files=all"); got != "" {
		t.Errorf("git status after a personal save:\n%s", got)
	}
	_ = exec.Command("git", "-C", repo, "add", "-A").Run()
	if got := git(t, repo, "ls-files", "--", file); got != "" {
		t.Errorf("git add staged %s", got)
	}
}

func excludeOf(t *testing.T, repo string) string {
	t.Helper()
	return readOptional(t, filepath.Join(repo, ".git", "info", "exclude"))
}

func TestEditScope_FirstPersonalSaveStaysOutOfGit(t *testing.T) {
	for _, tc := range []struct {
		name, path, body, file string
	}{
		{"scalar field", editFieldPath, `{"scope":"local","path":".agnostic-ai/settings/defaults.yaml","key":"model","value":"opus"}`, ".agnostic-ai/local/settings/defaults.yaml"},
		{"boolean value", editValuePath, `{"scope":"local","path":".agnostic-ai/settings/native.yaml","key":"x-claude.verbose","value":true}`, ".agnostic-ai/local/settings/native.yaml"},
		{"built-in", editBuiltinPath, `{"scope":"local","name":"handoff","enabled":true}`, "agnostic-ai.local.yaml"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := gitScopeProject(t)
			mustWrite(t, ".git/info/exclude", "# mine\n*.log\n")
			rec := postEdit(t, newEditHandler(project, false), tc.path, tc.body, nil)
			if rec.Code != 200 {
				t.Fatalf("save = %d %s", rec.Code, rec.Body)
			}
			assertGitHides(t, project, tc.file)
			if got := excludeOf(t, project); !strings.HasPrefix(got, "# mine\n*.log\n") {
				t.Errorf("info/exclude lost its lines:\n%s", got)
			}
		})
	}
}

func TestEditScope_PersonalSaveKeepsAnExistingIgnoreRule(t *testing.T) {
	project := gitScopeProject(t)
	mustWrite(t, ".gitignore", "/.agnostic-ai/local/\n")
	git(t, project, "add", ".gitignore")
	git(t, project, "commit", "-qm", "ignore")
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"scope":"local","path":".agnostic-ai/settings/defaults.yaml","key":"model","value":"opus"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	assertGitHides(t, project, ".agnostic-ai/local/settings/defaults.yaml")
	if got := excludeOf(t, project); strings.Contains(got, "agnostic-ai/local") {
		t.Errorf("info/exclude gained a rule .gitignore already has:\n%s", got)
	}
}

func TestEditScope_PersonalSaveInANestedProjectAnchorsItsFolder(t *testing.T) {
	isolateGit(t)
	repo := t.TempDir()
	git(t, repo, "init", "-q")
	project := filepath.Join(repo, "app")
	testutil.Chdir(t, repo)
	mustWrite(t, "app/agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWrite(t, "app/.agnostic-ai/settings/defaults.yaml", "name: defaults\nmodel: sonnet\n")
	git(t, repo, "add", ".")
	git(t, repo, "commit", "-qm", "base")
	silence(t)
	testutil.Chdir(t, project)
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"scope":"local","path":".agnostic-ai/settings/defaults.yaml","key":"model","value":"opus"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	assertGitHides(t, repo, "app/.agnostic-ai/local/settings/defaults.yaml")
	if got := excludeOf(t, repo); !strings.Contains(got, "/app/.agnostic-ai/local/\n") {
		t.Errorf("info/exclude = %q, want the nested personal folder", got)
	}
}

func TestEditScope_PersonalSaveInALinkedWorktreeStaysOutOfGit(t *testing.T) {
	main := gitScopeProject(t)
	linked := filepath.Join(t.TempDir(), "linked")
	git(t, main, "worktree", "add", "-q", "--detach", linked)
	testutil.Chdir(t, linked)
	rec := postEdit(t, newEditHandler(linked, false), editFieldPath, `{"scope":"local","path":".agnostic-ai/settings/defaults.yaml","key":"model","value":"opus"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	assertGitHides(t, linked, ".agnostic-ai/local/settings/defaults.yaml")
}

func TestEditScope_PersonalSaveInAGlobalSourceRepositoryStaysOutOfGit(t *testing.T) {
	silence(t)
	isolateGit(t)
	source := editHome(t)
	git(t, source, "init", "-q")
	git(t, source, "add", ".")
	git(t, source, "commit", "-qm", "base")
	rec := postEdit(t, newEditHandler(source, true), editFieldPath, `{"scope":"local","path":"settings/native.yaml","key":"effort","value":"high"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	assertGitHides(t, source, "local/settings/native.yaml")
}

func TestEditScope_PersonalSaveOutsideGitNeedsNoExclusion(t *testing.T) {
	isolateGit(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", os.TempDir())
	project := scopeProject(t)
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"scope":"local","path":".agnostic-ai/settings/defaults.yaml","key":"model","value":"opus"}`, nil)
	if rec.Code != 200 {
		t.Fatalf("save = %d %s", rec.Code, rec.Body)
	}
	if got := readOptional(t, ".agnostic-ai/local/settings/defaults.yaml"); !strings.Contains(got, "model: opus") {
		t.Errorf("overlay = %q", got)
	}
}

func TestEditScope_PersonalSaveRefusesWhenGitCannotIgnoreIt(t *testing.T) {
	project := gitScopeProject(t)
	if err := os.Remove(".git/info/exclude"); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".git/info/exclude", 0o755); err != nil {
		t.Fatal(err)
	}
	rec := postEdit(t, newEditHandler(project, false), editFieldPath, `{"scope":"local","path":".agnostic-ai/settings/defaults.yaml","key":"model","value":"opus"}`, nil)
	if rec.Code < 400 || !strings.Contains(rec.Body.String(), "Git") {
		t.Fatalf("save = %d %s, want a refusal that names Git", rec.Code, rec.Body)
	}
	if _, err := os.Stat(".agnostic-ai/local/settings/defaults.yaml"); !os.IsNotExist(err) {
		t.Errorf("the refused save wrote the overlay: %v", err)
	}
}
