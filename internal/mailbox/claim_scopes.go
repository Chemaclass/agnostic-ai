package mailbox

import (
	"slices"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

func (c Claim) inWorktree(root string) bool {
	return claimPathEqual(c.Worktree, root, c.IgnoreCase)
}

func (c Claim) owns(path string) bool {
	return pathInScope(c.Paths[0], path, c.IgnoreCase) && !slices.ContainsFunc(c.Except, func(except string) bool {
		return pathInScope(except, path, c.IgnoreCase)
	})
}

func (c Claim) overlap(path string) string {
	root := c.Paths[0]
	if pathInScope(root, path, c.IgnoreCase) {
		if c.owns(path) {
			return path
		}
	} else if pathInScope(path, root, c.IgnoreCase) {
		return root
	}
	return ""
}

func claimedByOther(claims []Claim, actor conversations.Ref, root, path string) (string, *conversations.Ref) {
	for _, c := range claims {
		if c.Owner != actor && c.inWorktree(root) {
			if at := c.overlap(path); at != "" {
				owner := c.Owner
				return at, &owner
			}
		}
	}
	return "", nil
}

func ownerCovers(claims []Claim, owner conversations.Ref, root, path string) bool {
	for _, c := range claims {
		if c.Owner != owner || !c.inWorktree(root) || !c.owns(path) {
			continue
		}
		for _, hole := range c.Except {
			if pathInScope(path, hole, c.IgnoreCase) && !ownerCovers(claims, owner, root, hole) {
				return false
			}
		}
		return true
	}
	return false
}

func removeScope(claims []Claim, owner conversations.Ref, root, path string) ([]Claim, bool) {
	retained := make([]Claim, 0, len(claims))
	changed := false
	for _, c := range claims {
		if c.Owner == owner && c.inWorktree(root) && c.overlap(path) != "" {
			changed = true
			if pathInScope(path, c.Paths[0], c.IgnoreCase) {
				continue
			}
			holes := slices.Clone(c.Except)
			holes = slices.DeleteFunc(holes, func(hole string) bool { return pathInScope(path, hole, c.IgnoreCase) })
			holes = append(holes, path)
			slices.Sort(holes)
			c.Except = holes
		}
		retained = append(retained, c)
	}
	return retained, changed
}

func addScopes(claims []Claim, owner conversations.Ref, scope Gives, fold bool, at time.Time, by string) []Claim {
	for _, path := range scope.Paths {
		if ownerCovers(claims, owner, scope.Worktree, path) {
			continue
		}
		claims, _ = removeScope(claims, owner, scope.Worktree, path)
		claims = append(claims, Claim{
			Owner: owner, Worktree: scope.Worktree, Paths: []string{path},
			Since: at, By: by, IgnoreCase: fold,
		})
	}
	return claims
}
