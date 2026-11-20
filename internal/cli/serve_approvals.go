package cli

import (
	"cmp"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
	"github.com/chemaclass/agnostic-ai/internal/preview"
)

const (
	approvalsViewPath = "/api/views/approvals"
	approvalsAskPath  = "/api/approvals/ask"
)

// approvalsHookWait is the timeout of the approvals hook entry. Claude
// Code records a permission wait while the hook runs, so a wait younger
// than this may still be answered by the hook (decision 022).
const approvalsHookWait = 10 * time.Second

// approvalsClockSlack lets a decision stamped just before the tool
// recorded its wait count for that wait.
const approvalsClockSlack = time.Second

// Rows of the approvals read: the latest decisions a session lists and
// the latest approved calls a conversation row carries.
const (
	approvalsSessionDecisions = 20
	approvalsRecentCalls      = 10
)

// approvalsRuleReasons say, per gate rule, why a request reached the
// person. The text is fixed here, so no file a session wrote can word it.
var approvalsRuleReasons = map[string]string{
	approvals.RuleMode:           "Approvals are set to ask here",
	approvals.RulePermissionMode: "Always asks in the tool's reviewer or plan mode",
	approvals.RuleTool:           "Always asks: a tool approvals do not read",
	approvals.RuleMCP:            "Always asks: MCP tool",
	approvals.RuleWeb:            "Always asks: web access",
	approvals.RuleInput:          "Always asks: input approvals cannot read",
	approvals.RuleShellSyntax:    "Always asks: shell approvals cannot read",
	approvals.RuleDelete:         "Always asks: deleting or overwriting",
	approvals.RulePublish:        "Always asks: publishing",
	approvals.RuleNetwork:        "Always asks: network access",
	approvals.RuleSecret:         "Always asks: secrets",
	approvals.RulePrivilege:      "Always asks: privileges or processes",
	approvals.RuleOutsideProject: "Always asks: outside the project",
	approvals.RuleProtectedPath:  "Always asks: a protected file",
	approvals.RulePolicy:         "Always asks: the approvals settings",
	approvals.RuleAlwaysAsk:      "Always asks: your always-ask list",
	approvals.RuleUnknownCommand: "Always asks: a command approvals do not know",
}

const (
	approvalsReasonNoAnswer       = "No answer from the approvals hook"
	approvalsReasonReviewer       = "The tool's reviewer left this to you"
	approvalsReasonStillWaiting   = "Still waits after the approvals hook approved a request"
	approvalsReasonCloud          = "Approvals do not reach cloud sessions"
	approvalsReasonUnsupportedFmt = "Approvals not supported for %s"
)

var approvalsToolLabels = map[conversations.Tool]string{
	conversations.ToolClaude:  "Claude Code",
	conversations.ToolCodex:   "Codex",
	conversations.ToolCursor:  "Cursor",
	conversations.ToolChatGPT: "ChatGPT",
}

func approvalsRuleReason(rule string) string {
	if reason, ok := approvalsRuleReasons[rule]; ok {
		return reason
	}
	return "Always asks"
}

// approvalsRouted reports whether tool carries out mode.
func approvalsRouted(tool, mode string) bool {
	return approvalRouteFor(tool).Mechanism(mode, approvalsGOOS) != nil
}

// approvalSession is what the decision log says about one session.
type approvalSession struct {
	Tool     string `json:"tool"`
	Session  string `json:"session"`
	Approved int    `json:"approved"`
	Asked    int    `json:"asked"`
	// Decisions are the latest decisions, newest first.
	Decisions []approvals.Entry `json:"decisions"`

	last   approvals.Entry
	recent []conversations.ApprovedCall
}

func approvalSessionKey(tool, session string) string { return tool + ":" + session }

// approvalSessionsOf indexes log entries by tool and session. Entries
// come oldest first; one without a session belongs to no row. A detail
// that may hold a credential is dropped: the hook filters what it
// writes, but a line written by hand or by another program is not.
func approvalSessionsOf(entries []approvals.Entry) map[string]*approvalSession {
	sessions := map[string]*approvalSession{}
	for _, e := range slices.Backward(entries) {
		if e.Session == "" {
			continue
		}
		if e.Detail != "" && preview.ProseSensitive(e.Detail) {
			e.Detail = ""
		}
		key := approvalSessionKey(e.Tool, e.Session)
		s := sessions[key]
		if s == nil {
			s = &approvalSession{Tool: e.Tool, Session: e.Session, last: e}
			sessions[key] = s
		}
		if len(s.Decisions) < approvalsSessionDecisions {
			s.Decisions = append(s.Decisions, e)
		}
		switch e.Decision {
		case approvals.Approved:
			s.Approved++
			if len(s.recent) < approvalsRecentCalls {
				s.recent = append(s.recent, conversations.ApprovedCall{Time: e.Time.UTC().Format(time.RFC3339), Call: e.Call, Detail: e.Detail})
			}
		case approvals.Asked:
			s.Asked++
		}
	}
	return sessions
}

// approvalsSnapshot is the policy and the decision log, read once.
type approvalsSnapshot struct {
	policy   config.ApprovalsPolicy
	policyOK bool
	sessions map[string]*approvalSession
	// projectAsk caches config.ProjectApprovalsAsk per folder; a project
	// file that cannot be read asks.
	projectAsk map[string]string
	routed     func(tool, mode string) bool
	// reviewerOn reports whether tool's own reviewer is on in its user
	// settings; reviewers caches it per tool.
	reviewerOn func(tool string) bool
	reviewers  map[string]bool
}

// approvalsListTools are the tools the conversations list shows.
var approvalsListTools = []conversations.Tool{conversations.ToolClaude, conversations.ToolCodex, conversations.ToolCursor, conversations.ToolChatGPT}

// carries reports whether tool carries out mode: it has a route for it,
// and for review its reviewer is on. A review mode it does not carry
// out asks, as approvals status says.
func (s approvalsSnapshot) carries(tool, mode string) bool {
	if mode == config.ApprovalsAsk || !s.routed(tool, mode) {
		return false
	}
	if mode != config.ApprovalsReview || s.reviewerOn == nil {
		return true
	}
	on, cached := s.reviewers[tool]
	if !cached {
		on = s.reviewerOn(tool)
		if s.reviewers != nil {
			s.reviewers[tool] = on
		}
	}
	return on
}

// modeFor is the approvals mode tool resolves to in folder, and whether
// the tool has a route for it. A policy that could not be read asks.
func (s approvalsSnapshot) modeFor(tool conversations.Tool, folder string) (string, bool) {
	if !s.policyOK {
		return config.ApprovalsAsk, false
	}
	policy := s.policy
	if folder != "" {
		ask, cached := s.projectAsk[folder]
		if !cached {
			var err error
			if ask, err = config.ProjectApprovalsAsk(folder); err != nil {
				ask = folder
			}
			s.projectAsk[folder] = ask
		}
		policy.ProjectAsk = ask
	}
	mode := policy.Resolve(string(tool), folder).Mode
	carried := s.carries(string(tool), mode)
	if mode == config.ApprovalsReview && !carried && s.routed(string(tool), mode) {
		return config.ApprovalsAsk, false
	}
	return mode, carried
}

// headerMode is the loosest mode the home config sets anywhere that a
// listed tool carries out, for the panel header; ask when none loosens
// or the environment turns them off.
func (s approvalsSnapshot) headerMode() string {
	if !s.policyOK || strings.TrimSpace(s.policy.Env) != "" {
		return config.ApprovalsAsk
	}
	home := s.policy.Home
	modes := []string{home.Mode}
	for _, mode := range home.Tools {
		modes = append(modes, mode)
	}
	for _, mode := range home.Projects {
		modes = append(modes, mode)
	}
	loosest := config.ApprovalsAsk
	for _, mode := range modes {
		if approvalsLooseness(mode) > approvalsLooseness(loosest) && slices.ContainsFunc(approvalsListTools, func(tool conversations.Tool) bool { return s.carries(string(tool), mode) }) {
			loosest = mode
		}
	}
	return loosest
}

func approvalsLooseness(mode string) int {
	switch mode {
	case config.ApprovalsReview:
		return 1
	case config.ApprovalsYolo:
		return 2
	}
	return 0
}

// waitVerdict says how a permission wait shows: held, while the hook
// may still answer it, until hold; otherwise shown, with reason when
// approvals have one. since is when the tool began to wait.
func (s approvalsSnapshot) waitVerdict(c conversations.Conversation, since, now time.Time) (reason string, hold time.Time) {
	mode, routed := s.modeFor(c.Tool, c.Folder)
	switch {
	case mode == config.ApprovalsAsk:
		return "", time.Time{}
	case c.App == conversations.AppCloud:
		return approvalsReasonCloud, time.Time{}
	case !routed:
		return fmt.Sprintf(approvalsReasonUnsupportedFmt, cmp.Or(approvalsToolLabels[c.Tool], string(c.Tool))), time.Time{}
	case mode == config.ApprovalsReview:
		// The reviewer decides before the tool shows a prompt, and the
		// hook approves nothing in review, so the wait is the person's.
		return approvalsReasonReviewer, time.Time{}
	}
	last, decided := s.decisionSince(c, since)
	if decided && last.Decision == approvals.Asked {
		return approvalsRuleReason(last.Rule), time.Time{}
	}
	// Codex runs the hook before it shows a prompt, so its wait is real.
	if c.Tool != conversations.ToolClaude {
		return "", time.Time{}
	}
	if decided {
		if until := last.Time.Add(approvalsHookWait); now.Before(until) {
			return "", until
		}
		return approvalsReasonStillWaiting, time.Time{}
	}
	if until := since.Add(approvalsHookWait); now.Before(until) {
		return "", until
	}
	return approvalsReasonNoAnswer, time.Time{}
}

// decisionSince is the decision made during the wait that began at
// since: the latest asked one, since one call left to the person keeps
// the wait open, else the latest. A zero since takes the latest of all.
func (s approvalsSnapshot) decisionSince(c conversations.Conversation, since time.Time) (approvals.Entry, bool) {
	session := s.sessions[approvalSessionKey(string(c.Tool), c.ID)]
	if session == nil {
		return approvals.Entry{}, false
	}
	if since.IsZero() {
		return session.last, true
	}
	var latest approvals.Entry
	decided := false
	for _, e := range session.Decisions {
		if e.Time.Before(since.Add(-approvalsClockSlack)) {
			break
		}
		if e.Decision == approvals.Asked {
			return e, true
		}
		if !decided {
			latest, decided = e, true
		}
	}
	return latest, decided
}

// waitSeen is when the server first and last saw a Claude Code wait
// whose start the tool did not give.
type waitSeen struct{ first, last time.Time }

// applyApprovals adds what approvals decided to each row of view. A
// permission wait the hook may still answer, or already approved, is
// shown as work: it never counts as needing the person, on this page or
// not. firstSeen keeps the waits whose start the tool did not give. It
// returns the end of the earliest hold, zero when none.
func applyApprovals(view *conversations.View, snap approvalsSnapshot, now time.Time, firstSeen map[string]waitSeen) time.Time {
	if mode := snap.headerMode(); mode != config.ApprovalsAsk {
		view.ApprovalsMode = mode
	}
	var earliest time.Time
	held := func(hold time.Time) {
		if earliest.IsZero() || hold.Before(earliest) {
			earliest = hold
		}
	}
	// The scanner may keep the rows for a later page, so they are copied.
	rows := slices.Clone(view.Conversations)
	onPage := map[string]bool{}
	for i := range rows {
		c := &rows[i]
		key := approvalSessionKey(string(c.Tool), c.ID)
		onPage[key] = true
		var note conversations.ApprovalsNote
		if session := snap.sessions[key]; session != nil {
			note.Approved, note.Recent = session.Approved, session.recent
		}
		reason, hold, waits := waitVerdictOf(*c, snap, now, firstSeen)
		if waits {
			note.Reason = reason
			if !hold.IsZero() {
				if c.State == conversations.StateBlocked {
					c.State = conversations.StateWorking
					view.NeedsYou = max(view.NeedsYou-1, 0)
				}
				c.Activity = conversations.ActivityWorking
				c.Waiting = nil
				held(hold)
			}
		}
		if note.Approved > 0 || note.Reason != "" {
			c.Approvals = &note
		}
	}
	view.Conversations = rows
	// A search or a later page leaves rows out that NeedsYou counts.
	for _, c := range view.PermissionWaits {
		if onPage[approvalSessionKey(string(c.Tool), c.ID)] {
			continue
		}
		if _, hold, _ := waitVerdictOf(c, snap, now, firstSeen); !hold.IsZero() {
			if c.State == conversations.StateBlocked {
				view.NeedsYou = max(view.NeedsYou-1, 0)
			}
			held(hold)
		}
	}
	// A wait no list showed for an hour is forgotten.
	for key, seen := range firstSeen {
		if now.Sub(seen.last) > time.Hour {
			delete(firstSeen, key)
		}
	}
	return earliest
}

// waitVerdictOf is waitVerdict for row c, and whether c is a live
// permission wait at all. A Claude Code wait whose start the tool did
// not give starts when the server first saw it, kept in firstSeen.
func waitVerdictOf(c conversations.Conversation, snap approvalsSnapshot, now time.Time, firstSeen map[string]waitSeen) (reason string, hold time.Time, waits bool) {
	key := approvalSessionKey(string(c.Tool), c.ID)
	since, waits := c.PermissionWait()
	waits = waits && c.Live
	if !waits || !since.IsZero() {
		delete(firstSeen, key)
	}
	if !waits {
		return "", time.Time{}, false
	}
	if since.IsZero() && c.Tool == conversations.ToolClaude {
		seen, ok := firstSeen[key]
		if !ok {
			seen.first = now
		}
		seen.last = now
		firstSeen[key] = seen
		since = seen.first
	}
	reason, hold = snap.waitVerdict(c, since, now)
	return reason, hold, true
}

// approvalsReader reads the approvals policy and decision log for the
// conversations list and the approvals routes, keeping the parsed log
// until the log changes.
type approvalsReader struct {
	source func() (string, error)
	home   func() (string, error)
	env    func() string
	now    func() time.Time
	routed func(tool, mode string) bool
	// reviewerOn reports whether tool's own reviewer is on in the user
	// settings under userHome.
	reviewerOn func(userHome, tool string) bool

	mu       sync.Mutex
	logStamp string
	sessions map[string]*approvalSession
	// firstSeen: see applyApprovals.
	firstSeen map[string]waitSeen
	// holds is, per list query, the end of the earliest hold in the
	// last answer to it; that list's tag changes once it passes, so the
	// held row shows without a write.
	holds map[string]time.Time
}

func newApprovalsReader() *approvalsReader {
	return &approvalsReader{
		source:     globalSourceRoot,
		home:       globalUserHome,
		env:        func() string { return os.Getenv(config.EnvApprovals) },
		now:        time.Now,
		routed:     approvalsRouted,
		reviewerOn: approvalsReviewerOn,
	}
}

// approvalsReviewerOn reports whether tool's native reviewer is on in
// its user settings. A tool with no reviewer key leaves it to its route;
// a settings file that cannot be read counts as off.
func approvalsReviewerOn(userHome, tool string) bool {
	rs, native, err := readReviewerState(tool, userHome, globalState{})
	return !native || err == nil && rs.on
}

// reviewerFiles are the user settings files that turn the listed tools'
// reviewers on.
func reviewerFiles(userHome string) []string {
	var paths []string
	for _, tool := range approvalsListTools {
		if _, ok := nativeReviewers[string(tool)]; ok {
			g := globalTargets[string(tool)]
			paths = append(paths, g.path(userHome, g.settings.path))
		}
	}
	return paths
}

// files are the home config files and the decision log, whose changes
// change what approvals add to the list.
func (a *approvalsReader) files() (configs []string, log string, err error) {
	source, err := a.source()
	if err != nil {
		return nil, "", err
	}
	return globalConfigPaths(source), approvals.LogPath(source), nil
}

// stamp names the state of every input of applyApprovals for the list
// that query asks for, for its tag.
func (a *approvalsReader) stamp(query string) string {
	configs, log, err := a.files()
	if err != nil {
		return "unreadable"
	}
	var b strings.Builder
	b.WriteString(fileStamps(append(configs, log, log+".1")...))
	if userHome, err := a.home(); err == nil {
		b.WriteString(fileStamps(reviewerFiles(userHome)...))
	}
	a.mu.Lock()
	hold := a.holds[query]
	a.mu.Unlock()
	if !hold.IsZero() {
		fmt.Fprintf(&b, "hold:%d:%t", hold.UnixNano(), !a.now().Before(hold))
	}
	return b.String()
}

func fileStamps(paths ...string) string {
	var b strings.Builder
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			fmt.Fprintf(&b, "%s:-;", path)
			continue
		}
		fmt.Fprintf(&b, "%s:%d:%d;", path, info.Size(), info.ModTime().UnixNano())
	}
	return b.String()
}

// snapshot reads the policy and, when it changed, the decision log. A
// policy that cannot be read asks; a log that cannot be read is empty.
func (a *approvalsReader) snapshot() approvalsSnapshot {
	snap := approvalsSnapshot{projectAsk: map[string]string{}, routed: a.routed, reviewers: map[string]bool{}}
	userHome, _ := a.home()
	if a.reviewerOn != nil {
		snap.reviewerOn = func(tool string) bool { return a.reviewerOn(userHome, tool) }
	}
	source, err := a.source()
	if err != nil {
		return snap
	}
	if home, err := config.LoadHomeApprovals(source); err == nil {
		snap.policy = config.ApprovalsPolicy{Home: home, UserHome: userHome, Env: a.env()}
		snap.policyOK = true
	}
	log := approvals.LogPath(source)
	stamp := fileStamps(log, log+".1")
	a.mu.Lock()
	defer a.mu.Unlock()
	if stamp != a.logStamp || a.sessions == nil {
		entries, err := approvals.ReadLog(log)
		if err != nil {
			entries = nil
		}
		a.logStamp, a.sessions = stamp, approvalSessionsOf(entries)
	}
	snap.sessions = a.sessions
	return snap
}

// decorate adds what approvals decided to the list the scanner read
// for query.
func (a *approvalsReader) decorate(view *conversations.View, query string) {
	snap := a.snapshot()
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.firstSeen == nil {
		a.firstSeen, a.holds = map[string]waitSeen{}, map[string]time.Time{}
	}
	if hold := applyApprovals(view, snap, now, a.firstSeen); !hold.IsZero() {
		a.holds[query] = hold
	} else {
		delete(a.holds, query)
	}
	// A query nobody asks for again keeps its hold only a while.
	for asked, hold := range a.holds {
		if now.Sub(hold) > time.Minute {
			delete(a.holds, asked)
		}
	}
}

// approvalsView is the read route's answer.
type approvalsView struct {
	// Mode is the loosest mode the home config sets; ask when none.
	Mode string `json:"mode"`
	// Tools is approvals status for the server's working directory.
	Tools []approvalsToolStatus `json:"tools"`
	// Sessions are the sessions in the decision log, latest first.
	Sessions []*approvalSession `json:"sessions"`
	// Problem says why the settings could not be read.
	Problem string `json:"problem,omitempty"`
}

type approvalsAskRequest struct {
	Mode string `json:"mode"`
}

type approvalsAskReply struct {
	Mode string `json:"mode"`
	Path string `json:"path"`
}

// approvalsHandler serves the approvals read route and the one write
// route, which can only turn approvals off.
type approvalsHandler struct {
	reader *approvalsReader
}

func (h approvalsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case approvalsViewPath:
		h.view(w, r)
	case approvalsAskPath:
		h.ask(w, r)
	default:
		writeEditJSON(w, http.StatusNotFound, editError{Error: "no endpoint at " + r.URL.Path})
	}
}

func (h approvalsHandler) view(w http.ResponseWriter, r *http.Request) {
	if !allowGetOnly(w, r) {
		return
	}
	snap := h.reader.snapshot()
	out := approvalsView{Mode: snap.headerMode(), Tools: []approvalsToolStatus{}, Sessions: []*approvalSession{}}
	if status, err := readApprovalsStatus(io.Discard); err != nil {
		out.Problem = err.Error()
	} else {
		out.Tools = status.Tools
	}
	for _, session := range snap.sessions {
		out.Sessions = append(out.Sessions, session)
	}
	slices.SortFunc(out.Sessions, func(a, b *approvalSession) int {
		if c := b.last.Time.Compare(a.last.Time); c != 0 {
			return c
		}
		return strings.Compare(approvalSessionKey(a.Tool, a.Session), approvalSessionKey(b.Tool, b.Session))
	})
	writeEditJSON(w, http.StatusOK, out)
}

// ask sets approvals.mode: ask in the home config's local file. It takes
// no other mode: loosening stays a change a person makes in that file.
func (h approvalsHandler) ask(w http.ResponseWriter, r *http.Request) {
	if status, message := editRequestGuard(r); status != 0 {
		writeEditJSON(w, status, editError{Error: message})
		return
	}
	var req approvalsAskRequest
	if err := decodeOneJSON(w, r, 1<<10, &req); err != nil {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: `send {"mode": "ask"}`})
		return
	}
	if req.Mode != config.ApprovalsAsk {
		writeEditJSON(w, http.StatusBadRequest, editError{Error: "only ask can be set here; review and yolo are set in $AGNOSTIC_AI_HOME/agnostic-ai.yaml"})
		return
	}
	source, err := h.reader.source()
	if err != nil {
		writeEditJSON(w, http.StatusInternalServerError, editError{Error: err.Error()})
		return
	}
	path := globalConfigPaths(source)[1]
	if err := writeApprovalsAsk(path); err != nil {
		writeEditJSON(w, http.StatusConflict, editError{Error: err.Error()})
		return
	}
	writeEditJSON(w, http.StatusOK, approvalsAskReply{Mode: config.ApprovalsAsk, Path: path})
}
