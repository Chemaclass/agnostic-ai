package mailbox

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

const claimsVersion = "mailbox-claims/v1"

type Claim struct {
	Owner      conversations.Ref `json:"owner"`
	Worktree   string            `json:"worktree"`
	Paths      []string          `json:"paths"`
	Except     []string          `json:"except,omitempty"`
	Since      time.Time         `json:"since"`
	By         string            `json:"by"`
	IgnoreCase bool              `json:"ignoreCase,omitempty"`
	Stale      bool              `json:"stale,omitempty"`
}

type ClaimConflict struct {
	Worktree  string             `json:"worktree"`
	Path      string             `json:"path"`
	Requester conversations.Ref  `json:"requester"`
	Holder    *conversations.Ref `json:"holder,omitempty"`
	Sender    *conversations.Ref `json:"sender,omitempty"`
	Thread    string             `json:"thread,omitempty"`
	N         int                `json:"n,omitempty"`
}

type ClaimView struct {
	Claims    []Claim         `json:"claims"`
	Conflicts []ClaimConflict `json:"conflicts"`
}

type claimsFile struct {
	SchemaVersion string           `json:"schemaVersion"`
	Claims        []Claim          `json:"claims"`
	Pending       *pendingTransfer `json:"pending"`
	Conflicts     []ClaimConflict  `json:"conflicts,omitempty"`
}

func (s Store) claimsPath() string { return filepath.Join(s.Dir, "claims.json") }

func (s Store) readClaims() (claimsFile, error) {
	file := s.claimsPath()
	var state claimsFile
	if err := readJSON(file, &state); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return claimsFile{SchemaVersion: claimsVersion, Claims: []Claim{}}, nil
		}
		return claimsFile{}, err
	}
	if state.SchemaVersion != claimsVersion {
		return claimsFile{}, fmt.Errorf("parse %s: unsupported claims schema", file)
	}
	for _, claim := range state.Claims {
		if !claim.valid() {
			return claimsFile{}, fmt.Errorf("parse %s: invalid path claim", file)
		}
	}
	if state.Pending != nil && !state.Pending.valid() {
		return claimsFile{}, fmt.Errorf("parse %s: invalid pending transfer", file)
	}
	return state, nil
}

func knownActor(by string) bool {
	return by == "human" || by == "agent"
}

func (c Claim) valid() bool {
	if len(c.Paths) != 1 || !knownActor(c.By) || !supportedRef(c.Owner) || c.Since.IsZero() ||
		validateClaimPaths(Gives{Worktree: c.Worktree, Paths: c.Paths}) != nil {
		return false
	}
	for _, hole := range c.Except {
		if claimPathEqual(c.Paths[0], hole, c.IgnoreCase) || !pathInScope(c.Paths[0], hole, c.IgnoreCase) ||
			validateClaimPaths(Gives{Worktree: c.Worktree, Paths: []string{hole}}) != nil {
			return false
		}
	}
	return true
}

func (s Store) writeClaims(state claimsFile) error {
	return writeJSON(s.claimsPath(), state)
}

func (s Store) Claim(ctx context.Context, owner conversations.Ref, gives Gives, sourceFolder, by string) error {
	if refused := ValidateRef(owner); refused != nil {
		return refused
	}
	if !knownActor(by) {
		return NewRefusal("format")
	}
	scope, fold, err := normalizeClaimPaths(ctx, sourceFolder, gives)
	if err != nil {
		return err
	}
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	state, err := s.readClaims()
	if err != nil {
		return err
	}
	for _, named := range scope.Paths {
		if overlap, holder := claimedByOther(state.Claims, owner, scope.Worktree, named); holder != nil {
			conflict := ClaimConflict{Worktree: scope.Worktree, Path: overlap, Requester: owner, Holder: holder}
			return s.recordClaimConflict(state, conflict)
		}
	}
	state.Claims = addScopes(state.Claims, owner, scope, fold, time.Now().UTC(), by)
	return s.writeClaims(state)
}

func (s Store) Release(ctx context.Context, owner conversations.Ref, gives Gives) error {
	if refused := ValidateRef(owner); refused != nil {
		return refused
	}
	if refused := validateClaimPaths(gives); refused != nil {
		return refused
	}
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	state, err := s.readClaims()
	if err != nil {
		return err
	}
	idx := indexClaimWorktree(state.Claims, owner, filepath.Clean(gives.Worktree))
	if idx < 0 {
		canonical, err := filepath.EvalSymlinks(gives.Worktree)
		if err != nil {
			return nil
		}
		idx = indexClaimWorktree(state.Claims, owner, canonical)
		if idx < 0 {
			return nil
		}
	}
	known := state.Claims[idx]
	paths, err := releasePaths(state.Claims, known, gives.Paths)
	if err != nil {
		return err
	}
	changed := false
	for _, named := range paths {
		var cut bool
		state.Claims, cut = removeScope(state.Claims, owner, known.Worktree, named)
		changed = changed || cut
	}
	if !changed {
		return nil
	}
	return s.writeClaims(state)
}

func indexClaimWorktree(claims []Claim, owner conversations.Ref, worktree string) int {
	return slices.IndexFunc(claims, func(c Claim) bool {
		return c.Owner == owner && c.inWorktree(worktree)
	})
}

func releasePaths(claims []Claim, known Claim, requested []string) ([]string, error) {
	info, err := os.Lstat(known.Worktree)
	stale := errors.Is(err, fs.ErrNotExist) || err == nil && !info.IsDir()
	if err != nil && !stale {
		return nil, err
	}
	paths := make([]string, 0, len(requested))
	for _, named := range requested {
		named = filepath.ToSlash(named)
		// A displayed claim keeps its original identity even if the path is now a symlink.
		if !stale && !hasRecordedClaimPath(claims, known, named) {
			canonical, err := resolveClaimPath(known.Worktree, named)
			if err != nil {
				return nil, err
			}
			named = canonical
		}
		paths = append(paths, named)
	}
	return paths, nil
}

func hasRecordedClaimPath(claims []Claim, known Claim, named string) bool {
	for _, c := range claims {
		if c.Owner == known.Owner && c.inWorktree(known.Worktree) &&
			(claimPathEqual(c.Paths[0], named, c.IgnoreCase) ||
				slices.ContainsFunc(c.Except, func(hole string) bool { return claimPathEqual(hole, named, c.IgnoreCase) })) {
			return true
		}
	}
	return false
}

func (s Store) Claims(ctx context.Context) (ClaimView, error) {
	release, present, err := s.lockForRead(ctx)
	if err != nil {
		return ClaimView{}, err
	}
	if !present {
		return emptyClaimView(), nil
	}
	defer release()
	return s.readClaimView()
}

func emptyClaimView() ClaimView {
	return ClaimView{Claims: []Claim{}, Conflicts: []ClaimConflict{}}
}

func (s Store) readClaimView() (ClaimView, error) {
	state, err := s.readClaims()
	if err != nil {
		return ClaimView{}, err
	}
	view := emptyClaimView()
	view.Claims = append(view.Claims, state.Claims...)
	for _, conflict := range state.Conflicts {
		if current, active := s.activeConflict(state.Claims, conflict); active {
			view.Conflicts = append(view.Conflicts, current)
		}
	}
	return view, nil
}

func (s Store) ClaimsOf(ctx context.Context, owner conversations.Ref) (ClaimView, error) {
	view, err := s.Claims(ctx)
	if err != nil {
		return ClaimView{}, err
	}
	view.Claims = slices.DeleteFunc(view.Claims, func(c Claim) bool { return c.Owner != owner })
	view.Conflicts = slices.DeleteFunc(view.Conflicts, func(c ClaimConflict) bool {
		return c.Requester != owner && (c.Holder == nil || *c.Holder != owner) && (c.Sender == nil || *c.Sender != owner)
	})
	return view, nil
}

func (s Store) activeConflict(claims []Claim, c ClaimConflict) (ClaimConflict, bool) {
	if c.Sender != nil {
		thread, message, err := s.loadMessage(c.Thread, c.N)
		if err != nil || thread.Status == Closed || message.Handoff != Offered {
			return c, false
		}
		if scope, failed := firstUnownedScope(claims, *c.Sender, *message.Gives); failed {
			c.Path = scope
			if named, holder := claimedByOther(claims, *c.Sender, c.Worktree, scope); holder != nil {
				c.Path, c.Holder = named, holder
			} else {
				c.Holder = nil
			}
			return c, true
		}
		return c, false
	}
	named, holder := claimedByOther(claims, c.Requester, c.Worktree, c.Path)
	if holder == nil {
		return c, false
	}
	c.Path, c.Holder = named, holder
	return c, true
}

func (s Store) recordClaimConflict(state claimsFile, c ClaimConflict) error {
	state.Conflicts = slices.DeleteFunc(state.Conflicts, func(record ClaimConflict) bool {
		if c.Thread != "" {
			return record.Thread == c.Thread && record.N == c.N
		}
		return record.Thread == "" && record.Requester == c.Requester && record.Worktree == c.Worktree && record.Path == c.Path
	})
	state.Conflicts = append(state.Conflicts, c)
	if err := s.writeClaims(state); err != nil {
		return err
	}
	if c.Holder == nil {
		return &Refusal{"ownership", fmt.Sprintf("The sender no longer holds %q.", c.Path), "Have the sender claim the path again before accepting."}
	}
	return &Refusal{"ownership", fmt.Sprintf("Path %q is held by %s session %s.", c.Path, c.Holder.Tool, c.Holder.ID),
		"Choose an available path or ask that session to release it."}
}
