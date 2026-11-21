package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

// approvalsFixture is a global conversations server with one live
// Claude Code session (inProject, pid 4242), a temporary agnostic-ai
// home, and a clock the test moves.
type approvalsFixture struct {
	h        *conversationsHandler
	project  string
	home     string
	userHome string
	now      time.Time
	routed   bool
	// reviewerOff turns the tools' own reviewers off in their settings.
	reviewerOff bool
}

func newApprovalsFixture(t *testing.T, mode string, routed bool) *approvalsFixture {
	t.Helper()
	h, _, project := conversationsFixture(t, true)
	h.scanner.Processes = runningProcesses{pid: 4242}
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	// A baseline before the transcripts, so a waiting session is unseen.
	writeTestFile(t, h.scanner.MarksPath, `{"schemaVersion":"conversation-marks/v1","since":"2026-01-01T00:00:00Z","marks":{}}`)
	f := &approvalsFixture{h: h, project: project, home: t.TempDir(), now: time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC), routed: routed}
	if mode != "" {
		writeTestFile(t, filepath.Join(f.home, config.ConfigFileName), "approvals:\n  mode: "+mode+"\n")
	}
	f.userHome = t.TempDir()
	h.approvals = &approvalsReader{
		source:    func() (string, error) { return f.home, nil },
		home:      func() (string, error) { return f.userHome, nil },
		env:       func() string { return "" },
		now:       func() time.Time { return f.now },
		routed:    func(string, string) bool { return f.routed },
		activated: func(_ string, mode string, _ string) bool { return mode != config.ApprovalsReview || !f.reviewerOff },
	}
	return f
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// wait writes the session file of a Claude Code session that waits on a
// permission prompt since at; a zero at leaves the start out.
func (f *approvalsFixture) wait(t *testing.T, at time.Time) {
	t.Helper()
	since := ""
	if !at.IsZero() {
		since = fmt.Sprintf(`,"statusUpdatedAt":%d`, at.UnixMilli())
	}
	writeTestFile(t, filepath.Join(f.h.scanner.Stores.ClaudeDir, "sessions", "4242.json"),
		`{"pid":4242,"sessionId":"`+inProject+`","status":"waiting","waitingFor":"permission prompt"`+since+`}`)
}

// decide appends one decision for session to the decision log.
func (f *approvalsFixture) decide(t *testing.T, session string, outcome approvals.Outcome, rule string, at time.Time) {
	t.Helper()
	log := approvals.Log{Path: approvals.LogPath(f.home)}
	entry := approvals.Entry{Time: at, Tool: "claude", Session: session, Call: "Bash", Detail: "go test ./...", Mode: config.ApprovalsYolo, Decision: outcome, Rule: rule}
	if err := log.Append(entry); err != nil {
		t.Fatal(err)
	}
}

// busy writes the session file of the same session at work.
func (f *approvalsFixture) busy(t *testing.T) {
	t.Helper()
	writeTestFile(t, filepath.Join(f.h.scanner.Stores.ClaudeDir, "sessions", "4242.json"), `{"pid":4242,"sessionId":"`+inProject+`","status":"busy"}`)
}

func (f *approvalsFixture) list(t *testing.T) (conversations.View, conversations.Conversation) {
	t.Helper()
	view := listConversations(t, f.h)
	for _, c := range view.Conversations {
		if c.ID == inProject {
			return view, c
		}
	}
	t.Fatalf("no conversation %s listed: %s", inProject, ids(view))
	return view, conversations.Conversation{}
}

func reasonOf(c conversations.Conversation) string {
	if c.Approvals == nil {
		return ""
	}
	return c.Approvals.Reason
}

func approvedOf(c conversations.Conversation) int {
	if c.Approvals == nil {
		return 0
	}
	return c.Approvals.Approved
}

// needsPermission reports whether a row shows under Needs you as a
// permission wait, which the circle's lock count counts.
func needsPermission(c conversations.Conversation) bool {
	_, waits := c.PermissionWait()
	return c.State == conversations.StateBlocked && (waits || (c.Waiting != nil && c.Waiting.For == "permission"))
}

// What the person sees, per mode: ask is today's behavior in every column.
func TestApprovals_AskShowsEveryWaitAsToday(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mode   string
		routed bool
		decide approvals.Outcome
	}{
		{"no setting", "", true, ""},
		{"routine request", config.ApprovalsAsk, true, approvals.Approved},
		{"request the gate asks about", config.ApprovalsAsk, true, approvals.Asked},
		{"tool with no route", config.ApprovalsAsk, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newApprovalsFixture(t, tc.mode, tc.routed)
			since := f.now.Add(-200 * time.Millisecond)
			f.wait(t, since)
			if tc.decide != "" {
				f.decide(t, inProject, tc.decide, approvals.RulePublish, since.Add(50*time.Millisecond))
			}

			view, c := f.list(t)

			if !needsPermission(c) || view.NeedsYou != 1 {
				t.Errorf("state %s, waiting %+v, needsYou %d; want a permission row that counts", c.State, c.Waiting, view.NeedsYou)
			}
			if reason := reasonOf(c); reason != "" {
				t.Errorf("reason %q in ask", reason)
			}
			if view.ApprovalsMode != "" {
				t.Errorf("approvalsMode %q in ask", view.ApprovalsMode)
			}
		})
	}
}

// yolo, routine request: no row under Needs you, no lock count, and
// "N approved for you" on the row. The hook decides nothing in review.
func TestApprovals_ApprovedRequestNeverNeedsYou(t *testing.T) {
	for _, mode := range []string{config.ApprovalsYolo} {
		t.Run(mode, func(t *testing.T) {
			f := newApprovalsFixture(t, mode, true)
			since := f.now.Add(-200 * time.Millisecond)
			f.wait(t, since)
			f.decide(t, inProject, approvals.Approved, "", since.Add(50*time.Millisecond))

			view, c := f.list(t)

			if needsPermission(c) || c.Waiting != nil || view.NeedsYou != 0 {
				t.Errorf("state %s, waiting %+v, needsYou %d; an approved request must not need the person", c.State, c.Waiting, view.NeedsYou)
			}
			if c.State != conversations.StateWorking || c.Activity != conversations.ActivityWorking {
				t.Errorf("state %s, activity %s; want working while the tool runs the approved call", c.State, c.Activity)
			}
			if approvedOf(c) != 1 || reasonOf(c) != "" {
				t.Errorf("approvals %+v; want 1 approved and no reason", c.Approvals)
			}
			if view.ApprovalsMode != mode {
				t.Errorf("approvalsMode %q, want %q", view.ApprovalsMode, mode)
			}
		})
	}
}

// yolo, request the gate sends to the person: a permission row that
// counts, with the rule as its reason.
func TestApprovals_RequestTheGateAsksAboutShowsWithItsReason(t *testing.T) {
	for _, mode := range []string{config.ApprovalsYolo} {
		t.Run(mode, func(t *testing.T) {
			f := newApprovalsFixture(t, mode, true)
			since := f.now.Add(-200 * time.Millisecond)
			f.wait(t, since)
			f.decide(t, inProject, approvals.Asked, approvals.RulePublish, since.Add(30*time.Millisecond))

			view, c := f.list(t)

			if !needsPermission(c) || view.NeedsYou != 1 {
				t.Errorf("state %s, needsYou %d; the gate's ask must reach the person at once", c.State, view.NeedsYou)
			}
			if reason := reasonOf(c); reason != "Always asks: publishing" {
				t.Errorf("reason %q", reason)
			}
		})
	}
}

// review and yolo, tool with no safe route: a permission row that says so.
func TestApprovals_ToolWithNoRouteSaysSo(t *testing.T) {
	for _, mode := range []string{config.ApprovalsReview, config.ApprovalsYolo} {
		t.Run(mode, func(t *testing.T) {
			f := newApprovalsFixture(t, mode, false)
			f.wait(t, f.now.Add(-200*time.Millisecond))

			view, c := f.list(t)

			if !needsPermission(c) || view.NeedsYou != 1 {
				t.Errorf("state %s, needsYou %d; a tool approvals cannot answer must show its prompt", c.State, view.NeedsYou)
			}
			if reason := reasonOf(c); reason != "Approvals not supported for Claude Code" {
				t.Errorf("reason %q", reason)
			}
		})
	}
}

// The grace rule (issue #3700): a wait with no decision shows only once
// the hook timeout passed, and the list tag changes when it does.
func TestApprovals_WaitWithNoDecisionShowsAfterTheHookTimeout(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	since := f.now.Add(-2 * time.Second)
	f.wait(t, since)

	view, c := f.list(t)
	if needsPermission(c) || view.NeedsYou != 0 {
		t.Fatalf("state %s, needsYou %d; a wait the hook may still answer must not show", c.State, view.NeedsYou)
	}
	tag := listTagOf(t, f.h, "")

	f.now = since.Add(approvalsHookWait + time.Second)
	if rec := getList(t, f.h, tag); rec.Code == http.StatusNotModified {
		t.Fatal("the list answered unchanged after the hold ended")
	}
	view, c = f.list(t)
	if !needsPermission(c) || view.NeedsYou != 1 {
		t.Errorf("state %s, needsYou %d; the wait must show after the hook timeout", c.State, view.NeedsYou)
	}
	if reason := reasonOf(c); reason != approvalsReasonNoAnswer {
		t.Errorf("reason %q", reason)
	}
}

// The reviewer decides before the tool shows a prompt, so a review
// wait is the person's at once (issue #3799).
func TestApprovals_ReviewWaitShowsAtOnce(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsReview, true)
	f.wait(t, f.now.Add(-200*time.Millisecond))

	view, c := f.list(t)

	if !needsPermission(c) || view.NeedsYou != 1 || reasonOf(c) != approvalsReasonReviewer {
		t.Errorf("state %s, needsYou %d, reason %q", c.State, view.NeedsYou, reasonOf(c))
	}
}

// Review with the tool's reviewer off asks, as approvals status says.
func TestApprovals_ReviewWithTheReviewerOffShowsAsAsk(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsReview, true)
	f.reviewerOff = true
	f.wait(t, f.now.Add(-approvalsHookWait-time.Second))

	view, c := f.list(t)

	if !needsPermission(c) || reasonOf(c) != "" || view.ApprovalsMode != "" {
		t.Errorf("state %s, reason %q, approvalsMode %q", c.State, reasonOf(c), view.ApprovalsMode)
	}
}

// A search that leaves the held row out still does not count it.
func TestApprovals_SearchDoesNotCountAHeldWaitItLeavesOut(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	f.wait(t, f.now.Add(-2*time.Second))

	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath+"?editor=cursor&q=zzzznomatch", nil))
	var view conversations.View
	if err := json.Unmarshal(rec.Body.Bytes(), &view); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}

	if len(view.Conversations) != 0 || view.NeedsYou != 0 {
		t.Errorf("rows %s, needsYou %d; a held wait must not count", ids(view), view.NeedsYou)
	}
}

func TestApprovals_ReviewWaitShowsImmediatelyOnlyNamingActiveReviewer(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsReview, true)
	f.h.approvals.activated = approvalsActivated
	f.h.approvals.routed = func(tool, mode string) bool { return tool == "claude" && approvalsRouted(tool, mode) }
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	f.wait(t, f.now.Add(-time.Second))

	view, c := f.list(t)
	if !needsPermission(c) || view.NeedsYou != 1 || reasonOf(c) != "" || view.ApprovalsMode != "" {
		t.Errorf("reviewer off: state %s, count %d, reason %q, mode %q", c.State, view.NeedsYou, reasonOf(c), view.ApprovalsMode)
	}
	tag := listTagOf(t, f.h, "")

	writeTestFile(t, filepath.Join(f.userHome, ".claude", "settings.json"), `{"permissions":{"defaultMode":"auto"}}`)
	if rec := getList(t, f.h, tag); rec.Code != http.StatusOK {
		t.Errorf("a native settings change returned %d, want 200", rec.Code)
	}
	view, c = f.list(t)
	if !needsPermission(c) || view.NeedsYou != 1 || reasonOf(c) != approvalsReasonReviewer || view.ApprovalsMode != config.ApprovalsReview {
		t.Errorf("reviewer on: state %s, count %d, reason %q, mode %q", c.State, view.NeedsYou, reasonOf(c), view.ApprovalsMode)
	}
}

func TestApprovals_ReviewWaitWithNoDecisionNamesTheReviewer(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsReview, true)
	f.wait(t, f.now.Add(-approvalsHookWait-time.Second))

	_, c := f.list(t)

	if !needsPermission(c) || reasonOf(c) != approvalsReasonReviewer {
		t.Errorf("state %s, reason %q", c.State, reasonOf(c))
	}
}

func TestApprovals_DecisionFromAnEarlierWaitDoesNotHoldThisOne(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	since := f.now.Add(-approvalsHookWait - time.Second)
	f.wait(t, since)
	f.decide(t, inProject, approvals.Approved, "", since.Add(-5*time.Second))

	_, c := f.list(t)

	if !needsPermission(c) || reasonOf(c) != approvalsReasonNoAnswer {
		t.Errorf("state %s, reason %q; an approval before this wait says nothing about it", c.State, reasonOf(c))
	}
	if approvedOf(c) != 1 {
		t.Errorf("approved %d, want the earlier approval counted", approvedOf(c))
	}
}

func TestApprovals_WaitThatOutlastsItsApprovalShows(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	since := f.now.Add(-approvalsHookWait - 2*time.Second)
	f.wait(t, since)
	f.decide(t, inProject, approvals.Approved, "", since.Add(100*time.Millisecond))

	_, c := f.list(t)

	if !needsPermission(c) || reasonOf(c) != approvalsReasonStillWaiting {
		t.Errorf("state %s, reason %q; a prompt still open long after the approval must reach the person", c.State, reasonOf(c))
	}
}

func TestApprovals_WaitWithoutAStartIsHeldFromWhenTheServerSawIt(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	f.wait(t, time.Time{})

	if _, c := f.list(t); needsPermission(c) {
		t.Fatalf("state %s; a new wait must be held first", c.State)
	}
	f.now = f.now.Add(approvalsHookWait + time.Second)
	if _, c := f.list(t); !needsPermission(c) || reasonOf(c) != approvalsReasonNoAnswer {
		t.Errorf("state %s, reason %q after the hook timeout", c.State, reasonOf(c))
	}
}

func TestApprovals_RowCountsItsSessionsApprovals(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	for i := range 3 {
		f.decide(t, inProject, approvals.Approved, "", f.now.Add(time.Duration(i-10)*time.Minute))
	}
	f.decide(t, inProject, approvals.Asked, approvals.RuleDelete, f.now.Add(-time.Minute))
	f.decide(t, elsewhere, approvals.Approved, "", f.now.Add(-time.Minute))

	view := listConversations(t, f.h)

	for _, c := range view.Conversations {
		switch c.ID {
		case inProject:
			if approvedOf(c) != 3 || len(c.Approvals.Recent) != 3 || c.Approvals.Recent[0].Time != "2026-11-07T11:52:00Z" || c.Approvals.Recent[0].Detail != "go test ./..." {
				t.Errorf("%s approvals %+v", c.ID, c.Approvals)
			}
		case elsewhere:
			if approvedOf(c) != 1 {
				t.Errorf("%s approvals %+v", c.ID, c.Approvals)
			}
		default:
			if c.Approvals != nil {
				t.Errorf("%s approvals %+v; it has no decision", c.ID, c.Approvals)
			}
		}
	}
}

func TestApprovals_NewDecisionChangesTheListTag(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	tag := listTagOf(t, f.h, "")
	if rec := getList(t, f.h, tag); rec.Code != http.StatusNotModified {
		t.Fatalf("an unchanged list = %d", rec.Code)
	}

	f.decide(t, inProject, approvals.Approved, "", f.now)

	if rec := getList(t, f.h, tag); rec.Code != http.StatusOK {
		t.Errorf("after a decision the list = %d; an approval must show within one refresh", rec.Code)
	}
}

// Codex runs the hook before it prompts, and a cloud session runs no
// local hook: neither wait is held.
func TestApplyApprovals_HoldsOnlyALocalClaudeCodeWait(t *testing.T) {
	now := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	var view conversations.View
	if err := json.Unmarshal([]byte(`{"needsYou":3,"conversations":[
		{"tool":"codex","id":"t1","live":true,"state":"blocked","waiting":{"for":"permission"}},
		{"tool":"codex","id":"t2","live":true,"state":"blocked","waiting":{"for":"permission"}},
		{"tool":"claude","app":"cloud","id":"c1","live":true,"state":"blocked","waiting":{"for":"permission"}}
	]}`), &view); err != nil {
		t.Fatal(err)
	}
	snap := approvalsSnapshot{
		policy: config.ApprovalsPolicy{Home: config.ApprovalsConfig{Mode: config.ApprovalsYolo}}, policyOK: true,
		projectAsk: map[string]string{}, routed: func(string, string) bool { return true },
		sessions: approvalSessionsOf([]approvals.Entry{{Time: now.Add(-time.Minute), Tool: "codex", Session: "t2", Decision: approvals.Asked, Rule: approvals.RuleNetwork}}),
	}
	before := view.Conversations[0]

	hold := applyApprovals(&view, snap, "", now, map[string]time.Time{})

	if !hold.IsZero() || view.NeedsYou != 3 {
		t.Errorf("hold %v, needsYou %d; nothing may be held", hold, view.NeedsYou)
	}
	want := map[string]string{"t1": "", "t2": "Always asks: network access", "c1": approvalsReasonCloud}
	for _, c := range view.Conversations {
		if c.State != conversations.StateBlocked || reasonOf(c) != want[c.ID] {
			t.Errorf("%s: state %s, reason %q, want %q", c.ID, c.State, reasonOf(c), want[c.ID])
		}
	}
	if before.Approvals != nil {
		t.Error("applyApprovals changed the scanner's row")
	}
}

func TestApprovalsSnapshot_HeaderModeIsTheLoosestSetting(t *testing.T) {
	project := t.TempDir()
	for _, tc := range []struct {
		home config.ApprovalsConfig
		env  string
		want string
	}{
		{config.ApprovalsConfig{}, "", config.ApprovalsAsk},
		{config.ApprovalsConfig{Mode: config.ApprovalsReview}, "", config.ApprovalsReview},
		{config.ApprovalsConfig{Mode: config.ApprovalsReview, Tools: map[string]string{"codex": config.ApprovalsYolo}}, "", config.ApprovalsYolo},
		{config.ApprovalsConfig{Projects: map[string]string{project: config.ApprovalsYolo}}, "", config.ApprovalsYolo},
		{config.ApprovalsConfig{Mode: config.ApprovalsYolo}, "ask", config.ApprovalsAsk},
	} {
		snap := approvalsSnapshot{
			policy: config.ApprovalsPolicy{Home: tc.home, Env: tc.env}, policyOK: true,
			projectAsk: map[string]string{}, routed: func(string, string) bool { return true },
		}
		if got := snap.headerMode(""); got != tc.want {
			t.Errorf("headerMode(%+v, env %q) = %q, want %q", tc.home, tc.env, got, tc.want)
		}
	}
}

// The header names only a mode the tool it is set for carries out (#3799).
func TestApprovalsSnapshot_HeaderModeSkipsModesNoToolCarriesOut(t *testing.T) {
	home := config.ApprovalsConfig{Mode: config.ApprovalsReview, Tools: map[string]string{"cursor": config.ApprovalsYolo}}
	for _, tc := range []struct {
		name       string
		routed     func(tool, mode string) bool
		reviewerOn bool
		want       string
	}{
		{"cursor has no yolo route", func(tool, mode string) bool { return mode == config.ApprovalsReview || tool == "claude" }, true, config.ApprovalsReview},
		{"every reviewer is off", func(tool, mode string) bool { return mode == config.ApprovalsReview }, false, config.ApprovalsAsk},
		{"no route at all", func(string, string) bool { return false }, true, config.ApprovalsAsk},
	} {
		snap := approvalsSnapshot{
			policy: config.ApprovalsPolicy{Home: home}, policyOK: true, routed: tc.routed,
			activated: func(_ string, mode string) bool { return mode != config.ApprovalsReview || tc.reviewerOn }, active: map[approvalDisplayRoute]bool{},
		}
		if got := snap.headerMode(""); got != tc.want {
			t.Errorf("%s: headerMode = %q, want %q", tc.name, got, tc.want)
		}
	}
	claudeOnly := approvalsSnapshot{
		policy:   config.ApprovalsPolicy{Home: config.ApprovalsConfig{Mode: config.ApprovalsReview, Tools: map[string]string{"claude": config.ApprovalsAsk}}},
		policyOK: true, routed: func(tool, mode string) bool { return tool == "claude" },
	}
	if got := claudeOnly.headerMode(""); got != config.ApprovalsAsk {
		t.Errorf("review for tools with no route, ask for the one with a route: headerMode = %q, want ask", got)
	}
}

func TestApprovals_UserHeaderFollowsProjectPolicyWithoutAConversation(t *testing.T) {
	f := newApprovalsFixture(t, "", true)
	settings := filepath.Join(f.home, config.ConfigFileName)
	project := filepath.Join(f.userHome, "sandbox")
	projectSettings := filepath.Join(project, config.ConfigFileName)
	const homePolicy = "approvals:\n  projects:\n    ~/sandbox: yolo\n"
	writeTestFile(t, settings, homePolicy)

	if view := listConversations(t, f.h); view.ApprovalsMode != config.ApprovalsYolo {
		t.Fatalf("header for the configured folder = %q, want yolo", view.ApprovalsMode)
	}
	tag := listTagOf(t, f.h, "")
	if rec := getList(t, f.h, tag); rec.Code != http.StatusNotModified {
		t.Fatalf("unchanged list returned %d, want 304", rec.Code)
	}

	writeTestFile(t, projectSettings, "approvals:\n  mode: ask\n")
	if rec := getList(t, f.h, tag); rec.Code != http.StatusOK {
		t.Errorf("changed project policy returned %d, want 200", rec.Code)
	}
	if view := listConversations(t, f.h); view.ApprovalsMode != "" {
		t.Errorf("header when the project asks = %q, want empty", view.ApprovalsMode)
	}

	if err := os.Remove(projectSettings); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, settings, homePolicy+fmt.Sprintf("    %q: ask\n", filepath.ToSlash(project)))
	if view := listConversations(t, f.h); view.ApprovalsMode != "" {
		t.Errorf("header with a stricter alias for the project = %q, want empty", view.ApprovalsMode)
	}
}

func TestApprovals_UserHeaderAlsoChecksThePhysicalProject(t *testing.T) {
	f := newApprovalsFixture(t, "", true)
	project := filepath.Join(f.userHome, "physical", "project")
	aliasDir := filepath.Join(f.userHome, "aliases")
	writeTestFile(t, filepath.Join(project, "README.md"), "Project.\n")
	writeTestFile(t, filepath.Join(aliasDir, config.ConfigFileName), "approvals:\n  mode: ask\n")
	if err := os.Symlink(project, filepath.Join(aliasDir, "project")); err != nil {
		t.Skipf("symlinks unsupported: %v", err)
	}
	writeTestFile(t, filepath.Join(f.home, config.ConfigFileName), "approvals:\n  projects:\n    ~/aliases/project: yolo\n")

	if view := listConversations(t, f.h); view.ApprovalsMode != config.ApprovalsYolo {
		t.Fatalf("header for the physical project = %q, want yolo", view.ApprovalsMode)
	}
	tag := listTagOf(t, f.h, "")
	writeTestFile(t, filepath.Join(project, config.ConfigFileName), "approvals:\n  mode: ask\n")
	if rec := getList(t, f.h, tag); rec.Code != http.StatusOK {
		t.Errorf("changed physical project policy returned %d, want 200", rec.Code)
	}
	if view := listConversations(t, f.h); view.ApprovalsMode != "" {
		t.Errorf("header when both paths ask = %q, want empty", view.ApprovalsMode)
	}
}

func TestApprovals_OnlyAnInstalledClaudeHookHoldsYoloWaits(t *testing.T) {
	oldOS := approvalsGOOS
	approvalsGOOS = "linux"
	t.Cleanup(func() { approvalsGOOS = oldOS })
	claude := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", claude)
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	f.h.userLevel = false
	f.h.approvals.activated = approvalsActivated
	f.h.approvals.routed = func(tool, mode string) bool { return tool == "claude" && approvalsRouted(tool, mode) }
	f.wait(t, f.now.Add(-time.Second))

	view, c := f.list(t)
	if !needsPermission(c) || view.NeedsYou != 1 || view.ApprovalsMode != "" {
		t.Errorf("hook absent: state %s, count %d, mode %q", c.State, view.NeedsYou, view.ApprovalsMode)
	}
	tag := listTagOf(t, f.h, "")

	writeTestFile(t, filepath.Join(claude, "settings.json"), `{"hooks":{"PermissionRequest":[{"hooks":[{"type":"command","command":"agnostic-ai hook approve"}]}]}}`)
	if rec := getList(t, f.h, tag); rec.Code != http.StatusOK {
		t.Errorf("installing the hook returned %d, want 200", rec.Code)
	}
	view, c = f.list(t)
	if needsPermission(c) || view.NeedsYou != 0 || view.ApprovalsMode != config.ApprovalsYolo {
		t.Errorf("hook installed: state %s, count %d, mode %q", c.State, view.NeedsYou, view.ApprovalsMode)
	}
	tag = listTagOf(t, f.h, "")
	writeTestFile(t, filepath.Join(f.project, config.ConfigFileName), "approvals:\n  mode: ask\n")
	if rec := getList(t, f.h, tag); rec.Code != http.StatusOK {
		t.Errorf("turning off approvals in the conversation folder returned %d, want 200", rec.Code)
	}
	view, c = f.list(t)
	if !needsPermission(c) || view.NeedsYou != 1 || reasonOf(c) != "" || view.ApprovalsMode != "" {
		t.Errorf("project asks: state %s, count %d, reason %q, mode %q", c.State, view.NeedsYou, reasonOf(c), view.ApprovalsMode)
	}
}

func TestApprovalsHandler_ReadRouteReturnsModeAndDecisionsPerSession(t *testing.T) {
	userHome := approvalsUserHome(t)
	home := filepath.Join(userHome, ".agnostic-ai")
	writeTestFile(t, filepath.Join(home, config.ConfigFileName), "targets: [claude]\napprovals:\n  mode: review\n")
	writeTestFile(t, filepath.Join(userHome, ".claude", "settings.json"), `{"permissions":{"defaultMode":"auto"}}`)
	log := approvals.Log{Path: approvals.LogPath(home)}
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	for i, e := range []approvals.Entry{
		{Tool: "claude", Session: "a", Call: "Bash", Detail: "go test ./...", Decision: approvals.Approved},
		{Tool: "claude", Session: "a", Call: "Bash", Detail: "git push", Decision: approvals.Asked, Rule: approvals.RulePublish},
		{Tool: "codex", Session: "b", Call: "shell", Decision: approvals.Approved},
	} {
		e.Time, e.Mode = at.Add(time.Duration(i)*time.Second), config.ApprovalsYolo
		if err := log.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	h := approvalsHandler{reader: newApprovalsReader()}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+approvalsViewPath, nil))

	var got struct {
		Mode     string `json:"mode"`
		Sessions []struct {
			Tool      string            `json:"tool"`
			Session   string            `json:"session"`
			Approved  int               `json:"approved"`
			Asked     int               `json:"asked"`
			Decisions []approvals.Entry `json:"decisions"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); rec.Code != http.StatusOK || err != nil {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	if got.Mode != config.ApprovalsReview || len(got.Sessions) != 2 {
		t.Fatalf("answer %s", rec.Body)
	}
	b, a := got.Sessions[0], got.Sessions[1]
	if b.Session != "b" || b.Approved != 1 || a.Session != "a" || a.Approved != 1 || a.Asked != 1 {
		t.Errorf("sessions %s", rec.Body)
	}
	if len(a.Decisions) != 2 || a.Decisions[0].Rule != approvals.RulePublish {
		t.Errorf("session a decisions, newest first: %+v", a.Decisions)
	}
}

func TestApprovalsHandler_WriteRouteAcceptsOnlyAsk(t *testing.T) {
	home := t.TempDir()
	t.Setenv(envUserRoot, home)
	shared := filepath.Join(home, config.ConfigFileName)
	writeTestFile(t, shared, "approvals:\n  mode: yolo\n")
	h := approvalsHandler{reader: newApprovalsReader()}
	local := filepath.Join(home, "local", config.ConfigFileName)

	for _, body := range []string{`{"mode":"yolo"}`, `{"mode":"review"}`, `{}`, `{"mode":"ask","tools":{"codex":"yolo"}}`, `not json`} {
		if rec := postEdit(t, h, approvalsAskPath, body, nil); rec.Code != http.StatusBadRequest {
			t.Errorf("POST %s = %d %s", body, rec.Code, rec.Body)
		}
	}
	if _, err := os.Stat(local); !os.IsNotExist(err) {
		t.Fatalf("a refused request wrote %s: %v", local, err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+approvalsAskPath, nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET %s = %d", approvalsAskPath, rec.Code)
	}
	if rec := postEdit(t, h, approvalsAskPath, `{"mode":"ask"}`, func(r *http.Request) { r.Header.Set("Origin", "https://example.com") }); rec.Code != http.StatusForbidden {
		t.Errorf("POST from another origin = %d", rec.Code)
	}

	if rec := postEdit(t, h, approvalsAskPath, `{"mode":"ask"}`, nil); rec.Code != http.StatusOK {
		t.Fatalf("POST ask = %d %s", rec.Code, rec.Body)
	}

	data, err := os.ReadFile(local)
	if err != nil || !strings.Contains(string(data), "mode: ask") {
		t.Fatalf("%s = %q, %v", local, data, err)
	}
	status, err := readApprovalsStatus(io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range status.Tools {
		if tool.Mode != config.ApprovalsAsk {
			t.Errorf("after Ask me, approvals status says %s for %s", tool.Mode, tool.Tool)
		}
	}
}

func listTagOf(t *testing.T, h http.Handler, held string) string {
	t.Helper()
	return getList(t, h, held).Header().Get("ETag")
}

func getList(t *testing.T, h http.Handler, held string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath+"?editor=cursor", nil)
	if held != "" {
		req.Header.Set("If-None-Match", held)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestApprovals_AskedCallInAWaitWinsOverALaterApproval(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	since := f.now.Add(-time.Second)
	f.wait(t, since)
	f.decide(t, inProject, approvals.Asked, approvals.RuleSecret, since.Add(100*time.Millisecond))
	f.decide(t, inProject, approvals.Approved, "", since.Add(200*time.Millisecond))

	_, c := f.list(t)

	if !needsPermission(c) || reasonOf(c) != "Always asks: secrets" {
		t.Errorf("state %s, reason %q; a call left to the person keeps the wait", c.State, reasonOf(c))
	}
}

func TestApprovals_ANewWaitWithoutAStartGetsItsOwnGrace(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	f.wait(t, time.Time{})
	f.list(t)
	f.now = f.now.Add(time.Minute)
	f.busy(t)
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath+"?q=nothing+matches", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("search = %d %s", rec.Code, rec.Body)
	}

	f.now = f.now.Add(time.Minute)
	f.wait(t, time.Time{})

	if _, c := f.list(t); needsPermission(c) {
		t.Errorf("state %s, reason %q; the second wait must be held from when it began", c.State, reasonOf(c))
	}
}

func TestApprovals_ASearchDoesNotDropTheListsHold(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	since := f.now.Add(-2 * time.Second)
	f.wait(t, since)
	tag := listTagOf(t, f.h, "")
	rec := httptest.NewRecorder()
	f.h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "http://127.0.0.1:4310"+conversationsViewPath+"?q=nothing+matches", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("search = %d %s", rec.Code, rec.Body)
	}

	f.now = since.Add(approvalsHookWait + time.Second)

	if rec := getList(t, f.h, tag); rec.Code == http.StatusNotModified {
		t.Error("the list answered unchanged after its hold ended, because a search cleared it")
	}
}

func TestApprovals_AProjectFileThatDoesNotParseAsks(t *testing.T) {
	f := newApprovalsFixture(t, config.ApprovalsYolo, true)
	writeTestFile(t, filepath.Join(f.project, config.ConfigFileName), "approvals: [\n")
	f.wait(t, f.now.Add(-200*time.Millisecond))

	view, c := f.list(t)

	if !needsPermission(c) || view.NeedsYou != 1 || reasonOf(c) != "" {
		t.Errorf("state %s, needsYou %d, reason %q; a broken project file must not loosen", c.State, view.NeedsYou, reasonOf(c))
	}
}
