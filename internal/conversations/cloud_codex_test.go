package conversations

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fakeAccount = "account-not-a-real-one"

type fakeCodexLogin struct {
	found bool
	err   error
}

func (f fakeCodexLogin) Login() (CodexLogin, bool, error) {
	return CodexLogin{AccessToken: fakeToken, AccountID: fakeAccount}, f.found, f.err
}

// codexAPI serves answer from a fake Codex cloud and refuses a request
// that lacks the headers the real one needs.
func codexAPI(t *testing.T, answer func(w http.ResponseWriter, r *http.Request)) *CodexCloud {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/threads" || r.URL.Query().Get("limit") != "100" {
			http.Error(w, "unexpected path "+r.URL.String(), http.StatusTeapot)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+fakeToken || r.Header.Get("ChatGPT-Account-Id") != fakeAccount || r.Header.Get("X-OpenAI-Product-Sku") != "codex" {
			http.Error(w, "missing the sign-in headers", http.StatusTeapot)
			return
		}
		answer(w, r)
	}))
	t.Cleanup(server.Close)
	return &CodexCloud{BaseURL: server.URL, Client: newCloudClient(), Credentials: fakeCodexLogin{found: true}}
}

const (
	codexIdle    = "01a1c27f-183f-73fb-b239-000000000001"
	codexBusy    = "01a1c27f-183f-73fb-b239-000000000002"
	codexWaiting = "01a1c27f-183f-73fb-b239-000000000003"
	codexNew     = "01a1c27f-183f-73fb-b239-000000000004"
)

func TestCodexCloud_ListsThreadsAsConversations(t *testing.T) {
	cloud := codexAPI(t, answerWith(http.StatusOK, `{"data": [
		{"id": "`+codexIdle+`", "name": "Fix the parser", "status": {"type": "idle"}, "createdAt": 1794264000, "updatedAt": 1794264100, "recencyAt": 1794264200, "cwd": "/workspace"},
		{"id": "`+codexBusy+`", "name": "Add a test", "status": {"type": "active", "activeFlags": []}, "updatedAt": 1794264300},
		{"id": "`+codexWaiting+`", "name": "Rename the flag", "status": {"type": "active", "activeFlags": ["waitingOnApproval"]}, "recencyAt": 1794264400},
		{"id": "`+codexNew+`", "name": "Something new", "status": {"type": "paused"}, "recencyAt": 1794264500}
	], "nextCursor": null}`))

	rows, err := cloud.List(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	type shown struct {
		id, title, started, updated string
		live                        bool
		activity                    Activity
	}
	want := []shown{
		{codexIdle, "Fix the parser", "2026-11-09T22:40:00Z", "2026-11-09T22:43:20Z", false, ""},
		{codexBusy, "Add a test", "", "2026-11-09T22:45:00Z", true, ActivityWorking},
		{codexWaiting, "Rename the flag", "", "2026-11-09T22:46:40Z", true, ActivityWaiting},
		// A status this version does not know is not live.
		{codexNew, "Something new", "", "2026-11-09T22:48:20Z", false, ""},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %s", asJSON(t, rows))
	}
	for i, row := range rows {
		got := shown{row.ID, row.Title, row.StartedAt, row.UpdatedAt, row.Live, row.Activity}
		if got != want[i] {
			t.Errorf("row %d = %+v, want %+v", i, got, want[i])
		}
		wantRoute := Route{Kind: RouteLink, Label: "Open in Codex app", Link: "codex://threads/" + row.ID + "?hostId=durable"}
		if row.Tool != ToolCodex || row.App != AppCloud || row.Folder != "" || row.Open != wantRoute {
			t.Errorf("row %d: tool %s, app %s, folder %q, route %+v", i, row.Tool, row.App, row.Folder, row.Open)
		}
	}
}

func TestCodexCloud_SaysWhyTheListCouldNotBeRead(t *testing.T) {
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
		{"an expired sign-in", answerWith(http.StatusUnauthorized, `{"detail":"token expired"}`), CloudSignedOut,
			"the Codex sign-in has expired or was refused. Run `codex` once to renew it.", `HTTP 401: {"detail":"token expired"}`},
		{"a refusal", answerWith(http.StatusForbidden, `forbidden`), CloudRefused,
			"the Codex cloud refused to list them.", "HTTP 403: forbidden"},
		{"a redirect, which is not followed", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://example.com/elsewhere", http.StatusFound)
		}, CloudRefused, "the Codex cloud refused to list them.", "HTTP 302:"},
		{"no answer in time", slow, CloudUnreachable, "the Codex cloud did not answer in time.", "Client.Timeout exceeded"},
		{"an answer that is not JSON", answerWith(http.StatusOK, `<html>`), CloudUnreadable,
			"the Codex cloud answered with something this version cannot read.", "thread list: invalid character '<'"},
		{"an answer without data", answerWith(http.StatusOK, `{"threads": []}`), CloudUnreadable,
			"the Codex cloud answered with something this version cannot read.", "thread list has no data field"},
		{"a row with an id of another kind", answerWith(http.StatusOK, `{"data": [{"id": "../etc", "recencyAt": 1794264000}]}`), CloudUnreadable,
			"the Codex cloud answered with something this version cannot read.", `thread list row 0: id "../etc" is not a cloud thread id`},
		{"a row with no time", answerWith(http.StatusOK, `{"data": [{"id": "`+codexIdle+`"}]}`), CloudUnreadable,
			"the Codex cloud answered with something this version cannot read.", "thread list row 0: neither recencyAt nor updatedAt is set"},
		{"a row with a time of another form", answerWith(http.StatusOK, `{"data": [{"id": "`+codexIdle+`", "recencyAt": "yesterday"}]}`), CloudUnreadable,
			"the Codex cloud answered with something this version cannot read.", "thread list: json: cannot unmarshal string"},
	}
	for _, tc := range cases {
		cloud := codexAPI(t, tc.answer)
		cloud.Client.Timeout = 100 * time.Millisecond
		_, err := cloud.List(context.Background())
		var cloudErr *CloudError
		if !errors.As(err, &cloudErr) {
			t.Errorf("%s: error = %v, want a *CloudError", tc.name, err)
			continue
		}
		if cloudErr.Tool != ToolCodex || cloudErr.Reason != tc.reason || err.Error() != tc.text {
			t.Errorf("%s: %s %s %q, want %s %q", tc.name, cloudErr.Tool, cloudErr.Reason, err.Error(), tc.reason, tc.text)
		}
		cause := fmt.Sprint(cloudErr.Cause)
		if !strings.Contains(cause, tc.cause) {
			t.Errorf("%s: cause %q, want it to hold %q", tc.name, cause, tc.cause)
		}
		if strings.Contains(cause, fakeToken) || strings.Contains(cause, fakeAccount) {
			t.Errorf("%s: the cause holds the login", tc.name)
		}
	}
}

func TestCodexCloud_AsksNothingAndSaysNothingWithoutACodexLogin(t *testing.T) {
	asked := false
	cloud := codexAPI(t, func(http.ResponseWriter, *http.Request) { asked = true })
	cloud.Credentials = fakeCodexLogin{found: false}

	rows, err := cloud.List(context.Background())

	if err != nil || len(rows) != 0 || asked {
		t.Errorf("rows %v, error %v, asked the API: %t; a person who does not use Codex has no threads and no note", rows, err, asked)
	}
}

func TestCodexAuthFile_ReadsOnlyAChatGPTLogin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	read := func(content string) (CodexLogin, bool, error) {
		t.Helper()
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		return CodexAuthFile{Path: path}.Login()
	}

	if _, found, err := (CodexAuthFile{Path: filepath.Join(dir, "missing.json")}).Login(); found || err != nil {
		t.Errorf("no auth.json: found %t, error %v; want neither", found, err)
	}
	if _, found, err := (CodexAuthFile{}).Login(); found || err != nil {
		t.Errorf("no home folder: found %t, error %v; want neither", found, err)
	}

	login, found, err := read(`{"OPENAI_API_KEY": null, "tokens": {"access_token": "` + fakeToken + `", "account_id": "` + fakeAccount + `", "refresh_token": "refresh-not-read"}}`)
	if err != nil || !found || login != (CodexLogin{AccessToken: fakeToken, AccountID: fakeAccount}) {
		t.Errorf("a ChatGPT login: found %t, error %v", found, err)
	}

	for name, content := range map[string]string{
		"an API key login":        `{"OPENAI_API_KEY": "key-not-read", "tokens": null}`,
		"a login without tokens":  `{"tokens": {"access_token": "", "account_id": "` + fakeAccount + `"}}`,
		"a file that is not JSON": `not json ` + fakeToken,
	} {
		_, found, err := read(content)
		var cloudErr *CloudError
		if found || !errors.As(err, &cloudErr) || cloudErr.Reason != CloudNoCredential {
			t.Errorf("%s: found %t, error %v; want no-credential", name, found, err)
			continue
		}
		if err.Error() != "the Codex sign-in on this machine is not a ChatGPT login. Sign in with `codex login` first." {
			t.Errorf("%s: text %q", name, err.Error())
		}
		if cause := fmt.Sprint(cloudErr.Cause); strings.Contains(cause, fakeToken) || strings.Contains(cause, "key-not-read") {
			t.Errorf("%s: the cause holds a secret: %q", name, cause)
		}
	}
}

func TestScan_ListsCodexCloudThreadsUnderCodexAndSaysWhyEachCloudIsMissing(t *testing.T) {
	scanner, _ := testScanner(t, noSQLite, fakeProcesses{})
	at := time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)
	scanner.Now = func() time.Time { return at }
	thread := Conversation{Tool: ToolCodex, App: AppCloud, ID: codexIdle, Title: "Fix the parser", UpdatedAt: "2026-11-07T11:30:00Z"}
	scanner.CodexCloud = &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) { return []Conversation{thread}, nil },
		func() ([]Conversation, error) {
			return nil, &CloudError{Tool: ToolCodex, Reason: CloudRefused, Cause: errors.New("HTTP 500")}
		},
	}}
	scanner.Cloud = &countingCloud{answers: []func() ([]Conversation, error){
		func() ([]Conversation, error) { return nil, nil },
		func() ([]Conversation, error) {
			return nil, &CloudError{Tool: ToolClaude, Reason: CloudUnreachable, Cause: errors.New("timeout")}
		},
	}}

	view := scanner.Scan(context.Background(), ScanOptions{})

	row := byID(t, view, codexIdle)
	if row.Tool != ToolCodex || row.App != AppCloud || row.State != StateActive || view.CloudNote != "" {
		t.Errorf("row %s, note %q", asJSON(t, row), view.CloudNote)
	}
	for _, source := range view.Tools {
		wantCount := map[Tool]int{ToolCodex: 1}[source.Tool]
		if source.Count != wantCount {
			t.Errorf("%s counts %d conversations, want %d", source.Tool, source.Count, wantCount)
		}
	}

	at = at.Add(freshSpan)
	view = scanner.Scan(context.Background(), ScanOptions{})
	want := "Claude Code cloud sessions are not listed: claude.ai did not answer in time. Codex cloud threads are not listed: the Codex cloud refused to list them."
	if len(view.Conversations) != 0 || view.CloudNote != want {
		t.Errorf("rows %s, note %q", asJSON(t, view.Conversations), view.CloudNote)
	}
}

func TestOpen_SendsACodexCloudThreadToTheCodexApp(t *testing.T) {
	row, err := codexCloudConversation(codexCloudRow{ID: codexIdle, Name: "Fix the parser", RecencyAt: 1794264000})
	if err != nil {
		t.Fatal(err)
	}
	launcher := &fakeLauncher{}
	reply := Open(context.Background(), row, fakeProcesses{}, launcher)
	want := OpenReply{Opened: true, Message: "Opened in the Codex app."}
	if reply != want || strings.Join(launcher.calls, ";") != "link codex://threads/"+codexIdle+"?hostId=durable" {
		t.Errorf("reply %+v after %v", reply, launcher.calls)
	}
	launcher = &fakeLauncher{err: errNotHere}
	reply = Open(context.Background(), row, fakeProcesses{}, launcher)
	want = OpenReply{Message: "Could not open the Codex app. This thread runs in the cloud: open codex://threads/" + codexIdle + "?hostId=durable"}
	if reply != want {
		t.Errorf("reply %+v", reply)
	}
}
