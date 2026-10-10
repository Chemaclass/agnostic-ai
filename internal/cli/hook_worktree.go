package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newHookWorktreeRemoveCmd() *cobra.Command {
	var repo, allowedRoot string
	cmd := &cobra.Command{
		Use:   "worktree-remove",
		Short: "Remove a clean registered worktree inside an allowed root",
		Long: "Reads a Claude Code WorktreeRemove payload on stdin. --repo anchors repository ownership; " +
			"--allowed-root explicitly permits cleanup of its descendants. Refuses the main checkout, " +
			"dirty or locked worktrees, unregistered directories, traversal, and symlink paths. " +
			"Uses git worktree remove without force. An already absent path prints its state and exits 0 " +
			"without pruning registrations. Branches are preserved.",
		Example: `  agnostic-ai hook worktree-remove --repo /path/to/repository --allowed-root /path/to/disposable-worktrees`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(repo) == "" || strings.TrimSpace(allowedRoot) == "" {
				return fmt.Errorf("worktree removal requires nonempty --repo and --allowed-root")
			}
			var payload struct {
				Event string `json:"hook_event_name"`
				Path  string `json:"worktree_path"`
			}
			const maxPayloadBytes = 1 << 20
			raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxPayloadBytes+1))
			if err != nil {
				return fmt.Errorf("read worktree removal payload: %w", err)
			}
			if len(raw) > maxPayloadBytes {
				return fmt.Errorf("worktree removal payload exceeds %d bytes", maxPayloadBytes)
			}
			dec := json.NewDecoder(bytes.NewReader(raw))
			if err := dec.Decode(&payload); err != nil {
				return fmt.Errorf("parse worktree removal payload: %w", err)
			}
			var extra any
			if err := dec.Decode(&extra); err != io.EOF {
				return fmt.Errorf("worktree removal payload must be one JSON object")
			}
			if payload.Event != "WorktreeRemove" || !filepath.IsAbs(payload.Path) || strings.ContainsRune(payload.Path, 0) {
				return fmt.Errorf("worktree removal requires a WorktreeRemove event and an absolute worktree_path")
			}
			state, err := removeHookWorktree(repo, allowedRoot, payload.Path)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s\n", state, payload.Path)
			return err
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "Repository whose registered worktree may be removed (use a surviving checkout)")
	cmd.Flags().StringVar(&allowedRoot, "allowed-root", "", "Directory whose descendants are explicitly disposable")
	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("allowed-root")
	return cmd
}

func removeHookWorktree(repo, allowedRoot, candidate string) (string, error) {
	for _, part := range strings.FieldsFunc(candidate, func(r rune) bool { return r == '/' || r == filepath.Separator }) {
		if part == ".." {
			return "", fmt.Errorf("%s: worktree path contains traversal", candidate)
		}
	}
	root, err := filepath.Abs(allowedRoot)
	if err != nil {
		return "", fmt.Errorf("resolve allowed cleanup root: %w", err)
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%s: worktree must be below allowed cleanup root %s", candidate, root)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resolve allowed cleanup root: %w", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return "", fmt.Errorf("%s: allowed cleanup root is not a directory", root)
	}
	path := root
	missing := false
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			missing = true
			continue
		}
		if err != nil {
			return "", fmt.Errorf("%s: inspect worktree path: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("%s: worktree path passes through a symlink", path)
		}
	}
	repository, err := filepath.Abs(repo)
	if err != nil {
		return "", fmt.Errorf("resolve repository: %w", err)
	}
	repository, err = filepath.EvalSymlinks(repository)
	if err != nil {
		return "", fmt.Errorf("resolve repository: %w", err)
	}
	raw, err := worktreeGit(repository, "worktree", "list", "--porcelain", "-z")
	if err != nil {
		return "", err
	}
	entries := strings.Split(string(raw), "\x00")
	registered, main := false, ""
	locked := false
	selected := false
	for _, entry := range entries {
		if strings.HasPrefix(entry, "worktree ") {
			listed := strings.TrimPrefix(entry, "worktree ")
			if resolved, err := filepath.EvalSymlinks(listed); err == nil {
				listed = resolved
			}
			if main == "" {
				main = listed
			}
			selected = sameWorktreePath(listed, path)
			registered = registered || selected
		} else if selected && (entry == "locked" || strings.HasPrefix(entry, "locked ")) {
			locked = true
		}
	}
	if sameWorktreePath(main, path) {
		return "", fmt.Errorf("%s: refusing to remove the main checkout", path)
	}
	if locked {
		return "", fmt.Errorf("%s: worktree is locked", path)
	}
	if missing {
		return "already absent", nil
	}
	if !registered {
		return "", fmt.Errorf("%s: not a registered worktree of %s", path, repository)
	}
	if err := verifyWorktreeRepository(repository, path); err != nil {
		return "", err
	}
	index, err := worktreeGit(path, "ls-files", "-v", "-z")
	if err != nil {
		return "", err
	}
	// These index flags can hide changed content from both status and removal.
	for _, entry := range strings.Split(string(index), "\x00") {
		if entry != "" && (entry[0] == 'S' || entry[0] >= 'a' && entry[0] <= 'z') {
			return "", fmt.Errorf("%s: worktree index contains assume-unchanged or skip-worktree entries", path)
		}
	}
	// Git's non-force removal permits ignored files; preserve those files too.
	status, err := worktreeGit(path, "status", "--porcelain", "-z", "--untracked-files=all", "--ignored")
	if err != nil {
		return "", err
	}
	if len(status) != 0 {
		return "", fmt.Errorf("%s: worktree contains changed, untracked, or ignored files", path)
	}
	if _, err := worktreeGit(repository, "worktree", "remove", "--", path); err != nil {
		return "", err
	}
	return "removed", nil
}

func verifyWorktreeRepository(repository, path string) error {
	common, err := worktreeGit(repository, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	owned, err := worktreeGit(path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	top, err := worktreeGit(path, "rev-parse", "--show-toplevel")
	if err != nil {
		return err
	}
	if !sameWorktreePath(strings.TrimSpace(string(common)), strings.TrimSpace(string(owned))) ||
		!sameWorktreePath(strings.TrimSpace(string(top)), path) {
		return fmt.Errorf("%s: registered path no longer belongs to the expected repository", path)
	}
	return nil
}

func sameWorktreePath(a, b string) bool {
	if filepath.Clean(a) == filepath.Clean(b) {
		return true
	}
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	return aerr == nil && berr == nil && os.SameFile(ai, bi)
}

func worktreeGit(repo string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = worktreeGitEnvironment(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func worktreeGitEnvironment(environ []string) []string {
	env := make([]string, 0, len(environ))
	for _, value := range environ {
		key, _, _ := strings.Cut(value, "=")
		switch strings.ToUpper(key) {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE":
			continue
		}
		env = append(env, value)
	}
	return env
}
