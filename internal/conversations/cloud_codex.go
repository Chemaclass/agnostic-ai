package conversations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// This file is the only code that knows the Codex cloud's thread API.
// The API is the one the Codex app reads its cloud threads from. It is
// undocumented: what it reads was measured on one account on
// 2026-11-10 and can change with any Codex release.

// codexCloudBase is where the thread list is asked for. The sign-in
// token is sent nowhere else.
const codexCloudBase = "https://codex-cloud-backend.chatgpt.com"

// codexCloudThreadsPath lists the person's cloud threads. It answers
// at most 100; the answer's nextCursor, which asks for more, is not
// followed yet.
const codexCloudThreadsPath = "/v1/threads?limit=100"

// codexThreadIDPattern is a cloud thread id, a UUID.
var codexThreadIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// codexStatuses maps a thread's status type to the live field, as the
// Codex app-server protocol names them. A type not listed is not live.
var codexStatuses = map[string]bool{
	"active":      true,
	"idle":        false,
	"notLoaded":   false,
	"systemError": false,
}

// codexWaitingFlags are the active flags that mean the thread waits
// for the person.
var codexWaitingFlags = map[string]bool{
	"waitingOnApproval":  true,
	"waitingOnUserInput": true,
}

// CodexLogin is the ChatGPT login the Codex command keeps.
type CodexLogin struct {
	AccessToken string
	AccountID   string
}

// CodexCredentials hands out the Codex login for one call.
type CodexCredentials interface {
	// Login reads the login afresh. found is false when the person
	// has never signed in to Codex on this machine, which is no error.
	// The caller never keeps, prints or logs the token.
	Login() (login CodexLogin, found bool, err error)
}

// CodexAuthFile reads the login the Codex command keeps in auth.json.
// Codex renews the token there; this reads it on each call and never
// touches the refresh token.
type CodexAuthFile struct {
	// Path is the auth.json of the Codex home folder.
	Path string
}

// NewCodexAuthFile reads the running user's Codex login: auth.json in
// $CODEX_HOME, or in ~/.codex when that is not set.
func NewCodexAuthFile() CodexAuthFile {
	if home := os.Getenv("CODEX_HOME"); home != "" {
		return CodexAuthFile{Path: filepath.Join(home, "auth.json")}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		// With no home folder there is no file to find; Login says so.
		return CodexAuthFile{}
	}
	return CodexAuthFile{Path: filepath.Join(home, ".codex", "auth.json")}
}

// maxCodexAuthBytes bounds auth.json: a few tokens.
const maxCodexAuthBytes = 1 << 20

// codexAuthEntry holds the fields of auth.json this reads. The refresh
// token, the id token and an API key are left out on purpose.
type codexAuthEntry struct {
	Tokens *struct {
		AccessToken string `json:"access_token"`
		AccountID   string `json:"account_id"`
	} `json:"tokens"`
}

func (f CodexAuthFile) Login() (CodexLogin, bool, error) {
	if f.Path == "" {
		return CodexLogin{}, false, nil
	}
	file, err := os.Open(f.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return CodexLogin{}, false, nil
	}
	if err != nil {
		return CodexLogin{}, false, &CloudError{Tool: ToolCodex, Reason: CloudNoCredential, Cause: err}
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxCodexAuthBytes))
	if err != nil {
		return CodexLogin{}, false, &CloudError{Tool: ToolCodex, Reason: CloudNoCredential, Cause: err}
	}
	var entry codexAuthEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		// The JSON error names a position and a type, never the text.
		return CodexLogin{}, false, &CloudError{Tool: ToolCodex, Reason: CloudNoCredential, Cause: fmt.Errorf("%s is not the JSON Codex writes: %w", f.Path, err)}
	}
	// Signed in with an API key: the cloud threads belong to a ChatGPT login.
	if entry.Tokens == nil || entry.Tokens.AccessToken == "" || entry.Tokens.AccountID == "" {
		return CodexLogin{}, false, &CloudError{Tool: ToolCodex, Reason: CloudNoCredential, Cause: fmt.Errorf("%s holds no ChatGPT login", f.Path)}
	}
	return CodexLogin{AccessToken: entry.Tokens.AccessToken, AccountID: entry.Tokens.AccountID}, true, nil
}

// CodexCloud lists the Codex cloud's threads from its API.
type CodexCloud struct {
	// BaseURL is the API's address; tests point it at a fake.
	BaseURL     string
	Client      *http.Client
	Credentials CodexCredentials
}

// NewCodexCloud lists the signed-in person's cloud threads with the
// login the Codex command keeps.
func NewCodexCloud() *CodexCloud {
	return &CodexCloud{BaseURL: codexCloudBase, Client: newCloudClient(), Credentials: NewCodexAuthFile()}
}

// codexCloudList is the list answer, with the fields the rows need.
type codexCloudList struct {
	Data *[]codexCloudRow `json:"data"`
}

type codexCloudRow struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status struct {
		Type        string   `json:"type"`
		ActiveFlags []string `json:"activeFlags"`
	} `json:"status"`
	// The times are seconds since 1970.
	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt"`
	RecencyAt int64 `json:"recencyAt"`
}

// List asks for the threads once, with no retry. A person who never
// signed in to Codex has no cloud threads: nothing is asked and
// nothing is said.
func (c *CodexCloud) List(ctx context.Context) ([]Conversation, error) {
	login, found, err := c.Credentials.Login()
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+codexCloudThreadsPath, nil)
	if err != nil {
		return nil, &CloudError{Tool: ToolCodex, Reason: CloudUnreachable, Cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+login.AccessToken)
	req.Header.Set("ChatGPT-Account-Id", login.AccountID)
	req.Header.Set("X-OpenAI-Product-Sku", "codex")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "agnostic-ai")
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, &CloudError{Tool: ToolCodex, Reason: CloudUnreachable, Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, codexRefusal(resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCloudReplyBytes+1))
	if err != nil {
		return nil, &CloudError{Tool: ToolCodex, Reason: CloudUnreachable, Cause: err}
	}
	if len(body) > maxCloudReplyBytes {
		return nil, &CloudError{Tool: ToolCodex, Reason: CloudUnreadable, Cause: fmt.Errorf("the thread list is larger than %d bytes", maxCloudReplyBytes)}
	}
	return codexCloudConversations(body)
}

// codexRefusal turns an answer other than 200 into a CloudError that
// keeps the start of the provider's text as its cause. Only 401 says
// something about the sign-in.
func codexRefusal(resp *http.Response) error {
	reason, known := cloudRefusals[resp.StatusCode]
	if !known {
		reason = CloudRefused
	}
	said, _ := io.ReadAll(io.LimitReader(resp.Body, maxCloudCauseBytes))
	return &CloudError{Tool: ToolCodex, Reason: reason, Cause: fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(said)))}
}

// codexCloudConversations reads the list answer. A row this version
// cannot read fails the whole answer rather than drop out of sight.
func codexCloudConversations(body []byte) ([]Conversation, error) {
	var list codexCloudList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, &CloudError{Tool: ToolCodex, Reason: CloudUnreadable, Cause: fmt.Errorf("thread list: %w", err)}
	}
	if list.Data == nil {
		return nil, &CloudError{Tool: ToolCodex, Reason: CloudUnreadable, Cause: errors.New("thread list has no data field")}
	}
	conversations := make([]Conversation, 0, len(*list.Data))
	for i, row := range *list.Data {
		c, err := codexCloudConversation(row)
		if err != nil {
			return nil, &CloudError{Tool: ToolCodex, Reason: CloudUnreadable, Cause: fmt.Errorf("thread list row %d: %w", i, err)}
		}
		conversations = append(conversations, c)
	}
	return conversations, nil
}

// codexCloudConversation turns one row into a list row. A row's folder
// is a path inside the cloud machine, not one of the person's, so
// folder and branch stay empty.
func codexCloudConversation(row codexCloudRow) (Conversation, error) {
	if !codexThreadIDPattern.MatchString(row.ID) {
		return Conversation{}, fmt.Errorf("id %q is not a cloud thread id", row.ID)
	}
	written := row.RecencyAt
	if written == 0 {
		written = row.UpdatedAt
	}
	if written <= 0 {
		return Conversation{}, errors.New("neither recencyAt nor updatedAt is set")
	}
	started := ""
	if row.CreatedAt > 0 {
		started = codexCloudTime(row.CreatedAt)
	}
	live := codexStatuses[row.Status.Type]
	activity := Activity("")
	if live {
		activity = ActivityWorking
		for _, flag := range row.Status.ActiveFlags {
			if codexWaitingFlags[flag] {
				activity = ActivityWaiting
			}
		}
	}
	return Conversation{
		Tool:      ToolCodex,
		App:       AppCloud,
		ID:        row.ID,
		Title:     row.Name,
		StartedAt: started,
		UpdatedAt: codexCloudTime(written),
		Live:      live,
		Activity:  activity,
		// hostId=durable is how the Codex app names a thread that lives in its cloud.
		Open: Route{Kind: RouteLink, Label: "Open in Codex app", Link: "codex://threads/" + row.ID + "?hostId=durable"},
	}, nil
}

// codexCloudTime writes seconds since 1970 as an RFC 3339 time.
func codexCloudTime(seconds int64) string {
	return time.Unix(seconds, 0).UTC().Format(time.RFC3339)
}
