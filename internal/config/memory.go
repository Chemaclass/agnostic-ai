package config

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
)

// Values memory.personal accepts.
const (
	PersonalMemoryCheckout = "checkout"
	PersonalMemoryRepo     = "repo"
)

// PersonalMemoryModes are the values memory.personal accepts.
var PersonalMemoryModes = []string{PersonalMemoryCheckout, PersonalMemoryRepo}

// MemoryConfig tunes the memory built-in for one user.
type MemoryConfig struct {
	Personal string `yaml:"personal,omitempty" json:"personal,omitempty" jsonschema:"enum=checkout,enum=repo" jsonschema_description:"Where personal memory lives: checkout (default) keeps it in .agnostic-ai/local/memory/ of each checkout; repo keeps one store per repository under $AGNOSTIC_AI_HOME/local/memory/, shared by every worktree. Set it in agnostic-ai.local.yaml only."`
}

// RepoPersonalMemory reports whether personal memory lives in the
// per-repository store rather than in the checkout. Nil-safe.
func (c *Config) RepoPersonalMemory() bool {
	return c != nil && c.Memory.Personal == PersonalMemoryRepo
}

// validateMemory rejects an unknown memory.personal value, and the key
// in the shared config: the store path is one user's choice.
func validateMemory(m MemoryConfig, base map[string]any, basePath, source string) error {
	if m.Personal != "" && !slices.Contains(PersonalMemoryModes, m.Personal) {
		return errs.Coded(errs.CodeConfigDecode, "%s: memory.personal: %q is not one of %s",
			source, m.Personal, strings.Join(PersonalMemoryModes, ", "))
	}
	if memory, ok := base["memory"].(map[string]any); ok {
		if _, set := memory["personal"]; set {
			return errs.Coded(errs.CodeConfigDecode, "%s: memory.personal is personal; move it to %s", basePath, LocalOverrideFileName)
		}
	}
	return nil
}

// gitLayout is what Git says about a project's repository.
type gitLayout struct{ gitDir, commonDir, prefix string }

func readGitLayout(root string) (gitLayout, bool) {
	layout, err := gitquery.LayoutAt(root)
	if err != nil {
		return gitLayout{}, false
	}
	return gitLayout{strings.TrimSpace(layout.GitDir), strings.TrimSpace(layout.CommonDir), strings.TrimSpace(layout.Prefix)}, true
}

// ProjectGit is what Git answers for a project whose copy has no .git:
// the shared Git dir that names the repo store, and the memory.personal
// a linked worktree inherits from the main checkout's local config.
type ProjectGit struct {
	CommonDir string
	// Personal and PersonalFrom are empty when nothing is inherited.
	Personal, PersonalFrom string
}

// ReadProjectGit asks Git once about the project at root. ok is false
// outside a repository; an inherited config that does not load is an error.
func ReadProjectGit(root string) (git ProjectGit, ok bool, err error) {
	layout, ok := readGitLayout(root)
	if !ok {
		return ProjectGit{}, false, nil
	}
	var inherited Config
	from, err := inheritFromMainCheckout(layout, &inherited)
	if err != nil {
		return ProjectGit{}, false, err
	}
	return ProjectGit{CommonDir: layout.commonDir, Personal: inherited.Memory.Personal, PersonalFrom: from}, true, nil
}

type projectGitOverride struct {
	root string
	git  ProjectGit
	set  bool
}

var projectGitCopy projectGitOverride

// UseProjectGit makes the copy at copyRoot answer as git until the returned function runs; one preview at a time.
func UseProjectGit(copyRoot string, git ProjectGit) (clear func()) {
	projectGitCopy = projectGitOverride{root: copyRoot, git: git, set: true}
	return func() { projectGitCopy = projectGitOverride{} }
}

// ProjectGitFor returns the answer set for the project at root.
func ProjectGitFor(root string) (ProjectGit, bool) {
	if !projectGitCopy.set {
		return ProjectGit{}, false
	}
	a, errA := filepath.Abs(root)
	b, errB := filepath.Abs(projectGitCopy.root)
	if errA != nil || errB != nil {
		return ProjectGit{}, false
	}
	if a != b {
		ra, errA := filepath.EvalSymlinks(a)
		rb, errB := filepath.EvalSymlinks(b)
		if errA != nil || errB != nil || ra != rb {
			return ProjectGit{}, false
		}
	}
	return projectGitCopy.git, true
}

func inheritPersonalMemory(cfg *Config, root string) (string, error) {
	if git, ok := ProjectGitFor(root); ok {
		if git.Personal != "" {
			cfg.Memory.Personal = git.Personal
		}
		return git.PersonalFrom, nil
	}
	layout, ok := readGitLayout(root)
	if !ok {
		return "", nil
	}
	return inheritFromMainCheckout(layout, cfg)
}

func inheritFromMainCheckout(layout gitLayout, cfg *Config) (string, error) {
	if layout.gitDir == layout.commonDir || filepath.Base(layout.commonDir) != ".git" {
		return "", nil
	}
	path := filepath.Join(filepath.Dir(layout.commonDir), filepath.FromSlash(layout.prefix), LocalOverrideFileName)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", nil
	} else if err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	doc, err := readYAMLMap(path)
	if err != nil {
		return "", err
	}
	memory, _ := doc["memory"].(map[string]any)
	value, set := memory["personal"]
	if !set {
		return "", nil
	}
	mode, ok := value.(string)
	if !ok {
		return "", errs.Coded(errs.CodeConfigDecode, "%s: memory.personal must be a string", path)
	}
	if err := validateMemory(MemoryConfig{Personal: mode}, nil, "", path); err != nil {
		return "", err
	}
	cfg.Memory.Personal = mode
	return path, nil
}
