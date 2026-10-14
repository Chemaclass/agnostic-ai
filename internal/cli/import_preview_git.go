package cli

import (
	"os"
	"os/exec"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/config"
)

// useProjectGitInCopy points git at the project's repository while an
// import runs in the copy, which has no .git of its own. With
// memory.personal: repo the personal store is named after the repository,
// and every render the import checks sync output against asks git for
// it. Without an answer the copy renders other files than sync wrote, so
// the preview loses what a real import keeps. Git runs without optional
// locks, so the project's index is not refreshed. Other projects ask
// nothing of git in the copy and keep running without it. The returned
// function restores the environment.
func useProjectGitInCopy(project string) func() {
	cfg, err := config.Load(project)
	if err != nil || !cfg.RepoPersonalMemory() || os.Getenv("GIT_DIR") != "" {
		return func() {}
	}
	out, err := exec.Command("git", "-C", project, "rev-parse", "--absolute-git-dir").Output()
	if err != nil {
		return func() {}
	}
	gitDir := strings.TrimSpace(string(out))
	locks, hadLocks := os.LookupEnv("GIT_OPTIONAL_LOCKS")
	_ = os.Setenv("GIT_DIR", gitDir)
	_ = os.Setenv("GIT_OPTIONAL_LOCKS", "0")
	return func() {
		_ = os.Unsetenv("GIT_DIR")
		if hadLocks {
			_ = os.Setenv("GIT_OPTIONAL_LOCKS", locks)
		} else {
			_ = os.Unsetenv("GIT_OPTIONAL_LOCKS")
		}
	}
}
