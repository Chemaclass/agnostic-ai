package mailbox

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/gitquery"
)

const (
	defaultReviewBase = "refs/remotes/origin/HEAD"
	reviewTimeout     = 10 * time.Second
)

var (
	objectID       = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	reviewRevision = regexp.MustCompile(`^[^-\x00-\x20\x7f][^\x00-\x20\x7f]*$`)
)

func ResolveReviewRefs(ctx context.Context, folder, base, commit string) (ReviewRefs, error) {
	if refused := checkReviewStrings(folder, base, commit); refused != nil {
		return ReviewRefs{}, refused
	}
	if err := ctx.Err(); err != nil {
		return ReviewRefs{}, err
	}
	if folder == "" {
		return ReviewRefs{}, notWorktree()
	}
	ctx, cancel := context.WithTimeout(ctx, reviewTimeout)
	defer cancel()
	out, err := reviewGit(ctx, folder, "rev-parse", "--path-format=absolute", "--show-toplevel")
	if err != nil {
		return ReviewRefs{}, reviewGitFailure(ctx, notWorktree())
	}
	root := strings.TrimSuffix(string(out), "\n")
	if runtime.GOOS == "windows" {
		root = strings.TrimSuffix(root, "\r")
	}
	if !filepath.IsAbs(root) {
		return ReviewRefs{}, notWorktree()
	}
	root = filepath.Clean(root)
	if refused := checkReviewStrings(root); refused != nil {
		return ReviewRefs{}, refused
	}
	if commit == "" {
		commit = "HEAD"
	}
	headID, err := reviewCommitID(ctx, root, commit)
	if err != nil {
		return ReviewRefs{}, err
	}
	defaulted := base == ""
	if defaulted {
		base = defaultReviewBase
	}
	baseID, err := reviewCommitID(ctx, root, base)
	if err != nil {
		if _, ok := errors.AsType[*Refusal](err); defaulted && ok {
			return ReviewRefs{}, missingBase()
		}
		return ReviewRefs{}, err
	}
	if _, err := reviewGit(ctx, root, "merge-base", "--is-ancestor", baseID, headID); err != nil {
		return ReviewRefs{}, reviewGitFailure(ctx, &Refusal{"refs", "The base must be an ancestor of the review commit.", "Choose an earlier commit on the same history."})
	}
	branch, err := reviewBranch(ctx, root)
	if err != nil {
		return ReviewRefs{}, err
	}
	paths, err := reviewPaths(ctx, root, baseID, headID)
	if err != nil {
		return ReviewRefs{}, err
	}
	dirty, err := reviewDirtyFiles(ctx, root)
	if err != nil {
		return ReviewRefs{}, err
	}
	return ReviewRefs{Worktree: root, Commit: headID, Base: baseID, Branch: branch, Paths: paths, Uncommitted: dirty}, nil
}

func resolveReviewSend(ctx context.Context, folder string, input ReviewInput) (ReviewRefs, error) {
	refs, err := ResolveReviewRefs(ctx, folder, input.Base, input.Commit)
	if err != nil {
		return ReviewRefs{}, err
	}
	candidate := filepath.Clean(input.Worktree)
	sameRoot := candidate == refs.Worktree
	if runtime.GOOS == "windows" {
		sameRoot = strings.EqualFold(candidate, refs.Worktree)
	}
	if !filepath.IsAbs(candidate) || !sameRoot {
		return ReviewRefs{}, &Refusal{"refs", "The review worktree differs from the sender's Git worktree.", "Refresh the review details from the sender's row."}
	}
	if input.Paths != nil {
		paths := slices.Clone(input.Paths)
		slices.Sort(paths)
		if !slices.Equal(paths, refs.Paths) {
			return ReviewRefs{}, &Refusal{"refs", "The supplied paths differ from the committed changes.", "Refresh the review details or omit the supplied paths."}
		}
	}
	return refs, nil
}

func reviewCommitID(ctx context.Context, root, ref string) (string, error) {
	if !reviewRevision.MatchString(ref) {
		return "", &Refusal{"refs", "The commit or base is not a valid Git reference.", "Use a branch, tag, or commit ID without whitespace or a leading dash."}
	}
	out, err := reviewGit(ctx, root, "rev-parse", "--verify", "--quiet", "--end-of-options", ref+"^{commit}")
	id := strings.TrimSpace(string(out))
	if err != nil || !objectID.MatchString(id) {
		return "", reviewGitFailure(ctx, &Refusal{"refs", "The commit or base could not be resolved in this Git worktree.", "Use an existing branch, tag, or commit."})
	}
	return id, nil
}

func reviewBranch(ctx context.Context, root string) (string, error) {
	out, err := reviewGit(ctx, root, "symbolic-ref", "--quiet", "HEAD")
	if err != nil {
		if failure, ok := errors.AsType[*exec.ExitError](err); ok && failure.ExitCode() == 1 && ctx.Err() == nil {
			return "", nil
		}
		return "", reviewGitFailure(ctx, NewRefusal("refs"))
	}
	branch := strings.TrimPrefix(strings.TrimSpace(string(out)), "refs/heads/")
	if refused := checkReviewStrings(branch); refused != nil {
		return "", refused
	}
	return branch, nil
}

func reviewPaths(ctx context.Context, root, base, commit string) ([]string, error) {
	out, err := reviewGit(ctx, root, "diff-tree", "-r", "--no-commit-id", "--name-only", "--no-renames",
		"--no-relative", "--no-ext-diff", "--no-textconv", "--ignore-submodules=none", "-z", base, commit, "--")
	if err != nil {
		return nil, reviewGitFailure(ctx, NewRefusal("refs"))
	}
	paths := []string{}
	for len(out) != 0 {
		raw, rest, ok := bytes.Cut(out, []byte{0})
		if !ok {
			return nil, NewRefusal("refs")
		}
		paths = append(paths, string(raw))
		if len(paths) > maxReviewPaths {
			break
		}
		out = rest
	}
	if refused := validateReviewPaths(paths); refused != nil {
		return nil, refused
	}
	slices.Sort(paths)
	return paths, nil
}

func reviewDirtyFiles(ctx context.Context, root string) (int, error) {
	args, err := reviewFilterOverrides(ctx, root)
	if err != nil {
		return 0, reviewGitFailure(ctx, NewRefusal("refs"))
	}
	args = append(args, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--renames", "--ignore-submodules=dirty")
	out, err := reviewGit(ctx, root, args...)
	if err != nil {
		return 0, reviewGitFailure(ctx, NewRefusal("refs"))
	}
	dirty := map[string]struct{}{}
	for len(out) != 0 {
		row, rest, ok := bytes.Cut(out, []byte{0})
		if !ok || len(row) < 4 || row[2] != ' ' {
			return 0, NewRefusal("refs")
		}
		dirty[string(row[3:])] = struct{}{}
		out = rest
		// In porcelain's NUL format the old name follows a rename or copy.
		if bytes.ContainsAny(row[:2], "RC") {
			_, out, ok = bytes.Cut(out, []byte{0})
			if !ok {
				return 0, NewRefusal("refs")
			}
		}
	}
	return len(dirty), nil
}

const disabledReviewFilter = "AGNOSTIC_AI_GIT_FILTER_DISABLED"

func reviewFilterOverrides(ctx context.Context, root string) ([]string, error) {
	// Status can refresh worktree bytes through clean filters. Override them for this read only.
	out, err := reviewGit(ctx, root, "config", "--null", "--name-only", "--get-regexp", `^filter\..*\.(clean|smudge|process|required)$`)
	if err != nil {
		if failure, ok := errors.AsType[*exec.ExitError](err); ok && failure.ExitCode() == 1 && len(out) == 0 && ctx.Err() == nil {
			return nil, nil
		}
		return nil, err
	}
	var overrides []string
	seen := map[string]bool{}
	for key := range strings.SplitSeq(string(out), "\x00") {
		if key != "" && !seen[key] {
			overrides = append(overrides, "--config-env="+key+"="+disabledReviewFilter)
			seen[key] = true
		}
	}
	return overrides, nil
}

func reviewGit(ctx context.Context, dir string, args ...string) ([]byte, error) {
	prefix := []string{"--no-pager", "--no-optional-locks", "--no-replace-objects", "-c", "core.fsmonitor=false", "-C", dir}
	cmd := exec.CommandContext(ctx, "git", append(prefix, args...)...)
	cmd.Env = append(reviewGitEnv(), "GIT_NO_LAZY_FETCH=1", "GIT_ALLOW_PROTOCOL=", disabledReviewFilter+"=")
	cmd.WaitDelay = time.Second
	return cmd.Output()
}

func reviewGitEnv() []string {
	env := gitquery.IsolatedEnv(os.Environ())
	return slices.DeleteFunc(env, func(value string) bool {
		key, _, _ := strings.Cut(value, "=")
		switch strings.ToUpper(key) {
		case "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_QUARANTINE_PATH", "GIT_NAMESPACE", "GIT_SHALLOW_FILE", "GIT_CONFIG":
			return true
		default:
			return false
		}
	})
}

func reviewGitFailure(ctx context.Context, refusal *Refusal) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return refusal
}

func notWorktree() *Refusal {
	return &Refusal{"refs", "The review folder is outside a Git worktree.", "Open a Git worktree or choose a local session in one."}
}

func missingBase() *Refusal {
	return &Refusal{"refs", "The review needs a base commit.", "Name a base to compare with the review commit."}
}
