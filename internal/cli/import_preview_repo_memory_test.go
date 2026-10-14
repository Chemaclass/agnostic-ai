package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const fencedDemoSkill = "---\nname: demo\ndescription: Demo skill.\n---\n\n# demo\n\n::target claude\n\nClaude only line.\n\n::end\n\nShared line.\n"

const previewProjectConfig = "version: 1\ntargets: [claude, cursor]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n"

func gitIn(t *testing.T, dir string, args ...string) {
	t.Helper()
	full := append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false"}, args...)
	if out, err := exec.Command("git", full...).CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// previewProject leaves the working directory in a synced project that
// holds a fenced skill. layout names where the project sits: its own
// repository (checkout, repo), a folder of one (subdir), or a linked
// worktree whose main checkout holds the local config (worktree,
// worktree-subdir). It returns the Git dirs whose index the preview must
// leave alone.
func previewProject(t *testing.T, layout string) []string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", home)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	main := filepath.Join(base, "main")
	sub := ""
	if strings.HasSuffix(layout, "subdir") || layout == "subdir" {
		sub = "sub"
	}
	project := filepath.Join(main, sub)
	for _, dir := range []string{main, project} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	gitIn(t, main, "init", "-q")
	silence(t)
	testutil.Chdir(t, project)
	mustWriteFile(t, "agnostic-ai.yaml", previewProjectConfig)
	mustWriteFile(t, ".agnostic-ai/skills/demo/SKILL.md", fencedDemoSkill)
	local := "memory:\n  personal: repo\n"
	if layout != "checkout" && !strings.HasPrefix(layout, "worktree") {
		mustWriteFile(t, "agnostic-ai.local.yaml", local)
	}
	gitDirs := []string{filepath.Join(main, ".git")}
	if strings.HasPrefix(layout, "worktree") {
		gitIn(t, main, "add", "-A")
		gitIn(t, main, "commit", "-q", "-m", "init")
		wt := filepath.Join(base, "wt")
		gitIn(t, main, "worktree", "add", "-q", wt)
		gitDirs = append(gitDirs, filepath.Join(main, ".git", "worktrees", "wt"))
		mustWriteFile(t, filepath.Join(project, "agnostic-ai.local.yaml"), local)
		testutil.Chdir(t, filepath.Join(wt, sub))
	}
	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	return gitDirs
}

func readIndexes(t *testing.T, gitDirs []string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, dir := range gitDirs {
		data, err := os.ReadFile(filepath.Join(dir, "index"))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		out[dir] = string(data)
	}
	return out
}

// With memory.personal: repo the preview ran in a copy without .git, so
// the repo store did not resolve there and the preview stopped seeing the
// generated skill files as sync output (#2181).
func TestImport_DryRunMatchesImportWithRepoMemory(t *testing.T) {
	for _, layout := range []string{"checkout", "repo", "subdir", "worktree", "worktree-subdir"} {
		t.Run(layout, func(t *testing.T) {
			gitDirs := previewProject(t, layout)
			indexes, env := readIndexes(t, gitDirs), os.Environ()

			var previewErr error
			out := captureStdout(t, func() {
				_, previewErr = runCLI(t, "import", "cursor", "--dry-run", "--diff")
			})
			if previewErr != nil {
				t.Errorf("import --dry-run --diff: %v", previewErr)
			}
			if strings.Contains(out, "  change ") || strings.Contains(out, "::target claude") {
				t.Errorf("preview reports changes the import does not make:\n%s", out)
			}
			if !strings.Contains(out, "unchanged .agnostic-ai/skills/demo/SKILL.md") {
				t.Errorf("preview does not list the skill as unchanged:\n%s", out)
			}
			for dir, index := range readIndexes(t, gitDirs) {
				if index != indexes[dir] {
					t.Errorf("preview changed %s/index", dir)
				}
			}
			if after := os.Environ(); !slices.Equal(after, env) {
				t.Errorf("preview changed the environment:\nbefore %v\nafter  %v", env, after)
			}

			if out, err := runCLI(t, "import", "cursor"); err != nil {
				t.Fatalf("import cursor: %v\n%s", err, out)
			}
			if got := readFile(t, ".agnostic-ai/skills/demo/SKILL.md"); got != fencedDemoSkill {
				t.Errorf("import rewrote the skill:\n%s", got)
			}
		})
	}
}

func TestImport_DryRunFailsLikeImportWhenTheMainCheckoutConfigDoesNotLoad(t *testing.T) {
	gitDirs := previewProject(t, "worktree")
	mustWriteFile(t, filepath.Join(filepath.Dir(gitDirs[0]), "agnostic-ai.local.yaml"), "memory:\n  personal: bogus\n")

	_, importErr := runCLI(t, "import", "cursor")
	_, previewErr := runCLI(t, "import", "cursor", "--dry-run")

	if importErr == nil || !strings.Contains(importErr.Error(), "[AAI-004]") {
		t.Fatalf("import error = %v, want AAI-004", importErr)
	}
	if previewErr == nil || !strings.Contains(previewErr.Error(), "[AAI-004]") {
		t.Errorf("dry run error = %v, want the import's AAI-004", previewErr)
	}
}
