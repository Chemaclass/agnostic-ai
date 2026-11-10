package conversations

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeToken stands in for a sign-in token. No test prints it, and each
// failure checks that its error does not hold it either.
const fakeToken = "test-token-not-a-secret"

type fakeCredentials struct {
	token string
	err   error
}

func (f fakeCredentials) AccessToken(context.Context) (string, error) { return f.token, f.err }

// cloudAPI serves answer from a fake API and refuses a request that
// lacks the headers the real one needs.
func cloudAPI(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) *ClaudeCloud {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/code/sessions" || r.URL.Query().Get("limit") != "100" {
			http.Error(w, "unexpected path "+r.URL.String(), http.StatusTeapot)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+fakeToken || r.Header.Get("anthropic-version") != "2023-06-01" {
			http.Error(w, "missing the sign-in headers", http.StatusTeapot)
			return
		}
		answer(w, r)
	}))
	t.Cleanup(server.Close)
	client := newCloudClient()
	return &ClaudeCloud{BaseURL: server.URL, Client: client, Credentials: fakeCredentials{token: fakeToken}}
}

func answerWith(status int, body string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}
}

func TestClaudeCloud_ListsSessionsAsConversations(t *testing.T) {
	cloud := cloudAPI(t, answerWith(http.StatusOK, `{"data": [
		{"id": "cse_working01", "title": "Fix the parser", "status_bucket": "working", "worker_status": "running", "created_at": "2026-11-09T08:00:00.123Z", "updated_at": "2026-11-09T09:00:00Z", "last_event_at": "2026-11-09T09:30:00.5Z", "user_message_count": 0},
		{"id": "cse_blocked01", "title": "Add a test", "status_bucket": "blocked", "updated_at": "2026-11-09T07:00:00Z"},
		{"id": "cse_review01", "title": "Rename the flag", "status_bucket": "review_ready", "last_event_at": "2026-11-08T07:00:00+02:00"},
		{"id": "cse_done01", "title": "Ship it", "status_bucket": "completed", "last_event_at": "2026-11-07T07:00:00Z"},
		{"id": "cse_new01", "title": "Something new", "status_bucket": "paused", "last_event_at": "2026-11-06T07:00:00Z"}
	], "resume_token": "next"}`))
	got, err := cloud.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := `[
  {
    "tool": "claude",
    "app": "cloud",
    "id": "cse_working01",
    "title": "Fix the parser",
    "startedAt": "2026-11-09T08:00:00Z",
    "updatedAt": "2026-11-09T09:30:00Z",
    "live": true,
    "activity": "working",
    "state": "",
    "open": {
      "kind": "link",
      "label": "Open on claude.ai",
      "link": "https://claude.ai/code/session_working01"
    }
  },
  {
    "tool": "claude",
    "app": "cloud",
    "id": "cse_blocked01",
    "title": "Add a test",
    "updatedAt": "2026-11-09T07:00:00Z",
    "live": true,
    "activity": "waiting",
    "state": "",
    "open": {
      "kind": "link",
      "label": "Open on claude.ai",
      "link": "https://claude.ai/code/session_blocked01"
    }
  },
  {
    "tool": "claude",
    "app": "cloud",
    "id": "cse_review01",
    "title": "Rename the flag",
    "updatedAt": "2026-11-08T05:00:00Z",
    "state": "",
    "open": {
      "kind": "link",
      "label": "Open on claude.ai",
      "link": "https://claude.ai/code/session_review01"
    }
  },
  {
    "tool": "claude",
    "app": "cloud",
    "id": "cse_done01",
    "title": "Ship it",
    "updatedAt": "2026-11-07T07:00:00Z",
    "state": "",
    "open": {
      "kind": "link",
      "label": "Open on claude.ai",
      "link": "https://claude.ai/code/session_done01"
    }
  },
  {
    "tool": "claude",
    "app": "cloud",
    "id": "cse_new01",
    "title": "Something new",
    "updatedAt": "2026-11-06T07:00:00Z",
    "state": "",
    "open": {
      "kind": "link",
      "label": "Open on claude.ai",
      "link": "https://claude.ai/code/session_new01"
    }
  }
]`
	if text := asJSON(t, got); text != want {
		t.Errorf("rows =\n%s\nwant\n%s", text, want)
	}
}

func TestClaudeCloud_SaysWhyTheListCouldNotBeRead(t *testing.T) {
	slow := func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}
	cases := []struct {
		name   string
		answer func(http.ResponseWriter, *http.Request)
		reason CloudReason
		text   string
		cause  string
	}{
		{"an expired sign-in", answerWith(http.StatusUnauthorized, `{"error":{"message":"token expired"}}`), CloudSignedOut,
			"the Claude sign-in has expired or was refused. Run `claude` once to renew it.", `HTTP 401: {"error":{"message":"token expired"}}`},
		{"a refusal", answerWith(http.StatusForbidden, `forbidden`), CloudRefused,
			"claude.ai refused to list them.", "HTTP 403: forbidden"},
		{"a server error", answerWith(http.StatusInternalServerError, `overloaded`), CloudRefused,
			"claude.ai refused to list them.", "HTTP 500: overloaded"},
		{"a redirect, which is not followed", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://example.com/elsewhere", http.StatusFound)
		}, CloudRefused, "claude.ai refused to list them.", "HTTP 302:"},
		{"no answer in time", slow, CloudUnreachable, "claude.ai did not answer in time.", "Client.Timeout exceeded"},
		{"an answer that is not JSON", answerWith(http.StatusOK, `<html>`), CloudUnreadable,
			"claude.ai answered with something this version cannot read.", "session list: invalid character '<'"},
		{"an answer without data", answerWith(http.StatusOK, `{"sessions": []}`), CloudUnreadable,
			"claude.ai answered with something this version cannot read.", "session list has no data field"},
		{"a row with an id of another kind", answerWith(http.StatusOK, `{"data": [{"id": "../etc", "last_event_at": "2026-11-09T09:00:00Z"}]}`), CloudUnreadable,
			"claude.ai answered with something this version cannot read.", `session list row 0: id "../etc" is not a cloud session id`},
		{"a row with no time", answerWith(http.StatusOK, `{"data": [{"id": "cse_a1"}]}`), CloudUnreadable,
			"claude.ai answered with something this version cannot read.", "session list row 0: neither last_event_at nor updated_at is set"},
		{"a row with a time of another form", answerWith(http.StatusOK, `{"data": [{"id": "cse_a1", "last_event_at": "yesterday"}]}`), CloudUnreadable,
			"claude.ai answered with something this version cannot read.", `session list row 0: time "yesterday"`},
	}
	for _, tc := range cases {
		cloud := cloudAPI(t, tc.answer)
		cloud.Client.Timeout = 100 * time.Millisecond
		_, err := cloud.List(context.Background())
		var cloudErr *CloudError
		if !errors.As(err, &cloudErr) {
			t.Errorf("%s: error = %v, want a *CloudError", tc.name, err)
			continue
		}
		if cloudErr.Reason != tc.reason || err.Error() != tc.text {
			t.Errorf("%s: %s %q, want %s %q", tc.name, cloudErr.Reason, err.Error(), tc.reason, tc.text)
		}
		cause := fmt.Sprint(cloudErr.Cause)
		if !strings.Contains(cause, tc.cause) {
			t.Errorf("%s: cause %q, want it to hold %q", tc.name, cause, tc.cause)
		}
		if strings.Contains(cause, fakeToken) {
			t.Errorf("%s: the cause holds the token", tc.name)
		}
	}
}

func TestClaudeCloud_AsksNothingWithoutASignIn(t *testing.T) {
	asked := false
	cloud := cloudAPI(t, func(http.ResponseWriter, *http.Request) { asked = true })
	cloud.Credentials = fakeCredentials{err: &CloudError{Tool: ToolClaude, Reason: CloudNoCredential, Cause: errors.New("no entry")}}
	_, err := cloud.List(context.Background())
	if err == nil || err.Error() != "no Claude sign-in that may list cloud sessions was found on this machine. Sign in with `claude` first." {
		t.Errorf("error = %v", err)
	}
	if asked {
		t.Error("the API was asked without a sign-in")
	}
}

func TestKeychainCredentials_ReadsOnlyASignInThatMayListSessions(t *testing.T) {
	entry := func(json string) func(context.Context) ([]byte, error) {
		return func(context.Context) ([]byte, error) { return []byte(json), nil }
	}
	failing := func(context.Context) ([]byte, error) {
		return nil, errors.New("security: The specified item could not be found in the keychain.")
	}
	cases := []struct {
		name  string
		creds KeychainCredentials
		text  string
		cause string
	}{
		{"another system", KeychainCredentials{GOOS: "linux", Read: entry(`{}`)},
			"this system has no supported place to read the Claude sign-in from; only macOS is supported.", "no Claude sign-in store known on linux"},
		{"no keychain entry", KeychainCredentials{GOOS: "darwin", Read: failing},
			"no Claude sign-in that may list cloud sessions was found on this machine. Sign in with `claude` first.", "could not be found in the keychain"},
		{"an entry that is not JSON", KeychainCredentials{GOOS: "darwin", Read: entry(`not json ` + fakeToken)},
			"no Claude sign-in that may list cloud sessions was found on this machine. Sign in with `claude` first.", `keychain entry "Claude Code-credentials" is not the JSON Claude Code writes`},
		{"an entry without a claude.ai sign-in", KeychainCredentials{GOOS: "darwin", Read: entry(`{"other": {}}`)},
			"no Claude sign-in that may list cloud sessions was found on this machine. Sign in with `claude` first.", `keychain entry "Claude Code-credentials" holds no claude.ai sign-in`},
		{"a sign-in without the sessions scope", KeychainCredentials{GOOS: "darwin", Read: entry(`{"claudeAiOauth": {"accessToken": "` + fakeToken + `", "scopes": ["user:inference"]}}`)},
			"no Claude sign-in that may list cloud sessions was found on this machine. Sign in with `claude` first.", "the claude.ai sign-in lacks the user:sessions:claude_code scope"},
	}
	for _, tc := range cases {
		token, err := tc.creds.AccessToken(context.Background())
		var cloudErr *CloudError
		if token != "" || !errors.As(err, &cloudErr) {
			t.Errorf("%s: got a token or a plain error: %v", tc.name, err)
			continue
		}
		if err.Error() != tc.text {
			t.Errorf("%s: error %q, want %q", tc.name, err.Error(), tc.text)
		}
		cause := fmt.Sprint(cloudErr.Cause)
		if !strings.Contains(cause, tc.cause) {
			t.Errorf("%s: cause %q, want it to hold %q", tc.name, cause, tc.cause)
		}
		if strings.Contains(cause, fakeToken) {
			t.Errorf("%s: the cause holds the token", tc.name)
		}
	}
	creds := KeychainCredentials{GOOS: "darwin", Read: entry(`{"claudeAiOauth": {"accessToken": "` + fakeToken + `", "refreshToken": "never-used", "expiresAt": 1, "scopes": ["user:inference", "user:sessions:claude_code"]}}`)}
	token, err := creds.AccessToken(context.Background())
	if err != nil || token != fakeToken {
		t.Errorf("a signed-in entry gave an error %v, or another token", err)
	}
}

// countingCloud answers each List with the next answer and counts the calls.
type countingCloud struct {
	calls   int
	answers []func() ([]Conversation, error)
}

func (c *countingCloud) List(context.Context) ([]Conversation, error) {
	answer := c.answers[min(c.calls, len(c.answers)-1)]
	c.calls++
	return answer()
}

func cloudRow(id, title, updated string) Conversation {
	return Conversation{Tool: ToolClaude, App: AppCloud, ID: id, Title: title, UpdatedAt: updated, Open: Route{Kind: RouteLink, Label: "Open on claude.ai", Link: cloudSessionLink(id)}}
}

func TestScan_ListsCloudSessionsWithTheLocalOnesAndAsksOncePerSpan(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeMarkedSession(t, stores, idSafari, "2026-11-07T11:00:00Z")
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return at }
	cloud := &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) {
			return []Conversation{cloudRow("cse_a1", "Fix the parser", "2026-11-07T11:30:00Z")}, nil
		},
		func() ([]Conversation, error) {
			return nil, &CloudError{Tool: ToolClaude, Reason: CloudSignedOut, Cause: errors.New("HTTP 401: expired")}
		},
	}}
	scanner.Cloud = cloud

	view := scanner.Scan(context.Background(), ScanOptions{})
	if row := byID(t, view, "cse_a1"); row.App != AppCloud || row.Title != "Fix the parser" || row.State != StateActive {
		t.Errorf("cloud row = %s", asJSON(t, row))
	}
	byID(t, view, idSafari)
	if view.CloudNote != "" || view.Tools[0].Count != 2 || view.Tools[0].Shown != 2 {
		t.Errorf("note %q, Claude Code source %s", view.CloudNote, asJSON(t, view.Tools[0]))
	}
	scanner.Scan(context.Background(), ScanOptions{Tool: ToolClaude})
	if _, ok := scanner.Find(context.Background(), ToolClaude, "cse_a1", ScanOptions{}); !ok || cloud.calls != 1 {
		t.Errorf("found %v, after %d calls; want one call in the span", ok, cloud.calls)
	}

	// The next span asks again, and the failure shows no old list.
	at = at.Add(freshSpan)
	view = scanner.Scan(context.Background(), ScanOptions{})
	if cloud.calls != 2 || len(view.Conversations) != 1 || view.Conversations[0].ID != idSafari {
		t.Errorf("after %d calls the list is %s", cloud.calls, asJSON(t, view.Conversations))
	}
	if want := "Claude Code cloud sessions are not listed: the Claude sign-in has expired or was refused. Run `claude` once to renew it."; view.CloudNote != want {
		t.Errorf("note = %q, want %q", view.CloudNote, want)
	}
	scanner.Scan(context.Background(), ScanOptions{})
	if cloud.calls != 2 {
		t.Errorf("a failure was asked again within its span: %d calls", cloud.calls)
	}
}

func TestScan_WithoutACloudSourceSaysNothingAboutTheCloud(t *testing.T) {
	scanner, stores := testScanner(t, noSQLite, fakeProcesses{})
	writeMarkedSession(t, stores, idSafari, "2026-11-07T11:00:00Z")
	if view := scanner.Scan(context.Background(), ScanOptions{}); view.CloudNote != "" || len(view.Conversations) != 1 {
		t.Errorf("note %q, rows %s", view.CloudNote, asJSON(t, view.Conversations))
	}
}

func TestScan_AProjectListNeverAsksTheCloud(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	cloud := &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) {
			return []Conversation{cloudRow("cse_a1", "Fix the parser", "2026-11-07T11:30:00Z")}, nil
		},
	}}
	scanner.Cloud = cloud
	view := scanner.Scan(context.Background(), ScanOptions{Within: []string{"/tmp/app"}})
	if cloud.calls != 0 || len(view.Conversations) != 0 || view.CloudNote != "" {
		t.Errorf("%d calls, note %q, rows %s", cloud.calls, view.CloudNote, asJSON(t, view.Conversations))
	}
}

func TestScan_KeepsNoCloudAnswerFromACanceledScan(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	cloud := &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) {
			return nil, &CloudError{Tool: ToolClaude, Reason: CloudUnreachable, Cause: context.Canceled}
		},
	}}
	scanner.Cloud = cloud
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	scanner.Scan(ctx, ScanOptions{})
	scanner.Scan(context.Background(), ScanOptions{})
	if cloud.calls != 2 {
		t.Errorf("a canceled read was kept: %d calls", cloud.calls)
	}
}

func TestScan_MarksAndHidesCloudSessionsByTheirID(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "marks.json")
	scanner.Cloud = &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) {
			return []Conversation{
				cloudRow("cse_a1", "Fix the parser", "2026-11-07T11:30:00Z"),
				cloudRow("cse_b2", "Add a test", "2026-11-07T11:40:00Z"),
			}, nil
		},
	}}
	if err := scanner.Mark(ToolClaude, "cse_a1", MarkHidden); err != nil {
		t.Fatal(err)
	}
	if err := scanner.Mark(ToolClaude, "cse_b2", MarkCompleted); err != nil {
		t.Fatal(err)
	}
	view := scanner.Scan(context.Background(), ScanOptions{})
	if len(view.Conversations) != 1 || view.Conversations[0].ID != "cse_b2" || view.Conversations[0].State != StateCompleted {
		t.Errorf("rows = %s", asJSON(t, view.Conversations))
	}
}

func TestOpen_SendsACloudSessionToItsOwnPage(t *testing.T) {
	row, err := cloudConversation(claudeCloudRow{ID: "cse_01AbCdEf", Title: "Fix the parser", LastEventAt: "2026-11-07T11:30:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	launcher := &fakeLauncher{}
	reply := Open(context.Background(), row, fakeProcesses{}, launcher)
	want := OpenReply{Opened: true, Message: "Opened on claude.ai."}
	if reply != want || strings.Join(launcher.calls, ";") != "link https://claude.ai/code/session_01AbCdEf" {
		t.Errorf("reply %+v after %v", reply, launcher.calls)
	}
	launcher = &fakeLauncher{err: errNotHere}
	reply = Open(context.Background(), row, fakeProcesses{}, launcher)
	want = OpenReply{Message: "Could not open a browser. This session runs in the cloud: open https://claude.ai/code/session_01AbCdEf"}
	if reply != want {
		t.Errorf("reply %+v", reply)
	}

	row.Open.Link = "https://example.com/session_01AbCdEf"
	launcher = &fakeLauncher{}
	reply = Open(context.Background(), row, fakeProcesses{}, launcher)
	want = OpenReply{Message: "This conversation runs in the cloud and its link is not one this version opens. Find it in https://claude.ai/code by its title."}
	if reply != want || len(launcher.calls) != 0 {
		t.Errorf("reply %+v after %v", reply, launcher.calls)
	}
}

func TestCheckedLink_OpensACloudSessionPageAndNoOtherWebPage(t *testing.T) {
	if err := checkedLink("https://claude.ai/code/session_01AbCdEf"); err != nil {
		t.Error(err)
	}
	for _, link := range []string{"https://claude.ai/code", "https://claude.ai/code/other", "https://claude.ai/code/session_01/../x", "https://claude.ai", "http://claude.ai/code/session_01AbCdEf", "https://claude.ai.example.com/code/session_01AbCdEf"} {
		if err := checkedLink(link); err == nil {
			t.Errorf("%s must be refused", link)
		}
	}
}

func TestClaudeCloud_DoesNotBelieveAWorkingBucketWithoutARunningWorker(t *testing.T) {
	// A session that failed keeps status_bucket working for weeks.
	row, err := cloudConversation(claudeCloudRow{ID: "cse_a1", StatusBucket: "working", WorkerStatus: "WORKER_STATUS_UNSPECIFIED", LastEventAt: "2026-11-07T11:30:00Z"})
	if err != nil || row.Live || row.Activity != "" {
		t.Errorf("live %t, activity %q, error %v; want a session that is not live", row.Live, row.Activity, err)
	}
	row, err = cloudConversation(claudeCloudRow{ID: "cse_a1", StatusBucket: "working", WorkerStatus: "running", LastEventAt: "2026-11-07T11:30:00Z"})
	if err != nil || !row.Live || row.Activity != ActivityWorking {
		t.Errorf("live %t, activity %q, error %v; want a working session", row.Live, row.Activity, err)
	}
}

func TestScan_BelievesACloudStatusForADayAfterItsLastWrite(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return at }
	waiting := func(id, updated string) Conversation {
		c := cloudRow(id, "Add a test", updated)
		c.Live, c.Activity = true, ActivityWaiting
		return c
	}
	scanner.Cloud = &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) {
			return []Conversation{waiting("cse_fresh", "2026-11-07T11:30:00Z"), waiting("cse_stale", "2026-10-11T11:30:00Z")}, nil
		},
	}}
	// The marks begin before the fresh session's last write.
	if err := writeMarks(scanner.MarksPath, marksFile{SchemaVersion: marksSchemaVersion, Since: "2026-11-07T10:00:00Z", Marks: map[string]markEntry{}}); err != nil {
		t.Fatal(err)
	}

	view := scanner.Scan(context.Background(), ScanOptions{Query: "add a test"})

	if fresh := byID(t, view, "cse_fresh"); fresh.State != StateBlocked {
		t.Errorf("a session that asked half an hour ago: state %s", fresh.State)
	}
	if stale := byID(t, view, "cse_stale"); stale.Live || stale.State != StateDormant {
		t.Errorf("a session that asked four weeks ago: live %t, state %s; want it archived", stale.Live, stale.State)
	}

	// Opening it is looking at it: it leaves the count until it writes again.
	if _, found := scanner.Reopen(context.Background(), ToolClaude, "cse_fresh", ScanOptions{}, &fakeLauncher{}); !found {
		t.Fatal("the fresh session was not found")
	}
	view = scanner.Scan(context.Background(), ScanOptions{})
	if fresh := byID(t, view, "cse_fresh"); fresh.State != StateActive || view.NeedsYou != 0 {
		t.Errorf("after opening it: state %s, needsYou %d", fresh.State, view.NeedsYou)
	}
}
