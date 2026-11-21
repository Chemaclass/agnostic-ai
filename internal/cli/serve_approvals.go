package cli

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/adapters/codex"
	"github.com/chemaclass/agnostic-ai/internal/approvals"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/conversations"
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
// come oldest first; one without a session belongs to no row.
func approvalSessionsOf(entries []approvals.Entry) map[string]*approvalSession {
	sessions := map[string]*approvalSession{}
	for _, e := range slices.Backward(entries) {
		if e.Session == "" {
			continue
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
	activated  func(tool, mode string) bool
	active     map[approvalDisplayRoute]bool
}

type approvalDisplayRoute struct{ tool, mode string }

// approvalsListTools are the tools the conversations list shows.
var approvalsListTools = []conversations.Tool{conversations.ToolClaude, conversations.ToolCodex, conversations.ToolCursor, conversations.ToolChatGPT}

// carries reports whether the tool has an active route for mode.
func (s approvalsSnapshot) carries(tool, mode string) bool {
	if mode == config.ApprovalsAsk || !s.routed(tool, mode) {
		return false
	}
	if s.activated == nil {
		return true
	}
	key := approvalDisplayRoute{tool, mode}
	on, cached := s.active[key]
	if !cached {
		on = s.activated(tool, mode)
		s.active[key] = on
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
			ask = displayProjectAsk(folder)
			s.projectAsk[folder] = ask
		}
		policy.ProjectAsk = ask
	}
	mode := policy.Resolve(string(tool), folder).Mode
	carried := s.carries(string(tool), mode)
	if !carried && s.routed(string(tool), mode) {
		return config.ApprovalsAsk, false
	}
	return mode, carried
}

func displayProjectAsk(folder string) string {
	ask, err := config.ProjectApprovalsAsk(folder)
	if err != nil {
		return folder
	}
	return ask
}

// headerMode is the loosest active mode for the project. The global
// list considers the home policy for all listed tools and projects.
func (s approvalsSnapshot) headerMode(folder string) string {
	if !s.policyOK || strings.TrimSpace(s.policy.Env) != "" {
		return config.ApprovalsAsk
	}
	loosest := config.ApprovalsAsk
	considerFolder := func(path string) {
		for _, tool := range approvalsListTools {
			if mode, active := s.modeFor(tool, path); active {
				loosest = looserApprovalsMode(loosest, mode)
			}
		}
	}
	if folder != "" {
		considerFolder(folder)
		return loosest
	}
	home := s.policy.Home
	consider := func(mode string, tools ...conversations.Tool) {
		if approvalsLooseness(mode) > approvalsLooseness(loosest) && slices.ContainsFunc(tools, func(tool conversations.Tool) bool { return s.carries(string(tool), mode) }) {
			loosest = mode
		}
	}
	var unnamed []conversations.Tool
	for _, tool := range approvalsListTools {
		if mode := home.Tools[string(tool)]; mode != "" {
			consider(mode, tool)
		} else {
			unnamed = append(unnamed, tool)
		}
	}
	consider(home.Mode, unnamed...)
	for _, path := range s.policy.ProjectFolders() {
		considerFolder(path)
		if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
			considerFolder(resolved)
		}
	}
	return loosest
}

func looserApprovalsMode(a, b string) string {
	if approvalsLooseness(b) > approvalsLooseness(a) {
		return b
	}
	return a
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
		// The approvals hook approves nothing in review, so no answer
		// is coming: the wait is the person's.
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

// applyApprovals adds what approvals decided to each row of view. A
// permission wait the hook may still answer, or already approved, is
// shown as work: it never counts as needing the person, on this page or
// not. firstSeen keeps the waits whose start the tool did not give. It
// returns the end of the earliest hold, zero when none.
func applyApprovals(view *conversations.View, snap approvalsSnapshot, folder string, now time.Time, firstSeen map[string]time.Time) time.Time {
	if mode := snap.headerMode(folder); mode != config.ApprovalsAsk {
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
	waiting := map[string]bool{}
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
			waiting[key] = true
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
		key := approvalSessionKey(string(c.Tool), c.ID)
		waiting[key] = true
		if onPage[key] {
			continue
		}
		if _, hold, _ := waitVerdictOf(c, snap, now, firstSeen); !hold.IsZero() {
			if c.State == conversations.StateBlocked {
				view.NeedsYou = max(view.NeedsYou-1, 0)
			}
			held(hold)
		}
	}
	for key := range firstSeen {
		if !waiting[key] {
			delete(firstSeen, key)
		}
	}
	return earliest
}

// waitVerdictOf is waitVerdict for row c, and whether c is a live
// permission wait at all. A Claude Code wait whose start the tool did
// not give starts when the server first saw it, kept in firstSeen.
func waitVerdictOf(c conversations.Conversation, snap approvalsSnapshot, now time.Time, firstSeen map[string]time.Time) (reason string, hold time.Time, waits bool) {
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
		since = firstSeen[key]
		if since.IsZero() {
			since = now
			firstSeen[key] = since
		}
	}
	reason, hold = snap.waitVerdict(c, since, now)
	return reason, hold, true
}

// approvalsReader reads the approvals policy and decision log for the
// conversations list and the approvals routes, keeping the parsed log
// until the log changes.
type approvalsReader struct {
	source    func() (string, error)
	home      func() (string, error)
	env       func() string
	now       func() time.Time
	routed    func(tool, mode string) bool
	activated func(tool, mode, home string) bool

	mu       sync.Mutex
	logStamp string
	sessions map[string]*approvalSession
	// firstSeen: see applyApprovals.
	firstSeen map[string]time.Time
	// holds is, per list query, the end of the earliest hold in the
	// last answer to it; that list's tag changes once it passes, so the
	// held row shows without a write.
	holds          map[string]time.Time
	projectFolders []string
}

func newApprovalsReader() *approvalsReader {
	return &approvalsReader{
		source:    userSourceRoot,
		home:      userHomeDir,
		env:       func() string { return os.Getenv(config.EnvApprovals) },
		now:       time.Now,
		routed:    approvalsRouted,
		activated: approvalsActivated,
	}
}

func approvalsActivated(tool, mode, home string) bool {
	if mode == config.ApprovalsYolo {
		return approvalsToolStatusFor(tool, config.ApprovalDecision{Mode: mode}).Effective == mode
	}
	if home == "" {
		return false
	}
	rs, native, err := readReviewerState(tool, home, userState{})
	if err != nil || !native || !rs.on {
		return false
	}
	if off := rs.reviewer.off; off != nil {
		if value, ok := settingsLookup(rs.values, off.key, userTargets[tool].settings.format); ok && sameSetting(value, off.value) {
			return false
		}
	}
	return rs.reviewer.overrideEnv == "" || strings.TrimSpace(os.Getenv(rs.reviewer.overrideEnv)) == ""
}

// reviewerFiles are the user settings files that turn the listed tools'
// reviewers on.
func reviewerFiles(userHome string) []string {
	var paths []string
	for _, tool := range approvalsListTools {
		if _, ok := nativeReviewers[string(tool)]; ok {
			g := userTargets[string(tool)]
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
	return userConfigPaths(source), approvals.LogPath(source), nil
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
	if path, ok := claudeUserSettingsPath(); ok {
		b.WriteString(fileStamps(path))
	}
	if path, err := codex.UserHooksPath(); err == nil {
		b.WriteString(fileStamps(path, filepath.Join(filepath.Dir(path), "config.toml")))
	}
	fmt.Fprintf(&b, "env:%q;", a.env())
	a.mu.Lock()
	hold, folders := a.holds[query], a.projectFolders
	a.mu.Unlock()
	for _, folder := range folders {
		resolved, _ := filepath.EvalSymlinks(folder)
		fmt.Fprintf(&b, "%q:%q:%q;", folder, resolved, displayProjectAsk(folder))
	}
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
	snap := approvalsSnapshot{projectAsk: map[string]string{}, routed: a.routed, active: map[approvalDisplayRoute]bool{}}
	userHome, _ := a.home()
	if a.activated != nil {
		snap.activated = func(tool, mode string) bool { return a.activated(tool, mode, userHome) }
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
func (a *approvalsReader) decorate(view *conversations.View, folder, query string) {
	snap := a.snapshot()
	now := a.now()
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.firstSeen == nil {
		a.firstSeen, a.holds = map[string]time.Time{}, map[string]time.Time{}
	}
	if hold := applyApprovals(view, snap, folder, now, a.firstSeen); !hold.IsZero() {
		a.holds[query] = hold
	} else {
		delete(a.holds, query)
	}
	a.projectFolders = slices.Sorted(maps.Keys(snap.projectAsk))
	// A query nobody asks for again keeps its hold only a while.
	for asked, hold := range a.holds {
		if now.Sub(hold) > time.Minute {
			delete(a.holds, asked)
		}
	}
}

// approvalsView is the read route's answer.
type approvalsView struct {
	// Mode is the loosest effective mode for the server directory.
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
	out := approvalsView{Mode: config.ApprovalsAsk, Tools: []approvalsToolStatus{}, Sessions: []*approvalSession{}}
	if status, err := readApprovalsStatus(io.Discard); err != nil {
		out.Problem = err.Error()
	} else {
		out.Tools = status.Tools
		home, _ := h.reader.home()
		for i := range out.Tools {
			tool := &out.Tools[i]
			if tool.Effective == config.ApprovalsReview && !h.reader.activated(tool.Tool, tool.Effective, home) {
				tool.Effective, tool.Route = config.ApprovalsAsk, ""
				tool.Reason = "the tool's reviewer is inactive"
			}
			out.Mode = looserApprovalsMode(out.Mode, tool.Effective)
		}
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
	path := userConfigPaths(source)[1]
	if err := writeApprovalsAsk(path); err != nil {
		writeEditJSON(w, http.StatusConflict, editError{Error: err.Error()})
		return
	}
	writeEditJSON(w, http.StatusOK, approvalsAskReply{Mode: config.ApprovalsAsk, Path: path})
}
