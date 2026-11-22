package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/authority"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

func mailboxClaimBody(id, root, path string) string {
	return fmt.Sprintf(`{"tool":"claude","id":%q,"worktree":%q,"paths":[%q]}`, id, root, path)
}

func testChangeClaim(t *testing.T, h http.Handler, action, id, root, path string) {
	t.Helper()
	rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/"+action, mailboxClaimBody(id, root, path))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("%s = %d %s", action, rec.Code, rec.Body)
	}
}

func mailboxOfferBody(to, root, path string) string {
	return fmt.Sprintf(`{"from":{"tool":"claude","id":%q},"to":{"id":%q},"type":"handoff","body":"See the handoff note.","gives":{"worktree":%q,"paths":[%q]}}`, inProject, to, root, path)
}

func mailboxHandoffBody(thread, action string) string {
	return fmt.Sprintf(`{"thread":%q,"n":1,"action":%q}`, thread, action)
}

type mailboxClaimReplyTest struct {
	Worktree string
	Claims   []struct {
		Owner struct{ ID string }
		Paths []string
		Stale bool
	}
	Conflicts []struct {
		Path   string
		Holder *struct{ ID string }
	}
}

func readTestClaims(t *testing.T, h http.Handler, id string) mailboxClaimReplyTest {
	t.Helper()
	path := "/api/mailbox/claims"
	if id != "" {
		path += "?tool=claude&id=" + id
	}
	rec := mailboxHTTP(t, h, http.MethodGet, path, "")
	var result mailboxClaimReplyTest
	if err := json.Unmarshal(rec.Body.Bytes(), &result); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("GET %s = %d %s (%v)", path, rec.Code, rec.Body, err)
	}
	return result
}

func TestMailboxHTTP_OfferThenAcceptTransfersOnlyTheOfferedPath(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	makeReviewRepo(t, h.root)
	const offered = "src/parser.go"
	const retained = "src/other.go"
	testChangeClaim(t, h, "claim", inProject, h.root, offered)
	testChangeClaim(t, h, "claim", inProject, h.root, retained)
	thread := mailboxSendThread(t, h, mailboxOfferBody(inWorktree, h.root, offered))
	if before := readTestClaims(t, h, inWorktree); len(before.Claims) != 0 {
		t.Errorf("an offer transferred ownership: %#v", before.Claims)
	}
	rec := mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/threads?tool=claude&id="+inWorktree, "")
	if !strings.Contains(rec.Body.String(), `"handoff":"offered"`) {
		t.Errorf("receiver's offered handoff = %d %s", rec.Code, rec.Body)
	}
	rec = mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/handoff", mailboxHandoffBody(thread, "accept"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("accept = %d %s", rec.Code, rec.Body)
	}
	rec = mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/threads?tool=claude&id="+inWorktree, "")
	if !strings.Contains(rec.Body.String(), `"handoff":"accepted"`) || !strings.Contains(rec.Body.String(), `"acceptedAt":`) ||
		!strings.Contains(rec.Body.String(), `"acceptedBy":`) {
		t.Errorf("recipient's accepted handoff = %d %s", rec.Code, rec.Body)
	}
	for id, path := range map[string]string{inProject: retained, inWorktree: offered} {
		got := readTestClaims(t, h, id)
		if len(got.Claims) != 1 || !slices.Equal(got.Claims[0].Paths, []string{path}) || got.Claims[0].Owner.ID != id || got.Claims[0].Stale {
			t.Errorf("claims of %s after acceptance = %#v", id, got.Claims)
		}
	}
	rec = mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/thread", fmt.Sprintf(`{"thread":%q,"action":"stop"}`, thread))
	if rec.Code != http.StatusNoContent {
		t.Errorf("stop = %d %s", rec.Code, rec.Body)
	}
	if after := readTestClaims(t, h, inWorktree); len(after.Claims) != 1 {
		t.Errorf("stopping removed an accepted claim: %#v", after.Claims)
	}
	testChangeClaim(t, h, "release", inWorktree, h.root, offered)
	if after := readTestClaims(t, h, inWorktree); len(after.Claims) != 0 {
		t.Errorf("release left claims: %#v", after.Claims)
	}
}

func TestMailboxHTTP_DeclineLeavesThePathsWithTheSender(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	makeReviewRepo(t, h.root)
	const path = "src/parser.go"
	testChangeClaim(t, h, "claim", inProject, h.root, path)
	thread := mailboxSendThread(t, h, mailboxOfferBody(inWorktree, h.root, path))
	rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/handoff", mailboxHandoffBody(thread, "decline"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("decline = %d %s", rec.Code, rec.Body)
	}
	rec = mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/threads?tool=claude&id="+inWorktree, "")
	if !strings.Contains(rec.Body.String(), `"handoff":"declined"`) {
		t.Errorf("receiver's declined handoff = %d %s", rec.Code, rec.Body)
	}
	if claims := readTestClaims(t, h, ""); len(claims.Claims) != 1 || claims.Claims[0].Owner.ID != inProject {
		t.Errorf("declining changed claims: %#v", claims.Claims)
	}
}

func TestMailboxHTTP_HandoffConflictShowsTheCurrentHolderOnBothRows(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	makeReviewRepo(t, h.root)
	const other = "44444444-4444-4444-8444-444444444444"
	writeClaudeSession(t, h.scanner.Stores.ClaudeDir, other, h.root, "Hold the parser")
	const path = "src/parser.go"
	testChangeClaim(t, h, "claim", inProject, h.root, path)
	thread := mailboxSendThread(t, h, mailboxOfferBody(inWorktree, h.root, path))
	testChangeClaim(t, h, "release", inProject, h.root, path)
	testChangeClaim(t, h, "claim", other, h.root, path)
	rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/handoff", mailboxHandoffBody(thread, "accept"))
	mailboxExpectRefusal(t, rec, "ownership")
	if !strings.Contains(rec.Body.String(), path) || !strings.Contains(rec.Body.String(), other) {
		t.Errorf("conflict response lacks the path or holder: %s", rec.Body)
	}
	if claims := readTestClaims(t, h, ""); len(claims.Claims) != 1 || claims.Claims[0].Owner.ID != other {
		t.Errorf("a refused accept changed claims: %#v", claims.Claims)
	}
	for _, id := range []string{inProject, inWorktree} {
		claims := readTestClaims(t, h, id)
		if len(claims.Conflicts) != 1 || claims.Conflicts[0].Holder == nil || claims.Conflicts[0].Holder.ID != other || claims.Conflicts[0].Path != path {
			t.Errorf("conflicts for %s = %#v", id, claims.Conflicts)
		}
	}
	var count int
	for _, row := range listConversations(t, h).Conversations {
		if row.ID != inProject && row.ID != inWorktree {
			continue
		}
		count++
		raw, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `"conflicts":`) || !strings.Contains(string(raw), other) || !strings.Contains(string(raw), path) {
			t.Errorf("row %s lacks the conflict details: %s", row.ID, raw)
		}
	}
	if count != 2 {
		t.Errorf("found %d affected rows, want two", count)
	}
}

func TestMailboxHTTP_ClaimsUseTheKnownSessionRootAndTrackStaleOwners(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	makeReviewRepo(t, h.root)
	foreign := realDir(t)
	gitInit(t, foreign)
	const path = "src/parser.go"
	if preview := readTestClaims(t, h, inProject); !sameRealDir(h.root, preview.Worktree) {
		t.Errorf("selected row's worktree = %q, want %q", preview.Worktree, h.root)
	}
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/claim", mailboxClaimBody(inProject, foreign, path)), "format")
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/claim", mailboxClaimBody(inProject, h.root, "src/../parser.go")), "format")
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/send", mailboxOfferBody(inWorktree, h.root, path)), "ownership")

	tag := mailboxHTTP(t, h, http.MethodGet, conversationsViewPath, "").Header().Get("ETag")
	testChangeClaim(t, h, "claim", inProject, h.root, path)
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath, nil)
	req.Header.Set("If-None-Match", tag)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("ETag") == tag || !strings.Contains(rec.Body.String(), `"claimCount":1`) {
		t.Errorf("claim should refresh the row summary: %d %s", rec.Code, rec.Body)
	}

	req = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310/api/mailbox/claims", nil)
	req.Header.Set("Origin", "https://allowed.example")
	rec = httptest.NewRecorder()
	authority.CORS(h, "https://allowed.example").ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Errorf("allowed foreign origin read private claims: %d %s", rec.Code, rec.Body)
	}

	folder := projectFolderName.ReplaceAllString(filepath.Join(h.root, "src"), "-")
	logPath := filepath.Join(h.scanner.Stores.ClaudeDir, "projects", folder, inProject+".jsonl")
	if err := os.Remove(logPath); err != nil {
		t.Fatal(err)
	}
	if claims := readTestClaims(t, h, ""); len(claims.Claims) != 1 || !claims.Claims[0].Stale {
		t.Errorf("missing owner did not appear as stale: %#v", claims.Claims)
	}
	testChangeClaim(t, h, "release", inProject, h.root, path)
	if claims := readTestClaims(t, h, ""); len(claims.Claims) != 0 {
		t.Errorf("stale claim remained after release: %#v", claims.Claims)
	}
}

func TestMailboxHTTP_ClaimsRefuseOverlapAndExposeTheHolderToBothSessions(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	makeReviewRepo(t, h.root)
	const other = "44444444-4444-4444-8444-444444444444"
	writeClaudeSession(t, h.scanner.Stores.ClaudeDir, other, h.root, "Check the parser")
	testChangeClaim(t, h, "claim", inProject, h.root, "src")
	rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/claim", mailboxClaimBody(other, h.root, "src/parser.go"))
	mailboxExpectRefusal(t, rec, "ownership")
	for _, id := range []string{inProject, other} {
		got := readTestClaims(t, h, id)
		if len(got.Conflicts) != 1 || got.Conflicts[0].Holder == nil ||
			got.Conflicts[0].Holder.ID != inProject || got.Conflicts[0].Path != "src/parser.go" {
			t.Errorf("conflicts for %s = %#v", id, got.Conflicts)
		}
	}
}

func TestMailboxHTTP_ClaimsAndHandoffsRejectAmbiguousAndExtraFields(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	claim := mailboxClaimBody(inProject, h.root, "src/parser.go")
	offer := mailboxOfferBody(inWorktree, h.root, "src/parser.go")
	for _, tc := range []struct{ name, path, body string }{
		{"duplicate paths", "/api/mailbox/claim", strings.Replace(claim, `"paths":`, `"paths":["src"],"paths":`, 1)},
		{"case alias", "/api/mailbox/release", strings.Replace(claim, `"tool":`, `"Tool":`, 1)},
		{"forged recipient", "/api/mailbox/handoff", `{"thread":"thread","n":1,"action":"accept","by":"agent"}`},
		{"duplicate kind", "/api/mailbox/send", strings.Replace(offer, `"type":`, `"type":"question","type":`, 1)},
		{"duplicate worktree", "/api/mailbox/send", strings.Replace(offer, `"worktree":`, `"worktree":"/other","worktree":`, 1)},
		{"inapplicable review fields", "/api/mailbox/send", strings.TrimSuffix(offer, "}") + `,"refs":null}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, tc.path, tc.body), "format")
		})
	}
}

func TestMailboxHTTP_ListCountsClaimedPathsAndStaleOwners(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	makeReviewRepo(t, h.root)
	if view := listConversations(t, h); view.MailboxClaims != nil {
		t.Errorf("no claim should send no totals: %#v", view.MailboxClaims)
	}
	body := fmt.Sprintf(`{"tool":"claude","id":%q,"worktree":%q,"paths":["src/parser.go","README.md"]}`, inProject, h.root)
	if rec := mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/claim", body); rec.Code != http.StatusNoContent {
		t.Fatalf("claim = %d %s", rec.Code, rec.Body)
	}
	const other = "44444444-4444-4444-8444-444444444444"
	writeClaudeSession(t, h.scanner.Stores.ClaudeDir, other, h.root, "Check the parser")
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/claim", mailboxClaimBody(other, h.root, "README.md")), "ownership")
	view := listConversations(t, h)
	want := conversations.MailboxClaims{Paths: 2, Conflicts: 1}
	if view.MailboxClaims == nil || *view.MailboxClaims != want {
		t.Errorf("totals = %#v, want %#v", view.MailboxClaims, want)
	}
	for _, row := range view.Conversations {
		if row.ID == inProject && row.ClaimedPaths != 2 {
			t.Errorf("owner row claimedPaths = %d, want 2", row.ClaimedPaths)
		}
	}

	folder := projectFolderName.ReplaceAllString(filepath.Join(h.root, "src"), "-")
	if err := os.Remove(filepath.Join(h.scanner.Stores.ClaudeDir, "projects", folder, inProject+".jsonl")); err != nil {
		t.Fatal(err)
	}
	want = conversations.MailboxClaims{Paths: 2, Stale: 2, Conflicts: 1}
	if view := listConversations(t, h); view.MailboxClaims == nil || *view.MailboxClaims != want {
		t.Errorf("totals with a gone owner = %#v, want %#v", view.MailboxClaims, want)
	}
}

func dismissConflictHTTP(t *testing.T, h http.Handler, secret, id, root, path string) *httptest.ResponseRecorder {
	t.Helper()
	body := fmt.Sprintf(`{"tool":"claude","id":%q,"worktree":%q,"path":%q}`, id, root, path)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:4310/api/mailbox/conflict/dismiss", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set(companionSecretHeader, secret)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestMailboxHTTP_TheCompanionDismissesARefusedClaimAndKeepsTheHoldersClaim(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	h.companion = testCompanionSecret
	makeReviewRepo(t, h.root)
	const other = "44444444-4444-4444-8444-444444444444"
	const path = "src/parser.go"
	writeClaudeSession(t, h.scanner.Stores.ClaudeDir, other, h.root, "Check the parser")
	testChangeClaim(t, h, "claim", inProject, h.root, path)
	thread := mailboxSendThread(t, h, mailboxSendBody(inWorktree, "question", "Is the parser done?", ""))
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/claim", mailboxClaimBody(other, h.root, path)), "ownership")
	if totals := listConversations(t, h).MailboxClaims; totals == nil || totals.Conflicts != 1 {
		t.Fatalf("totals before dismiss = %#v", totals)
	}

	secret := string(testCompanionSecret)
	if rec := dismissConflictHTTP(t, h, "", other, h.root, path); rec.Code != http.StatusForbidden {
		t.Errorf("dismiss without the companion secret = %d %s", rec.Code, rec.Body)
	}
	if rec := dismissConflictHTTP(t, h, strings.Repeat("0", len(secret)), other, h.root, path); rec.Code != http.StatusForbidden {
		t.Errorf("dismiss with a wrong secret = %d %s", rec.Code, rec.Body)
	}
	mailboxExpectRefusal(t, dismissConflictHTTP(t, h, secret, other, "", path), "format")
	mailboxExpectRefusal(t, dismissConflictHTTP(t, h, secret, "not-a-session", h.root, path), "unknown-session")
	if rec := dismissConflictHTTP(t, h, secret, other, h.root, path); rec.Code != http.StatusNoContent {
		t.Fatalf("dismiss = %d %s", rec.Code, rec.Body)
	}
	for read := range 2 {
		totals := listConversations(t, h).MailboxClaims
		if totals == nil || totals.Conflicts != 0 || totals.Paths != 1 {
			t.Errorf("read %d after dismiss: totals = %#v, want the claim and no conflict", read, totals)
		}
	}
	if claims := readTestClaims(t, h, ""); len(claims.Claims) != 1 || claims.Claims[0].Owner.ID != inProject {
		t.Errorf("dismiss changed the holder's claim: %#v", claims.Claims)
	}
	if threads := readTestThreads(t, h, inProject); len(threads) != 1 || threads[0] != thread {
		t.Errorf("dismiss changed the threads: %v", threads)
	}
}

func TestMailboxHTTP_GoneReleaseChecksTheOwnerAndTheExactClaim(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	makeReviewRepo(t, h.root)
	const path = "src/parser.go"
	testChangeClaim(t, h, "claim", inProject, h.root, path)
	gone := func(p string) *httptest.ResponseRecorder {
		body := fmt.Sprintf(`{"tool":"claude","id":%q,"worktree":%q,"paths":[%q],"gone":true}`, inProject, h.root, p)
		return mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/release", body)
	}
	mailboxExpectRefusal(t, gone(path), "ownership")
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/claim", fmt.Sprintf(`{"tool":"claude","id":%q,"worktree":%q,"paths":[%q],"gone":true}`, inProject, h.root, path)), "format")

	folder := projectFolderName.ReplaceAllString(filepath.Join(h.root, "src"), "-")
	if err := os.Remove(filepath.Join(h.scanner.Stores.ClaudeDir, "projects", folder, inProject+".jsonl")); err != nil {
		t.Fatal(err)
	}
	mailboxExpectRefusal(t, gone("src"), "ownership")
	if rec := gone(path); rec.Code != http.StatusNoContent {
		t.Fatalf("gone release = %d %s", rec.Code, rec.Body)
	}
	if claims := readTestClaims(t, h, ""); len(claims.Claims) != 0 {
		t.Errorf("claims after gone release = %#v", claims.Claims)
	}
}

func TestClaimOwnerGone_OnlyWhenTheOwnersStoreWasReadInFull(t *testing.T) {
	owner := conversations.Ref{Tool: conversations.ToolCodex, ID: "0199a2b4-5c1e-7d30-9f2a-3b6c8d1e4f50"}
	for _, tc := range []struct {
		name string
		read conversations.RefsRead
		want bool
	}{
		{"listed and usable", conversations.RefsRead{Rows: map[conversations.Ref]conversations.Conversation{owner: {Tool: owner.Tool, ID: owner.ID}}}, false},
		{"listed as a cloud row", conversations.RefsRead{Rows: map[conversations.Ref]conversations.Conversation{owner: {Tool: owner.Tool, ID: owner.ID, App: conversations.AppCloud}}}, true},
		{"missing from a complete read", conversations.RefsRead{Complete: map[conversations.Tool]bool{conversations.ToolCodex: true}}, true},
		{"missing after a failed read of its store", conversations.RefsRead{Complete: map[conversations.Tool]bool{conversations.ToolClaude: true}}, false},
		{"files listed without a row", conversations.RefsRead{Complete: map[conversations.Tool]bool{conversations.ToolCodex: true}, Listed: map[conversations.Ref]bool{owner: true}}, false},
		{"no read", conversations.RefsRead{}, false},
	} {
		if got := claimOwnerGone(tc.read, owner); got != tc.want {
			t.Errorf("%s: gone = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func readTestThreads(t *testing.T, h http.Handler, id string) []string {
	t.Helper()
	rec := mailboxHTTP(t, h, http.MethodGet, "/api/mailbox/threads?tool=claude&id="+id, "")
	var reply struct {
		Threads []struct {
			ID string `json:"id"`
		} `json:"threads"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("threads = %d %s (%v)", rec.Code, rec.Body, err)
	}
	ids := make([]string, 0, len(reply.Threads))
	for _, thread := range reply.Threads {
		ids = append(ids, thread.ID)
	}
	return ids
}

func TestMailboxHTTP_ConflictShowsOnBothRowsUntilTheRequesterDismissesIt(t *testing.T) {
	h, _ := mailboxHTTPFixture(t)
	h.companion = testCompanionSecret
	makeReviewRepo(t, h.root)
	const other = "44444444-4444-4444-8444-444444444444"
	const path = "src/parser.go"
	writeClaudeSession(t, h.scanner.Stores.ClaudeDir, other, h.root, "Check the parser")
	testChangeClaim(t, h, "claim", inProject, h.root, path)
	mailboxExpectRefusal(t, mailboxHTTP(t, h, http.MethodPost, "/api/mailbox/claim", mailboxClaimBody(other, h.root, path)), "ownership")
	conflictsOf := func() map[string][]conversations.MailboxConflict {
		got := map[string][]conversations.MailboxConflict{}
		for _, row := range listConversations(t, h).Conversations {
			if len(row.ClaimConflicts) > 0 {
				got[row.ID] = row.ClaimConflicts
			}
		}
		return got
	}
	got := conflictsOf()
	for _, id := range []string{inProject, other} {
		if len(got[id]) != 1 || got[id][0].Path != path || got[id][0].Requester.ID != other || got[id][0].Holder == nil || got[id][0].Holder.ID != inProject {
			t.Errorf("row %s claimConflicts = %#v", id, got[id])
		}
	}
	if len(got) != 2 {
		t.Errorf("rows with conflicts = %d, want the holder and the requester", len(got))
	}
	secret := string(testCompanionSecret)
	if rec := dismissConflictHTTP(t, h, secret, inProject, h.root, path); rec.Code != http.StatusNoContent {
		t.Fatalf("holder dismiss = %d %s", rec.Code, rec.Body)
	}
	if got := conflictsOf(); len(got) != 2 {
		t.Errorf("the holder dismissed the requester's conflict: %#v", got)
	}
	if rec := dismissConflictHTTP(t, h, secret, other, h.root, path); rec.Code != http.StatusNoContent {
		t.Fatalf("requester dismiss = %d %s", rec.Code, rec.Body)
	}
	if got := conflictsOf(); len(got) != 0 {
		t.Errorf("conflicts after dismiss = %#v", got)
	}
}
