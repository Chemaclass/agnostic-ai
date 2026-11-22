package mailbox

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

type pendingTransfer struct {
	Thread string            `json:"thread"`
	N      int               `json:"n"`
	From   conversations.Ref `json:"from"`
	To     conversations.Ref `json:"to"`
	Gives
	IgnoreCase bool      `json:"ignoreCase,omitempty"`
	At         time.Time `json:"at"`
	By         string    `json:"by,omitempty"`
}

func (move pendingTransfer) actor() string {
	if move.By == "" {
		return "human"
	}
	return move.By
}

func (move pendingTransfer) valid() bool {
	return threadID.MatchString(move.Thread) && move.N > 0 &&
		supportedRef(move.From) && supportedRef(move.To) && move.From != move.To &&
		!move.At.IsZero() && (move.By == "" || knownActor(move.By)) && validateClaimPaths(move.Gives) == nil
}

func firstUnownedScope(claims []Claim, owner conversations.Ref, scope Gives) (string, bool) {
	for _, named := range scope.Paths {
		if !ownerCovers(claims, owner, scope.Worktree, named) {
			return named, true
		}
	}
	return "", false
}

func (s Store) handoffRefusal(owner conversations.Ref, scope Gives) (*Refusal, error) {
	state, err := s.readClaims()
	if err != nil {
		return nil, err
	}
	if named, failed := firstUnownedScope(state.Claims, owner, scope); failed {
		return &Refusal{"ownership", fmt.Sprintf("The sender does not hold %q.", named), "Claim the paths before offering the handoff."}, nil
	}
	return nil, nil
}

func (s Store) AnswerHandoff(ctx context.Context, id string, n int, action string) error {
	return s.answerHandoff(ctx, nil, id, n, action)
}

// AnswerHandoffFromAgent lets only the session the handoff was offered to answer it.
func (s Store) AnswerHandoffFromAgent(ctx context.Context, actor conversations.Ref, id string, n int, action string) error {
	if refused := ValidateRef(actor); refused != nil {
		return refused
	}
	return s.answerHandoff(ctx, &actor, id, n, action)
}

func answererRefusal(thread Thread, m *Message, actor *conversations.Ref) *Refusal {
	switch {
	case actor == nil:
		return nil
	case !thread.includes(*actor):
		return NewRefusal("not-in-thread")
	case *actor != m.To:
		return &Refusal{"ownership", "Only the session this handoff was offered to can answer it.", "Wait for that session to accept or decline it."}
	case m.State != Delivered && m.State != Staged:
		return &Refusal{"format", "This handoff has not reached this session yet.", "Answer it after it arrives with a prompt."}
	}
	return nil
}

func (s Store) answerHandoff(ctx context.Context, actor *conversations.Ref, id string, n int, action string) error {
	if refused := checkStrings(id, action); refused != nil {
		return refused
	}
	by := "human"
	if actor != nil {
		by = "agent"
	}
	if action != "accept" && action != "decline" {
		return NewRefusal("format")
	}
	var scope Gives
	var fold bool
	if action == "accept" {
		// Git runs before the lock so a slow repository does not block other mailbox calls.
		thread, offered, err := s.loadMessage(id, n)
		if err != nil {
			return err
		}
		if refused := answererRefusal(thread, offered, actor); refused != nil {
			return errors.Join(refused, s.RecordRefusal(ctx, *actor, SendRequest{Type: Handoff, Thread: id}, refused))
		}
		if offered.Type != Handoff || offered.Gives == nil {
			return NewRefusal("format")
		}
		if refused := awaitingApproval(offered); refused != nil {
			return refused
		}
		if offered.Handoff == Offered {
			scope, fold, err = normalizeClaimPaths(ctx, offered.Gives.Worktree, *offered.Gives)
			if err != nil {
				return err
			}
		}
	}
	release, err := s.lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	thread, message, err := s.loadMessage(id, n)
	if err != nil {
		return err
	}
	if refused := answererRefusal(thread, message, actor); refused != nil {
		return errors.Join(refused, s.recordRefusal(time.Now().UTC(), *actor, SendRequest{Type: Handoff, Thread: id}, refused, thread))
	}
	if message.Type != Handoff || message.Gives == nil {
		return NewRefusal("format")
	}
	if refused := awaitingApproval(message); refused != nil {
		return refused
	}
	if action == "accept" && message.Handoff == Accepted || action == "decline" && message.Handoff == Declined {
		return nil
	}
	if message.Handoff != Offered {
		return &Refusal{"ownership", "This handoff already has an answer.", "Send a new offer to move these paths again."}
	}
	if thread.Status == Paused {
		return NewRefusal("paused")
	}
	if action == "decline" {
		message.Handoff = Declined
		thread.addEventBy(time.Now().UTC(), by, "decline", n)
		return s.writeThread(thread)
	}
	if thread.Status == Closed {
		return NewRefusal("closed")
	}
	state, err := s.readClaims()
	if err != nil {
		return err
	}
	if !claimPathEqual(scope.Worktree, message.Gives.Worktree, fold) ||
		!slices.EqualFunc(scope.Paths, message.Gives.Paths, func(a, b string) bool { return claimPathEqual(a, b, fold) }) ||
		!resolvesUnchanged(scope, fold) {
		return &Refusal{"ownership", "The offered paths now resolve to a different location.", "Review the paths and send a new handoff."}
	}

	from, to := message.From.Ref, message.To
	if named, failed := firstUnownedScope(state.Claims, from, scope); failed {
		conflict := ClaimConflict{Worktree: scope.Worktree, Path: named, Requester: to, Sender: &from, Thread: id, N: n}
		if at, holder := claimedByOther(state.Claims, from, scope.Worktree, named); holder != nil {
			conflict.Path, conflict.Holder = at, holder
		}
		refused := s.recordClaimConflict(state, conflict)
		if _, ok := errors.AsType[*Refusal](refused); ok && !slices.ContainsFunc(thread.Events, func(e Event) bool { return e.Action == "conflict" && e.N == n }) {
			thread.addEventBy(time.Now().UTC(), by, "conflict", n)
			if err := s.writeThread(thread); err != nil {
				return err
			}
		}
		return refused
	}
	move := pendingTransfer{Thread: id, N: n, From: from, To: to, Gives: *message.Gives, IgnoreCase: fold, At: time.Now().UTC(), By: by}
	return writeTransfer(state, thread, move, s.writeClaims, s.writeThread)
}

func awaitingApproval(m *Message) *Refusal {
	switch m.State {
	case Held:
		return &Refusal{"format", "This handoff is waiting for approval.", "Approve the handoff in the companion first."}
	case Rejected:
		return NewRefusal("rejected")
	default:
		return nil
	}
}

// resolvesUnchanged repeats the file checks under the lock, without Git, so a
// symlink swapped while acceptance waited for the lock is caught.
func resolvesUnchanged(scope Gives, fold bool) bool {
	for _, named := range scope.Paths {
		again, err := resolveClaimPath(scope.Worktree, named)
		if err != nil || !claimPathEqual(again, named, fold) {
			return false
		}
	}
	return true
}

func (s Store) recoverTransfer() error {
	state, err := s.readClaims()
	if _, ok := errors.AsType[*fs.PathError](err); ok {
		return err
	}
	// A corrupt claims file fails the claim operations that read it, not thread operations.
	if err != nil || state.Pending == nil {
		return nil
	}
	move := *state.Pending
	_, message, err := s.loadMessage(move.Thread, move.N)
	var refused *Refusal
	if errors.As(err, &refused) && refused.Guard == "not-in-thread" {
		state.Pending = nil
		return s.writeClaims(state)
	}
	if err != nil {
		return fmt.Errorf("recover transfer %s message %d: %w", move.Thread, move.N, err)
	}
	if err := move.checkMessage(message); err != nil {
		return err
	}
	if message.Handoff == Accepted {
		if err := finishTransfer(&state, move); err != nil {
			return fmt.Errorf("recover transfer %s message %d: %w", move.Thread, move.N, err)
		}
	}
	state.Pending = nil
	return s.writeClaims(state)
}

func writeTransfer(state claimsFile, thread Thread, move pendingTransfer, saveClaims func(claimsFile) error, saveThread func(Thread) error) error {
	message, refused := thread.message(move.N)
	if refused != nil {
		return refused
	}
	if err := move.checkMessage(message); err != nil {
		return err
	}
	// Check the result before placing the journal so no impossible transfer starts.
	next := state
	if err := finishTransfer(&next, move); err != nil {
		return err
	}
	state.Pending = &move
	if err := saveClaims(state); err != nil {
		return err
	}
	message.Handoff = Accepted
	message.AcceptedAt = &move.At
	message.AcceptedBy = &move.To
	thread.addEventBy(move.At, move.actor(), "accept", move.N)
	if err := saveThread(thread); err != nil {
		return err
	}
	return saveClaims(next)
}

func (move pendingTransfer) checkMessage(message *Message) error {
	if message.Type != Handoff || message.Gives == nil || message.From.Ref != move.From || message.To != move.To ||
		message.Gives.Worktree != move.Worktree || !slices.Equal(message.Gives.Paths, move.Paths) {
		return fmt.Errorf("recover transfer %s message %d: recorded offer differs from the journal", move.Thread, move.N)
	}
	if message.Handoff == Accepted && (message.AcceptedAt == nil || !message.AcceptedAt.Equal(move.At) ||
		message.AcceptedBy == nil || *message.AcceptedBy != move.To) {
		return fmt.Errorf("recover transfer %s message %d: acceptance differs from the journal", move.Thread, move.N)
	}
	return nil
}

func finishTransfer(state *claimsFile, move pendingTransfer) error {
	if _, failed := firstUnownedScope(state.Claims, move.From, move.Gives); failed {
		return errors.New("the original owner no longer covers the pending transfer")
	}
	for _, named := range move.Paths {
		state.Claims, _ = removeScope(state.Claims, move.From, move.Worktree, named)
	}
	state.Claims = addScopes(state.Claims, move.To, move.Gives, move.IgnoreCase, move.At, move.actor())
	state.Conflicts = slices.DeleteFunc(slices.Clone(state.Conflicts), func(c ClaimConflict) bool {
		return c.Thread == move.Thread && c.N == move.N
	})
	retireUnblocked(state)
	state.Pending = nil
	return nil
}
