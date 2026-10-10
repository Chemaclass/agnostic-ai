package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// hookBlock is the part of a git hook install-hook owns. Its sentinel
// comment line marks a hook as installed.
type hookBlock struct {
	// file is the hook's filename under a hooks directory, e.g.
	// "pre-commit" or "post-checkout".
	file     string
	sentinel string
	// legacy is the line older versions wrote without a sentinel.
	legacy       string
	checks       string
	legacyChecks string
	// upgrades maps a check line an older version wrote to the line that
	// replaces it, so installing again updates an existing hook.
	upgrades map[string]string
}

func (b hookBlock) text() string {
	return b.sentinel + "\n" + b.checks + "\n"
}

func (b hookBlock) installedIn(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == b.sentinel || (b.legacy != "" && line == b.legacy) {
			return true
		}
	}
	return false
}

var projectHook = hookBlock{
	file:     "pre-commit",
	sentinel: "# agnostic-ai install-hook",
	legacy:   "agnostic-ai sync --check",
	// The staged state is what the commit holds: checking the working
	// tree passed a commit that left regenerated outputs unstaged.
	checks: projectPreCommitHookChecks,
	upgrades: map[string]string{
		"agnostic-ai sync --check || exit 1":                 projectPreCommitHookChecks,
		"agnostic-ai sync --check":                           projectPreCommitHookChecks,
		"agnostic-ai sync --check --against index || exit 1": projectPreCommitHookChecks,
	},
}

// globalHook runs from the home's repository root, so the checks read
// the home being committed even when AGNOSTIC_AI_HOME names another.
var globalHook = hookBlock{
	file:     "pre-commit",
	sentinel: "# agnostic-ai install-hook --global",
	checks: `AGNOSTIC_AI_HOME="$(git rev-parse --show-toplevel)" || exit 1
export AGNOSTIC_AI_HOME
agnostic-ai lint --global --strict || exit 1
agnostic-ai validate --global || exit 1
# A linked worktree holds a branch, not the specs sync --global deployed,
# so only the main checkout compares them with the live files.
git_dir="$(cd "$(git rev-parse --git-dir)" && pwd -P)" || exit 1
common_dir="$(cd "$(git rev-parse --git-common-dir)" && pwd -P)" || exit 1
if [ "$git_dir" = "$common_dir" ]; then
	agnostic-ai sync --global --check || exit 1
fi`,
}

const legacyProjectSyncHookChecks = `root="$(git rev-parse --show-toplevel)" || exit 0
command -v agnostic-ai >/dev/null 2>&1 || exit 0
[ -f "$root/agnostic-ai.yaml" ] || exit 0
cd "$root" && agnostic-ai sync -q`

const projectHookResolver = `root="$(git rev-parse --show-toplevel)" || exit 1
cd "$root" || exit 1
agnostic_ai_bin=agnostic-ai
if [ -x "$root/node_modules/.bin/agnostic-ai" ]; then
	agnostic_ai_bin="$root/node_modules/.bin/agnostic-ai"
fi`

const projectHookCapability = `command -v "$agnostic_ai_bin" >/dev/null 2>&1 || { echo 'agnostic-ai is missing; install it, then run agnostic-ai project --bootstrap' >&2; exit 1; }
AGNOSTIC_AI_NO_UPDATE_CHECK=1 "$agnostic_ai_bin" project --help >/dev/null 2>&1 || { echo 'agnostic-ai: upgrade the selected package to a release supporting project; update its declared version and requires explicitly' >&2; exit 1; }`

const projectPreCommitHookChecks = projectHookResolver + "\n" + projectHookCapability + "\n\"$agnostic_ai_bin\" project --check --against index || exit 1"

const projectSyncHookChecks = projectHookResolver + `
[ -f "$root/agnostic-ai.yaml" ] || exit 0
` + projectHookCapability + "\n\"$agnostic_ai_bin\" project"

var postCheckoutHook = hookBlock{
	file:         "post-checkout",
	sentinel:     "# agnostic-ai install-hook --post-checkout",
	checks:       `[ "$3" = "1" ] || exit 0` + "\n" + projectSyncHookChecks,
	legacyChecks: `[ "$3" = "1" ] || exit 0` + "\n" + legacyProjectSyncHookChecks,
}

var postMergeHook = hookBlock{
	file:         "post-merge",
	sentinel:     "# agnostic-ai install-hook --post-checkout (post-merge)",
	checks:       projectSyncHookChecks,
	legacyChecks: legacyProjectSyncHookChecks,
}

func newInstallHookCmd() *cobra.Command {
	var shared, global, postCheckout bool
	cmd := &cobra.Command{
		Use:   "install-hook",
		Short: "Install a pre-commit check or checkout and merge sync hooks.",
		Long: "Writes .git/hooks/pre-commit (or appends to an existing file). " +
			"With --shared, writes to .githooks/pre-commit and sets core.hooksPath so " +
			"the hook is committed alongside the project. With --global, run in the " +
			"global home kept in git, writes a hook that runs lint --global --strict, " +
			"validate --global, and sync --global --check. With --post-checkout, writes " +
			"post-checkout and post-merge in .git/hooks (or .githooks with --shared) instead: " +
			"they run `agnostic-ai project` with the local binary from the worktree root after a branch or " +
			"worktree checkout or a merge, including a pull, to restore the tool files.",
		Example: `  # Install into .git/hooks/pre-commit (local only)
  agnostic-ai install-hook

  # Install into .githooks/ and set core.hooksPath (shared with team)
  agnostic-ai install-hook --shared

  # Gate commits to the global home, run inside ~/.agnostic-ai
  agnostic-ai install-hook --global

  # Regenerate tool files after a checkout or pull
  agnostic-ai install-hook --post-checkout

  # Same, committed alongside the project
  agnostic-ai install-hook --post-checkout --shared`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if global {
				return installGlobalPreCommitHook(".", cmd.OutOrStdout())
			}
			if err := refuseGlobalHome(".", globalHomeHookRemedy); err != nil {
				return err
			}
			if postCheckout {
				for _, block := range []hookBlock{postCheckoutHook, postMergeHook} {
					if err := installHook(".", block, shared, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
						return err
					}
				}
				return nil
			}
			return installPreCommitHook(".", shared, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&shared, "shared", false, "Write .githooks/<hook> and set core.hooksPath so the hook is shared with the team.")
	cmd.Flags().BoolVar(&global, "global", false, "Write the pre-commit hook of the global home in $AGNOSTIC_AI_HOME (default ~/.agnostic-ai), which must be the root of a git repository.")
	cmd.Flags().BoolVar(&postCheckout, "post-checkout", false, "Write post-checkout and post-merge hooks that run `agnostic-ai project` after a checkout or pull, preserving edits and never installing dependencies.")
	cmd.MarkFlagsMutuallyExclusive("shared", "global")
	cmd.MarkFlagsMutuallyExclusive("global", "post-checkout")
	return cmd
}

// installGlobalPreCommitHook writes the global home's pre-commit hook.
// dir must be the home itself, at the root of its git repository, so
// the hook's checks read the same home the commit is for.
func installGlobalPreCommitHook(dir string, out io.Writer) error {
	source, err := globalSourceRoot()
	if err != nil {
		return err
	}
	home, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("global home %s: %w", source, err)
	}
	if here, err := os.Stat(dir); err != nil || !os.SameFile(here, home) {
		return fmt.Errorf("install-hook --global runs in the global home %s; cd there first, or set %s to this directory", source, envUserGlobalRoot)
	}
	top, err := gitRevParse(source, "--show-toplevel")
	if err != nil {
		return fmt.Errorf("%s is not the root of a git repository; run `git init` there first", source)
	}
	if info, err := os.Stat(top); err != nil || !os.SameFile(info, home) {
		return fmt.Errorf("%s is not the root of a git repository (it sits inside %s); keep the global home in its own repository", source, top)
	}
	hooksDir, err := gitHooksDir(source, globalHook)
	if err != nil {
		return err
	}
	hookPath := filepath.Join(hooksDir, "pre-commit")
	if existing, err := os.ReadFile(hookPath); err == nil && hasProjectCheck(string(existing)) {
		return fmt.Errorf("%s runs `agnostic-ai sync --check`, which fails in the global home without blocking the commit; remove that line and run install-hook --global again", hookPath)
	}
	written, err := writeHookAt(hooksDir, globalHook)
	if err != nil {
		return err
	}
	reportHook(out, hookPath, written, "")
	return nil
}

// gitHooksDir returns the hooks directory of the repository at dir, and
// refuses when core.hooksPath sends git elsewhere: a hook written there
// would never run, and a user-level hooksPath would run it in every
// repo. Resolved through git rev-parse rather than a filesystem walk
// for a `.git` directory, so it works from a linked worktree too, where
// `.git` is a file naming the common dir, not a directory (#1330
// review): hooks always run from the common dir, shared by every
// worktree, regardless of which one dir is.
func gitHooksDir(dir string, block hookBlock) (string, error) {
	commonDir, err := gitRevParse(dir, "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("%s is not inside a git work tree; run `git init` first", dir)
	}
	hooksDir := absFrom(dir, filepath.Join(commonDir, "hooks"))
	gitPath, err := gitRevParse(dir, "--git-path", "hooks")
	if err != nil {
		return "", err
	}
	if active := absFrom(dir, gitPath); !sameHooksDir(active, hooksDir) {
		value, origin := gitHooksPathSetting(dir)
		return "", fmt.Errorf("core.hooksPath is %s (set in %s), so git runs hooks from %s, not %s; unset it, or add these lines to the %s hook there by hand:\n\n%s", value, origin, active, hooksDir, block.file, block.text())
	}
	return hooksDir, nil
}

// gitHooksPathSetting returns core.hooksPath and the config file that
// sets it, or empty strings when it is unset.
func gitHooksPathSetting(dir string) (string, string) {
	out, err := exec.Command("git", "-C", dir, "config", "--show-origin", "core.hooksPath").Output()
	if err != nil {
		return "", ""
	}
	return parseConfigOrigin(string(out))
}

// parseConfigOrigin splits a `git config --show-origin` line. Git
// C-quotes an origin path that holds backslashes, so every Windows path.
func parseConfigOrigin(line string) (string, string) {
	origin, value, _ := strings.Cut(strings.TrimSpace(line), "\t")
	origin = strings.TrimPrefix(origin, "file:")
	if unquoted, err := strconv.Unquote(origin); err == nil {
		origin = unquoted
	}
	return value, origin
}

// hasProjectCheck reports a project sync --check line, which has no
// place in the global home's hook.
func hasProjectCheck(content string) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), projectHook.legacy) || strings.HasPrefix(strings.TrimSpace(line), "agnostic-ai project --check") || strings.HasPrefix(strings.TrimSpace(line), `"$agnostic_ai_bin" project --check`) {
			return true
		}
	}
	return false
}

func gitRevParse(dir string, args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", dir, "rev-parse"}, args...)...).Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse %s in %s: %w", strings.Join(args, " "), dir, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func absFrom(dir, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(dir, path)
}

// sameHooksDir compares two paths through symlinks when both exist.
func sameHooksDir(a, b string) bool {
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	if aerr == nil && berr == nil {
		return os.SameFile(ai, bi)
	}
	return sameDir(a, b)
}

// installHook writes block's hook file, locally or, with shared, into
// the repo's shared hooks directory. Shared by the pre-commit and
// regeneration hooks: each names its own file via block.file.
func installHook(root string, block hookBlock, shared bool, out, warn io.Writer) error {
	if shared {
		return installSharedHook(root, block, out, warn)
	}
	return installLocalHook(root, block, out)
}

// installPreCommitHook is installHook pinned to projectHook.
func installPreCommitHook(root string, shared bool, out, warn io.Writer) error {
	return installHook(root, projectHook, shared, out, warn)
}

const sharedHooksPath = ".githooks"

// mainWorktreeToplevel returns the root of the repository's main
// working tree, from dir, whether dir sits in the main worktree or a
// linked one. The common dir (`--git-common-dir`) is always the main
// worktree's own `.git`, in both cases, so its parent is the answer.
func mainWorktreeToplevel(dir string) (string, error) {
	commonDir, err := gitRevParse(dir, "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", fmt.Errorf("%s is not inside a git work tree; run `git init` first", dir)
	}
	return filepath.Dir(commonDir), nil
}

// installSharedHook writes .githooks/<block.file> at the main worktree's
// root and sets core.hooksPath so the hook lives in the repo and runs
// for every collaborator. Always the main worktree's root, never the
// invoking one's: core.hooksPath is one repository-level setting every
// worktree resolves against its own directory, so a copy written under
// a linked worktree would leave every other worktree, main included,
// pointed at a hooks directory with nothing in it (#1330 review).
func installSharedHook(dir string, block hookBlock, out, warn io.Writer) error {
	top, err := mainWorktreeToplevel(dir)
	if err != nil {
		return err
	}
	value, origin := gitHooksPathSetting(top)
	if value != "" && value != sharedHooksPath {
		return fmt.Errorf("core.hooksPath is already %s (set in %s), and --shared would replace it; add these lines to the %s hook there by hand, or unset core.hooksPath:\n\n%s", value, origin, block.file, block.text())
	}
	var stopped []string
	if value == "" {
		if stopped, err = activeHooks(top); err != nil {
			return err
		}
	}
	written, err := writeHookAt(filepath.Join(top, sharedHooksPath), block)
	if err != nil {
		return err
	}
	if err := exec.Command("git", "-C", top, "config", "core.hooksPath", sharedHooksPath).Run(); err != nil {
		return fmt.Errorf("git config core.hooksPath: %w", err)
	}
	reportHook(out, filepath.Join(top, sharedHooksPath, block.file), written, " (core.hooksPath → "+sharedHooksPath+")")
	if len(stopped) > 0 {
		_, _ = fmt.Fprintf(warn, "warning: git no longer runs these hooks, since core.hooksPath is now %s: %s\n", sharedHooksPath, strings.Join(stopped, ", "))
	}
	return nil
}

// activeHooks lists the hooks in the repository's hooks directory that
// git runs today, skipping the .sample files git init writes.
func activeHooks(top string) ([]string, error) {
	commonDir, err := gitRevParse(top, "--git-common-dir")
	if err != nil {
		return nil, err
	}
	hooksDir := absFrom(top, filepath.Join(commonDir, "hooks"))
	entries, err := os.ReadDir(hooksDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", hooksDir, err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && !strings.HasSuffix(entry.Name(), ".sample") {
			names = append(names, filepath.Join(hooksDir, entry.Name()))
		}
	}
	return names, nil
}

// installLocalHook writes to the repository's hooks directory: the
// common dir's, shared by every worktree, not root's own (#1330 review).
func installLocalHook(root string, block hookBlock, out io.Writer) error {
	hooksDir, err := gitHooksDir(root, block)
	if err != nil {
		return err
	}
	written, err := writeHookAt(hooksDir, block)
	if err != nil {
		return err
	}
	reportHook(out, filepath.Join(hooksDir, block.file), written, "")
	return nil
}

type hookWrite int

const (
	hookCreated hookWrite = iota
	hookAppended
	hookUnchanged
	hookUpdated
)

func reportHook(out io.Writer, path string, written hookWrite, suffix string) {
	switch written {
	case hookAppended:
		_, _ = fmt.Fprintf(out, "✓ appended the checks to %s, after its existing content%s\n", path, suffix)
	case hookUnchanged:
		_, _ = fmt.Fprintf(out, "✓ %s already runs the checks%s\n", path, suffix)
	case hookUpdated:
		_, _ = fmt.Fprintf(out, "✓ updated the checks in %s%s\n", path, suffix)
	default:
		_, _ = fmt.Fprintf(out, "✓ installed %s%s\n", path, suffix)
	}
}

// writeHookAt ensures dir exists and writes block into its hook file
// (block.file): a new sh script when there is none, nothing when the
// hook already holds block, and otherwise block appended to the hook.
func writeHookAt(dir string, block hookBlock) (hookWrite, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return 0, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	path := filepath.Join(dir, block.file)
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return 0, fmt.Errorf("read %s: %w", path, err)
	}
	if strings.TrimSpace(string(existing)) == "" {
		return hookCreated, os.WriteFile(path, []byte("#!/bin/sh\n"+block.text()), 0o755)
	}
	if block.installedIn(string(existing)) {
		if block.legacyChecks != "" && strings.Contains(string(existing), block.legacyChecks) {
			content := strings.Replace(string(existing), block.legacyChecks, block.checks, 1)
			return hookUpdated, os.WriteFile(path, []byte(content), 0o755)
		}
		lines := strings.Split(string(existing), "\n")
		updated := false
		for i, line := range lines {
			if next, ok := block.upgrades[strings.TrimSpace(line)]; ok {
				lines[i] = strings.Replace(line, strings.TrimSpace(line), next, 1)
				updated = true
			}
		}
		if !updated {
			return hookUnchanged, nil
		}
		return hookUpdated, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o755)
	}
	if reason := appendBlocker(string(existing)); reason != "" {
		return 0, fmt.Errorf("%s %s, so checks appended to it would never run; add these lines by hand where they run:\n\n%s", path, reason, block.text())
	}
	content := strings.TrimRight(string(existing), "\n") + "\n\n" + block.text()
	return hookAppended, os.WriteFile(path, []byte(content), 0o755)
}

// appendBlocker says why lines appended to an existing hook would never
// run, or returns "" when they would. Only sh and bash hooks reach
// their end; an exec that replaces the shell or an unindented exit ends
// them first. An indented exit usually sits in a failure branch.
func appendBlocker(content string) string {
	lines := strings.Split(content, "\n")
	if !strings.HasPrefix(lines[0], "#!") {
		return "has no #!/bin/sh line"
	}
	if shell := shebangCommand(lines[0]); shell != "sh" && shell != "bash" {
		return fmt.Sprintf("runs %s, not sh or bash", shell)
	}
	for _, line := range lines[1:] {
		trimmed := strings.TrimSpace(line)
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		command := strings.TrimSuffix(fields[0], ";")
		if command == "exec" && !onlyRedirects(fields[1:]) {
			return fmt.Sprintf("ends in its %q line", trimmed)
		}
		if command == "exit" && line == trimmed {
			return fmt.Sprintf("ends at its %q line", trimmed)
		}
	}
	return ""
}

// shebangCommand names the interpreter a #! line runs, looking through
// /usr/bin/env.
func shebangCommand(line string) string {
	fields := strings.Fields(strings.TrimPrefix(line, "#!"))
	if len(fields) == 0 {
		return ""
	}
	command := filepath.Base(fields[0])
	if command != "env" {
		return command
	}
	for _, field := range fields[1:] {
		if !strings.HasPrefix(field, "-") {
			return filepath.Base(field)
		}
	}
	return command
}

// onlyRedirects reports an exec that only redirects, such as exec 1>&2,
// which keeps the shell running.
func onlyRedirects(args []string) bool {
	for _, arg := range args {
		if rest := strings.TrimLeft(arg, "0123456789"); !strings.HasPrefix(rest, ">") && !strings.HasPrefix(rest, "<") {
			return false
		}
	}
	return true
}
