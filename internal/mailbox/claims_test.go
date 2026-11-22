package mailbox

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

var mailThird = conversations.Ref{Tool: conversations.ToolCodex, ID: "77998a87-7f00-4c06-bc0c-b5fbf693eb76"}

func claimRepo(t *testing.T) string {
	t.Helper()
	repo, _ := newReviewRepo(t)
	root, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func claimAndOffer(t *testing.T, s Store, root string, paths ...string) Receipt {
	t.Helper()
	if err := s.Claim(t.Context(), claude, Gives{root, []string{"scope"}}, root, "human"); err != nil {
		t.Fatal(err)
	}
	return offerPaths(t, s, root, paths...)
}

func offerPaths(t *testing.T, s Store, root string, paths ...string) Receipt {
	t.Helper()
	gives := Gives{root, paths}
	receipt, err := s.SendFromWorktree(t.Context(), claude, SendRequest{
		To: codex, Type: Handoff, Body: "See the handoff note.", Gives: &gives,
	}, root)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func TestClaims_TransfersOneChildAndReleaseKeepsItFree(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	offer := claimAndOffer(t, s, root, "scope/a.go")
	c := onlyThread(t, s)
	if c.Messages[0].Handoff != Offered {
		t.Errorf("new handoff = %q", c.Messages[0].Handoff)
	}
	expectedOwner(t, s, root, "scope/a.go", claude)
	if err := s.AnswerHandoff(t.Context(), offer.Thread, offer.N, "accept"); err != nil {
		t.Fatal(err)
	}
	if err := s.AnswerHandoff(t.Context(), offer.Thread, offer.N, "accept"); err != nil {
		t.Errorf("retry acceptance: %v", err)
	}
	expectedOwner(t, s, root, "scope/a.go", codex)
	expectedOwner(t, s, root, "scope/b.go", claude)
	view, err := s.ClaimsOf(t.Context(), claude)
	if err != nil || len(view.Claims) != 1 || !slices.Equal(view.Claims[0].Except, []string{"scope/a.go"}) {
		t.Errorf("sender's remaining scopes = %+v, %v", view, err)
	}
	if err := s.Release(t.Context(), codex, Gives{root, []string{"scope/a.go"}}); err != nil {
		t.Fatal(err)
	}
	expectedOwner(t, s, root, "scope/a.go", conversations.Ref{})
	if err := s.Claim(t.Context(), claude, Gives{root, []string{"scope/a.go"}}, root, "human"); err != nil {
		t.Fatal(err)
	}
	second := offerPaths(t, s, root, "scope")
	if err := s.AnswerHandoff(t.Context(), second.Thread, second.N, "accept"); err != nil {
		t.Fatal(err)
	}
	expectedOwner(t, s, root, "scope/b.go", codex)
}

func TestClaims_ReleasesThroughTheSameWorktreeAlias(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	alias := filepath.Join(t.TempDir(), "worktree")
	claimSymlink(t, root, alias)
	gives := Gives{alias, []string{"scope/file.go"}}
	if err := s.Claim(t.Context(), claude, gives, root, "human"); err != nil {
		t.Fatal(err)
	}
	if err := s.Release(t.Context(), claude, gives); err != nil {
		t.Fatal(err)
	}
	expectedOwner(t, s, root, "scope/file.go", conversations.Ref{})
}

func TestClaims_AConflictAtAcceptanceMovesNothingAndShowsBothSides(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	offer := claimAndOffer(t, s, root, "scope/a.go", "scope/b.go")
	file := Gives{root, []string{"scope/a.go"}}
	if err := s.Release(t.Context(), claude, file); err != nil {
		t.Fatal(err)
	}
	if err := s.Claim(t.Context(), mailThird, file, root, "human"); err != nil {
		t.Fatal(err)
	}
	err := s.AnswerHandoff(t.Context(), offer.Thread, offer.N, "accept")
	requireGuard(t, err, "ownership")
	expectedOwner(t, s, root, "scope/a.go", mailThird)
	expectedOwner(t, s, root, "scope/b.go", claude)
	for _, session := range []conversations.Ref{claude, codex} {
		view, err := s.ClaimsOf(t.Context(), session)
		if err != nil || len(view.Conflicts) != 1 || view.Conflicts[0].Holder == nil ||
			*view.Conflicts[0].Holder != mailThird || view.Conflicts[0].Path != "scope/a.go" {
			t.Errorf("conflict for %+v = %+v, %v", session, view, err)
		}
	}
}

func TestClaims_CompetingOffersTransferOneTime(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	first := claimAndOffer(t, s, root, "scope/a.go")
	second := offerPaths(t, s, root, "scope/a.go")
	results := make(chan error, 2)
	var group sync.WaitGroup
	for _, offer := range []Receipt{first, second} {
		group.Go(func() { results <- s.AnswerHandoff(t.Context(), offer.Thread, offer.N, "accept") })
	}
	group.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else {
			requireGuard(t, err, "ownership")
		}
	}
	if accepted != 1 {
		t.Errorf("accepted %d offers, want 1", accepted)
	}
	expectedOwner(t, s, root, "scope/a.go", codex)
}

func TestClaims_RepairsEachInterruptedWriteAndRetriesAcceptance(t *testing.T) {
	for failAfter := 1; failAfter <= 3; failAfter++ {
		t.Run(fmt.Sprintf("write %d", failAfter), func(t *testing.T) {
			s := Store{Dir: t.TempDir()}
			root := claimRepo(t)
			offer := claimAndOffer(t, s, root, "scope/a.go")
			thread := onlyThread(t, s)
			state, err := s.readClaims()
			if err != nil {
				t.Fatal(err)
			}
			move := pendingTransfer{Thread: offer.Thread, N: offer.N, From: claude, To: codex,
				Gives: *thread.Messages[0].Gives, IgnoreCase: state.Claims[0].IgnoreCase, At: time.Now().UTC()}
			remaining := failAfter
			after := func(err error) error {
				if err != nil {
					return err
				}
				remaining--
				if remaining == 0 {
					return os.ErrPermission
				}
				return nil
			}
			err = writeTransfer(state, thread, move, func(state claimsFile) error {
				return after(s.writeClaims(state))
			}, func(thread Thread) error {
				return after(s.writeThread(thread))
			})
			if !errors.Is(err, os.ErrPermission) {
				t.Fatalf("interrupted transfer = %v", err)
			}
			threads, view, err := s.Snapshot(t.Context())
			if err != nil || len(threads.Threads) != 1 {
				t.Fatalf("snapshot after recovery = %+v, %+v, %v", threads, view, err)
			}
			wantOwner, wantStatus := codex, Accepted
			if failAfter == 1 {
				wantOwner, wantStatus = claude, Offered
			}
			if got := threads.Threads[0].Messages[0].Handoff; got != wantStatus {
				t.Errorf("snapshot saw handoff %q, want %q", got, wantStatus)
			}
			expectedOwnerIn(t, view, root, "scope/a.go", wantOwner)
			expectedOwnerIn(t, view, root, "scope/b.go", claude)
			state, err = s.readClaims()
			if err != nil || state.Pending != nil {
				t.Errorf("recovery left pending journal: %+v, %v", state.Pending, err)
			}
			if err := s.AnswerHandoff(t.Context(), offer.Thread, offer.N, "accept"); err != nil {
				t.Errorf("retry acceptance: %v", err)
			}
			expectedOwner(t, s, root, "scope/a.go", codex)
		})
	}
}

func TestClaims_ReleasesStaleScopeAfterWorktreeDisappears(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	gives := Gives{root, []string{"scope"}}
	if err := s.Claim(t.Context(), claude, gives, root, "human"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(root, filepath.Join(filepath.Dir(root), "moved")); err != nil {
		t.Fatal(err)
	}
	if err := s.Release(t.Context(), claude, gives); err != nil {
		t.Fatal(err)
	}
	expectedOwner(t, s, root, "scope/a.go", conversations.Ref{})
}

func expectedOwner(t *testing.T, s Store, root, path string, want conversations.Ref) {
	t.Helper()
	view, err := s.Claims(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	expectedOwnerIn(t, view, root, path, want)
}

func expectedOwnerIn(t *testing.T, view ClaimView, root, path string, want conversations.Ref) {
	t.Helper()
	var owners []conversations.Ref
	for _, claim := range view.Claims {
		if claim.Worktree == root && claim.owns(path) {
			owners = append(owners, claim.Owner)
		}
	}
	if want.ID == "" && len(owners) != 0 || want.ID != "" && (len(owners) != 1 || owners[0] != want) {
		t.Errorf("owners of %s = %+v, want %+v", path, owners, want)
	}
}

func TestClaims_ReleaseDoesNotRequireGitMetadata(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	if err := s.Claim(t.Context(), claude, Gives{root, []string{"scope"}}, root, "human"); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, ".git"), filepath.Join(root, "removed-git")); err != nil {
		t.Fatal(err)
	}
	if err := s.Release(t.Context(), claude, Gives{root, []string{"scope/a.go"}}); err != nil {
		t.Fatal(err)
	}
	expectedOwner(t, s, root, "scope/a.go", conversations.Ref{})
	expectedOwner(t, s, root, "scope/b.go", claude)
}

func TestClaims_ReleaseOfStoredPathDoesNotFollowRetargetedSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks can require Windows privileges")
	}
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	if err := s.Claim(t.Context(), claude, Gives{root, []string{"a.go", "b.go"}}, root, "human"); err != nil {
		t.Fatal(err)
	}
	writeReviewFile(t, root, "b.go", "keep this claim")
	if err := os.Symlink("b.go", filepath.Join(root, "a.go")); err != nil {
		t.Fatal(err)
	}
	if err := s.Release(t.Context(), claude, Gives{root, []string{"a.go"}}); err != nil {
		t.Fatal(err)
	}
	expectedOwner(t, s, root, "a.go", conversations.Ref{})
	expectedOwner(t, s, root, "b.go", claude)
}

func TestClaims_DeclineAndStopKeepOwnership(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	offer := claimAndOffer(t, s, root, "scope/a.go")
	err := s.Claim(t.Context(), codex, Gives{root, []string{"scope/a.go"}}, root, "human")
	requireGuard(t, err, "ownership")
	for range 2 {
		if err := s.AnswerHandoff(t.Context(), offer.Thread, offer.N, "decline"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Change(t.Context(), offer.Thread, "stop"); err != nil {
		t.Fatal(err)
	}
	thread := onlyThread(t, s)
	if thread.Messages[0].Handoff != Declined {
		t.Errorf("declined handoff = %q", thread.Messages[0].Handoff)
	}
	expectedOwner(t, s, root, "scope/a.go", claude)
}

func TestClaims_HandoffChecksOwnershipAndCredentials(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	for _, scenario := range []struct {
		name  string
		paths []string
		guard string
	}{
		{"no paths", nil, "ownership"},
		{"unowned path", []string{"unowned.txt"}, "ownership"},
		{"credential path", []string{"api_key=example-secret-sentinel"}, "secret"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			_, err := s.SendFromWorktree(t.Context(), claude, SendRequest{
				To: codex, Type: Handoff, Body: "See the handoff note.", Gives: &Gives{root, scenario.paths},
			}, root)
			requireGuard(t, err, scenario.guard)
		})
	}
}

func TestClaims_HandoffCountsTowardTheCapAndRespectsPauseAndClose(t *testing.T) {
	ctx := t.Context()
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	offer := claimAndOffer(t, s, root, "scope/a.go")
	gives := Gives{root, []string{"scope/a.go"}}
	request := SendRequest{To: codex, Type: Handoff, Body: "See the handoff note.", Gives: &gives, Thread: offer.Thread}
	if err := s.Change(ctx, offer.Thread, "pause"); err != nil {
		t.Fatal(err)
	}
	_, err := s.SendFromWorktree(ctx, claude, request, root)
	requireGuard(t, err, "paused")
	requireGuard(t, s.AnswerHandoff(ctx, offer.Thread, offer.N, "accept"), "paused")
	if err := s.Change(ctx, offer.Thread, "resume"); err != nil {
		t.Fatal(err)
	}
	for range 7 {
		sendQuestion(t, s, offer.Thread)
	}
	_, err = s.SendFromWorktree(ctx, claude, request, root)
	requireGuard(t, err, "cap")
	if err := s.Change(ctx, offer.Thread, "stop"); err != nil {
		t.Fatal(err)
	}
	_, err = s.SendFromWorktree(ctx, claude, request, root)
	requireGuard(t, err, "closed")
	requireGuard(t, s.AnswerHandoff(ctx, offer.Thread, offer.N, "accept"), "closed")
	expectedOwner(t, s, root, "scope/a.go", claude)
	if err := s.AnswerHandoff(ctx, offer.Thread, offer.N, "decline"); err != nil {
		t.Fatalf("decline on a closed thread: %v", err)
	}
	if got := onlyThread(t, s).Messages[0].Handoff; got != Declined {
		t.Errorf("handoff after decline = %q", got)
	}
}

func TestClaims_CorruptClaimsFileLeavesThreadsWorking(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	sendQuestion(t, s, "")
	if err := os.WriteFile(s.claimsPath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	sendQuestion(t, s, "")
	if _, err := s.Threads(t.Context()); err != nil {
		t.Fatalf("threads with a corrupt claims file: %v", err)
	}
	if _, err := s.Claims(t.Context()); err == nil {
		t.Error("claims read a corrupt file without an error")
	}
}

func TestClaims_PendingTransferOfAMissingThreadIsCleared(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	if err := s.Claim(t.Context(), claude, Gives{root, []string{"scope"}}, root, "human"); err != nil {
		t.Fatal(err)
	}
	state, err := s.readClaims()
	if err != nil {
		t.Fatal(err)
	}
	state.Pending = &pendingTransfer{
		Thread: "th_20261121_7f3a9c10", N: 1, From: claude, To: codex,
		Gives: Gives{root, []string{"scope"}}, At: time.Now().UTC(),
	}
	if err := s.writeClaims(state); err != nil {
		t.Fatal(err)
	}
	expectedOwner(t, s, root, "scope", claude)
	if state, err := s.readClaims(); err != nil || state.Pending != nil {
		t.Errorf("pending after recovery = %v, %v", state.Pending, err)
	}
}

func TestClaims_ListReadsDoNotWaitForABusyLock(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	sendQuestion(t, s, "")
	release, err := s.lock(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, _, err := s.Snapshot(t.Context()); err != nil {
		t.Fatalf("snapshot while the lock is held: %v", err)
	}
}

func TestClaims_StoresUnicodePathsInOneNormalForm(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	if err := s.Claim(t.Context(), claude, Gives{root, []string{"cafe\u0301.go"}}, root, "human"); err != nil {
		t.Fatal(err)
	}
	expectedOwner(t, s, root, "caf\u00e9.go", claude)
	err := s.Claim(t.Context(), codex, Gives{root, []string{"caf\u00e9.go"}}, root, "human")
	requireGuard(t, err, "ownership")
}

func TestClaims_RetryOfAnAcceptedHandoffAfterTheWorktreeLeaves(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	offer := claimAndOffer(t, s, root, "scope/a.go")
	if err := s.AnswerHandoff(t.Context(), offer.Thread, offer.N, "accept"); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	if err := s.AnswerHandoff(t.Context(), offer.Thread, offer.N, "accept"); err != nil {
		t.Fatalf("second accept: %v", err)
	}
}

func TestClaims_AcceptRefusesAPathThatNowResolvesElsewhere(t *testing.T) {
	root := claimRepo(t)
	if err := os.MkdirAll(filepath.Join(root, "other"), 0o700); err != nil {
		t.Fatal(err)
	}
	if !resolvesUnchanged(Gives{root, []string{"scope/a.go"}}, false) {
		t.Fatal("an unchanged path counts as moved")
	}
	if err := os.MkdirAll(filepath.Join(root, "scope"), 0o700); err != nil {
		t.Fatal(err)
	}
	claimSymlink(t, filepath.Join(root, "other", "a.go"), filepath.Join(root, "scope", "a.go"))
	if resolvesUnchanged(Gives{root, []string{"scope/a.go"}}, false) {
		t.Error("a path swapped for a symlink counts as unchanged")
	}
}

func TestClaims_DismissRemovesOnlyTheRequestersConflictWithNoThread(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	root := claimRepo(t)
	offer := claimAndOffer(t, s, root, "scope/a.go")
	file := Gives{root, []string{"scope/a.go"}}
	if err := s.Release(t.Context(), claude, file); err != nil {
		t.Fatal(err)
	}
	if err := s.Claim(t.Context(), mailThird, file, root, "human"); err != nil {
		t.Fatal(err)
	}
	requireGuard(t, s.AnswerHandoff(t.Context(), offer.Thread, offer.N, "accept"), "ownership")
	requireGuard(t, s.Claim(t.Context(), claude, Gives{root, []string{"scope"}}, root, "human"), "ownership")
	conflicts := func() []ClaimConflict {
		view, err := s.Claims(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return view.Conflicts
	}
	if got := conflicts(); len(got) != 2 {
		t.Fatalf("conflicts before dismiss = %+v", got)
	}

	for _, unknown := range []struct {
		requester conversations.Ref
		path      string
	}{{codex, "scope/a.go"}, {claude, "scope/b.go"}} {
		if err := s.DismissConflict(t.Context(), unknown.requester, root, unknown.path); err != nil {
			t.Fatal(err)
		}
	}
	if got := conflicts(); len(got) != 2 {
		t.Errorf("an unknown or threaded conflict was dismissed: %+v", got)
	}
	if err := s.DismissConflict(t.Context(), claude, root, "scope/a.go"); err != nil {
		t.Fatal(err)
	}
	if got := conflicts(); len(got) != 1 || got[0].Thread != offer.Thread {
		t.Errorf("after dismiss = %+v, want only the handoff conflict", got)
	}
}
