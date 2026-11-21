package mailbox

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/chemaclass/agnostic-ai/internal/filealias"
)

const (
	maxClaimPaths     = 200
	maxClaimPathBytes = 400
	maxClaimLinkHops  = 40
)

func ResolveClaimWorktree(ctx context.Context, sourceFolder string) (string, error) {
	if refused := checkClaimStrings(sourceFolder); refused != nil {
		return "", refused
	}
	if !filepath.IsAbs(sourceFolder) {
		return "", claimWorktreeRefusal()
	}
	ctx, cancel := context.WithTimeout(ctx, mailboxGitTimeout)
	defer cancel()
	root, err := gitWorktreeRoot(ctx, sourceFolder)
	if err != nil {
		return "", reviewGitFailure(ctx, claimWorktreeRefusal())
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", claimWorktreeRefusal()
	}
	if refused := checkClaimStrings(root); refused != nil {
		return "", refused
	}
	return root, nil
}

func normalizeClaimPaths(ctx context.Context, sourceFolder string, input Gives) (Gives, bool, error) {
	if refused := validateClaimPaths(input); refused != nil {
		return Gives{}, false, refused
	}
	root, err := ResolveClaimWorktree(ctx, sourceFolder)
	if err != nil {
		return Gives{}, false, err
	}
	rootInfo, err := os.Stat(root)
	if err != nil {
		return Gives{}, false, claimWorktreeRefusal()
	}
	candidateInfo, err := os.Stat(input.Worktree)
	if err != nil || !os.SameFile(rootInfo, candidateInfo) {
		return Gives{}, false, claimWorktreeRefusal()
	}
	fold := claimWorktreeIgnoresCase(root)
	paths := make([]string, 0, len(input.Paths))
	for _, supplied := range input.Paths {
		if err := ctx.Err(); err != nil {
			return Gives{}, false, err
		}
		candidate, err := resolveClaimPath(root, supplied)
		if err != nil {
			return Gives{}, false, err
		}
		if !slices.ContainsFunc(paths, func(other string) bool { return claimPathEqual(other, candidate, fold) }) {
			paths = append(paths, candidate)
		}
	}
	slices.Sort(paths)
	gives := Gives{Worktree: root, Paths: paths}
	if refused := validateClaimPaths(gives); refused != nil {
		return Gives{}, false, refused
	}
	return gives, fold, nil
}

func validateClaimPaths(gives Gives) *Refusal {
	if refused := checkClaimStrings(gives.Worktree); refused != nil {
		return refused
	}
	if gives.Worktree == "" || len(gives.Paths) == 0 {
		return &Refusal{"ownership", "A file claim needs a worktree and at least one path.", "Choose a Git worktree and enter its relative paths."}
	}
	if !filepath.IsAbs(gives.Worktree) {
		return claimWorktreeRefusal()
	}
	if len(gives.Paths) > maxClaimPaths {
		return claimPathSizeRefusal()
	}
	for _, value := range gives.Paths {
		if refused := checkClaimStrings(value); refused != nil {
			return refused
		}
		if len(value) > maxClaimPathBytes {
			return claimPathSizeRefusal()
		}
		slash := filepath.ToSlash(value)
		if !filepath.IsLocal(value) || slash != path.Clean(slash) {
			return invalidClaimPath()
		}
	}
	return nil
}

func checkClaimStrings(value string) *Refusal {
	if !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
		return invalidClaimPath()
	}
	return checkStrings(value)
}

func resolveClaimPath(root, relative string) (string, error) {
	at := filepath.Join(root, filepath.FromSlash(relative))
	var missing []string
	for hops := 0; ; {
		info, err := os.Lstat(at)
		if err == nil {
			resolved, err := filepath.EvalSymlinks(at)
			if errors.Is(err, fs.ErrNotExist) && info.Mode()&os.ModeSymlink != 0 && hops < maxClaimLinkHops {
				at, err = futureClaimTarget(root, at)
				if err != nil {
					return "", err
				}
				hops++
				continue
			}
			if err != nil {
				return "", invalidClaimPath()
			}
			if len(missing) > 0 && !info.IsDir() {
				info, err = os.Stat(resolved)
				if err != nil || !info.IsDir() {
					return "", invalidClaimPath()
				}
			}
			slices.Reverse(missing)
			candidate, err := filepath.Rel(root, filepath.Join(append([]string{resolved}, missing...)...))
			if err != nil || !filepath.IsLocal(candidate) {
				return "", invalidClaimPath()
			}
			return filepath.ToSlash(candidate), nil
		}
		if !errors.Is(err, fs.ErrNotExist) || at == root {
			return "", invalidClaimPath()
		}
		missing = append(missing, filepath.Base(at))
		at = filepath.Dir(at)
	}
}

func futureClaimTarget(root, link string) (string, error) {
	target, err := os.Readlink(link)
	if err != nil || filepath.Clean(target) != target {
		return "", invalidClaimPath()
	}
	if !filepath.IsAbs(target) {
		if filepath.VolumeName(target) != "" || os.IsPathSeparator(target[0]) {
			return "", invalidClaimPath()
		}
		parent, err := filepath.EvalSymlinks(filepath.Dir(link))
		if err != nil {
			return "", invalidClaimPath()
		}
		target = filepath.Join(parent, target)
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || !filepath.IsLocal(relative) {
		return "", invalidClaimPath()
	}
	return target, nil
}

func pathInScope(scope, item string, ignoreCase bool) bool {
	if scope == "." {
		return true
	}
	if !ignoreCase {
		return item == scope || strings.HasPrefix(item, scope+"/")
	}
	for segment := range strings.SplitSeq(scope, "/") {
		component, rest, _ := strings.Cut(item, "/")
		if !strings.EqualFold(segment, component) {
			return false
		}
		item = rest
	}
	return true
}

func claimPathEqual(left, right string, ignoreCase bool) bool {
	if ignoreCase {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func claimWorktreeIgnoresCase(root string) bool {
	var aliases filealias.Cache
	match, err := aliases.Match(filepath.Join(root, ".GIT"), nil, []string{filepath.Join(root, ".git")})
	return err == nil && match != ""
}

func claimWorktreeRefusal() *Refusal {
	return &Refusal{"format", "Choose the source session's Git worktree for this claim.", "Open the session from a worktree, then reload the claim form."}
}

func invalidClaimPath() *Refusal {
	return &Refusal{"format", "Each path must stay inside the Git worktree.", "Use a relative path without empty or parent components, or use . for the whole worktree."}
}

func claimPathSizeRefusal() *Refusal {
	return &Refusal{"size", "A claim can have up to 200 paths of 400 bytes each.", "Choose fewer or shorter paths."}
}
