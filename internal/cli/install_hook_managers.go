package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// hookManager is a tool that owns a repository's git hooks: it writes
// them on its own install, replacing a hook install-hook wrote, or runs
// only the hooks its config lists.
type hookManager struct {
	name string
	// files are the paths, relative to the worktree root, that mark the
	// manager in use. The first that exists names it.
	files []string
	// preCommit and sync are the config lines that run the same command
	// as the pre-commit and the --post-checkout hooks.
	preCommit, sync string
	// install applies the config.
	install string
}

var hookManagers = []hookManager{
	{
		name:  "lefthook",
		files: []string{"lefthook.yml", "lefthook.yaml", ".lefthook.yml", ".lefthook.yaml"},
		preCommit: `pre-commit:
  commands:
    agnostic-ai-check:
      run: agnostic-ai project --check --against index`,
		sync: `post-checkout:
  commands:
    agnostic-ai-sync:
      run: '[ "{3}" = 1 ] || exit 0; agnostic-ai project'
post-merge:
  commands:
    agnostic-ai-sync:
      run: agnostic-ai project
post-rewrite:
  commands:
    agnostic-ai-sync:
      run: '[ "{1}" = rebase ] || exit 0; agnostic-ai project'`,
		install: "lefthook install",
	},
	{
		name:  "husky",
		files: []string{".husky"},
		preCommit: `# .husky/pre-commit
agnostic-ai project --check --against index`,
		sync: `# .husky/post-checkout
[ "$3" = 1 ] || exit 0
agnostic-ai project

# .husky/post-merge
agnostic-ai project

# .husky/post-rewrite
[ "$1" = rebase ] || exit 0
agnostic-ai project`,
		install: "npm install (husky's prepare script)",
	},
	{
		name:  "pre-commit",
		files: []string{".pre-commit-config.yaml"},
		preCommit: `repos:
  - repo: local
    hooks:
      - id: agnostic-ai-check
        name: agnostic-ai project --check
        entry: agnostic-ai project --check --against index
        language: system
        pass_filenames: false`,
		sync: `default_install_hook_types: [pre-commit, post-checkout, post-merge, post-rewrite]
repos:
  - repo: local
    hooks:
      - id: agnostic-ai-sync
        name: agnostic-ai project
        entry: sh -c '[ "${PRE_COMMIT_CHECKOUT_TYPE:-1}" = 1 ] && [ "${PRE_COMMIT_REWRITE_COMMAND:-rebase}" = rebase ] || exit 0; agnostic-ai project'
        language: system
        pass_filenames: false
        always_run: true
        stages: [post-checkout, post-merge, post-rewrite]`,
		install: "pre-commit install --hook-type post-checkout --hook-type post-merge --hook-type post-rewrite",
	},
}

// detectHookManager returns the hook manager the worktree at root uses
// and the file that shows it, or false when it uses none.
func detectHookManager(root string) (hookManager, string, bool) {
	for _, manager := range hookManagers {
		for _, file := range manager.files {
			if _, err := os.Stat(filepath.Join(root, file)); err == nil {
				return manager, file, true
			}
		}
	}
	return hookManager{}, "", false
}

// printHookManagerSnippet prints the lines to add to the hook manager the
// repository at dir uses, and reports whether it found one. A manager
// replaces the hooks in .git/hooks on its next install, or runs only its
// own, so install-hook leaves the hooks to it.
func printHookManagerSnippet(dir string, postCheckout bool, out io.Writer) (bool, error) {
	top, err := gitRevParse(dir, "--show-toplevel")
	if err != nil {
		return false, nil
	}
	manager, file, ok := detectHookManager(top)
	if !ok {
		return false, nil
	}
	snippet, use := manager.preCommit, "check staged outputs before each commit"
	if postCheckout {
		snippet, use = manager.sync, "sync after each branch checkout, merge and rebase"
	}
	_, err = fmt.Fprintf(out, "%s manages this repository's git hooks (%s), so install-hook wrote nothing: %s would replace or skip a hook in .git/hooks.\nTo %s, add this to its config, then run %s:\n\n%s\n\nRun install-hook --force to write the plain git hooks anyway.\n",
		manager.name, file, manager.name, use, manager.install, snippet)
	return true, err
}
