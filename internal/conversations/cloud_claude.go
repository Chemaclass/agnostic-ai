package conversations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
)

// This file is the only code that knows Claude Code's cloud session
// API. The API is undocumented: what it reads was measured on one
// account on 2026-11-09 and can change with any Claude Code release.

// claudeCloudBase is where the session list is asked for. The sign-in
// token is sent nowhere else.
const claudeCloudBase = "https://api.anthropic.com"

// claudeCloudSessionsPath lists the person's sessions. It answers at
// most 100; how to ask for more is not known yet.
const claudeCloudSessionsPath = "/v1/code/sessions?limit=100"

// claudeCloudSessionPath is one session's route; the id follows it. A
// PUT with {"title"} renames the session, as Claude Code's own
// updateSessionTitle does; seen in Claude Code 2.1.325 and checked with
// a rename to the same title on 2026-11-16.
const claudeCloudSessionPath = "/v1/code/sessions/"

// claudeCloudLink is where cloud sessions are listed in the browser.
const claudeCloudLink = "https://claude.ai/code"

// cloudSessionIDPrefix starts every cloud session id.
const cloudSessionIDPrefix = "cse_"

// cloudSessionLink is a session's own page: claude.ai names it by the
// id with session_ in place of cse_. Seen on one account's sessions on
// 2026-11-10; like the rest of this file it can change.
func cloudSessionLink(id string) string {
	return claudeCloudLink + "/session_" + strings.TrimPrefix(id, cloudSessionIDPrefix)
}

// cloudSessionLinkPattern is the only shape of web page the server opens.
var cloudSessionLinkPattern = regexp.MustCompile(`^https://claude\.ai/code/session_[A-Za-z0-9_-]{1,128}$`)

// cloudTimeout bounds one list call. A slow answer is a failed one: the
// list shows without cloud sessions rather than wait.
const cloudTimeout = 5 * time.Second

// maxCloudReplyBytes bounds the list answer. 100 rows of a few
// kilobytes each sit far below it.
const maxCloudReplyBytes = 8 << 20

// maxCloudCauseBytes is how much of a refusal's body is kept for logs.
const maxCloudCauseBytes = 300

// claudeCloudScope is the sign-in scope the session routes accept.
const claudeCloudScope = "user:sessions:claude_code"

// cloudSessionIDPattern is a cloud session id, such as cse_0a1B2c3D.
var cloudSessionIDPattern = regexp.MustCompile(`^cse_[A-Za-z0-9_-]{1,128}$`)

// cloudBucket is what a status_bucket says about a session.
type cloudBucket struct {
	live     bool
	activity Activity
}

// cloudBuckets maps each status_bucket seen to the live and activity
// fields. A bucket not listed is not live. The bucket alone is not
// believed for working: see claudeCloudWorkerRunning.
var cloudBuckets = map[string]cloudBucket{
	"working":      {live: true, activity: ActivityWorking},
	"blocked":      {live: true, activity: ActivityWaiting},
	"review_ready": {live: false},
	"completed":    {live: false},
}

// claudeCloudWorkerRunning is the worker_status of a session whose
// agent runs now. A session that failed keeps the working bucket for
// weeks with another worker status, so working needs both.
const claudeCloudWorkerRunning = "running"

// CloudCredentials hands out the claude.ai sign-in token for one call.
type CloudCredentials interface {
	// AccessToken reads the token afresh, or returns a *CloudError.
	// The caller never keeps, prints or logs it.
	AccessToken(ctx context.Context) (string, error)
}

// KeychainCredentials reads the token Claude Code keeps in the macOS
// keychain, or in its credentials file on Linux. Claude Code renews it
// there; this reads it on each call and never touches the refresh token.
type KeychainCredentials struct {
	// GOOS is the running system; only darwin, linux and windows have a store to read.
	GOOS string
	// Read returns the stored sign-in's JSON.
	Read func(ctx context.Context) ([]byte, error)
}

// NewKeychainCredentials reads the running user's sign-in store.
func NewKeychainCredentials() KeychainCredentials {
	return newKeychainCredentials(runtime.GOOS)
}

func newKeychainCredentials(goos string) KeychainCredentials {
	if goos == "linux" || goos == "windows" {
		path := filepath.Join(DefaultStores().ClaudeDir, claudeCredentialsFile)
		return KeychainCredentials{GOOS: goos, Read: func(context.Context) ([]byte, error) { return os.ReadFile(path) }}
	}
	return KeychainCredentials{GOOS: goos, Read: readClaudeKeychain}
}

// claudeCredentialsFile is where Claude Code keeps its sign-in, in its
// config folder, on systems without a keychain it uses.
const claudeCredentialsFile = ".credentials.json"

// claudeKeychainService is the keychain entry Claude Code keeps its sign-in in.
const claudeKeychainService = "Claude Code-credentials"

func readClaudeKeychain(ctx context.Context) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, cloudTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "security", "find-generic-password", "-s", claudeKeychainService, "-w")
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		// Only stderr goes into the error: stdout would hold the secret.
		return nil, fmt.Errorf("security find-generic-password: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// claudeKeychainEntry holds the fields of the keychain entry this
// reads. The refresh token is left out on purpose.
type claudeKeychainEntry struct {
	ClaudeAiOauth *struct {
		AccessToken string   `json:"accessToken"`
		Scopes      []string `json:"scopes"`
	} `json:"claudeAiOauth"`
}

func (k KeychainCredentials) AccessToken(ctx context.Context) (string, error) {
	store := fmt.Sprintf("keychain entry %q", claudeKeychainService)
	switch k.GOOS {
	case "darwin":
	case "linux", "windows":
		store = "Claude Code's " + claudeCredentialsFile
	default:
		return "", &CloudError{Tool: ToolClaude, Reason: CloudUnsupported, Cause: fmt.Errorf("no Claude sign-in store known on %s", k.GOOS)}
	}
	raw, err := k.Read(ctx)
	if err != nil {
		return "", &CloudError{Tool: ToolClaude, Reason: CloudNoCredential, Cause: err}
	}
	var entry claudeKeychainEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		// The JSON error names a position and a type, never the text.
		return "", &CloudError{Tool: ToolClaude, Reason: CloudNoCredential, Cause: fmt.Errorf("%s is not the JSON Claude Code writes: %w", store, err)}
	}
	oauth := entry.ClaudeAiOauth
	if oauth == nil || oauth.AccessToken == "" {
		return "", &CloudError{Tool: ToolClaude, Reason: CloudNoCredential, Cause: fmt.Errorf("%s holds no claude.ai sign-in", store)}
	}
	if !slices.Contains(oauth.Scopes, claudeCloudScope) {
		return "", &CloudError{Tool: ToolClaude, Reason: CloudNoCredential, Cause: fmt.Errorf("the claude.ai sign-in lacks the %s scope", claudeCloudScope)}
	}
	return oauth.AccessToken, nil
}

// ClaudeCloud lists Claude Code's cloud sessions from the API.
type ClaudeCloud struct {
	// BaseURL is the API's address; tests point it at a fake.
	BaseURL     string
	Client      *http.Client
	Credentials CloudCredentials
}

// NewClaudeCloud lists the signed-in person's sessions from
// api.anthropic.com with the keychain's token.
func NewClaudeCloud() *ClaudeCloud {
	return &ClaudeCloud{BaseURL: claudeCloudBase, Client: newCloudClient(), Credentials: NewKeychainCredentials()}
}

// newCloudClient gives up after cloudTimeout and follows no redirect,
// so the token goes to BaseURL and nowhere else.
func newCloudClient() *http.Client {
	return &http.Client{
		Timeout: cloudTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// claudeCloudList is the list answer, with the fields the rows need.
type claudeCloudList struct {
	Data *[]claudeCloudRow `json:"data"`
}

func (l claudeCloudList) rows() *[]claudeCloudRow { return l.Data }

type claudeCloudRow struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	StatusBucket string `json:"status_bucket"`
	WorkerStatus string `json:"worker_status"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	LastEventAt  string `json:"last_event_at"`
}

// claudeCloudTitleChange is the body of a rename.
type claudeCloudTitleChange struct {
	Title string `json:"title"`
}

// claudeCloudRenamed is the rename answer, with the field it checks.
type claudeCloudRenamed struct {
	Session *struct {
		Title string `json:"title"`
	} `json:"session"`
}

// claudeCloudRenameRefusals maps an answer to a reason; any other
// answer that is not 200 is a refusal.
var claudeCloudRenameRefusals = map[int]RenameReason{
	http.StatusUnauthorized: RenameSignedOut,
}

// SetTitle renames one cloud session, asking once with no retry. It
// returns nil only when the answer holds the new title.
func (c *ClaudeCloud) SetTitle(ctx context.Context, sessionID, title string) error {
	if !cloudSessionIDPattern.MatchString(sessionID) {
		return &RenameError{Tool: ToolClaude, Reason: RenameUnexpected, Cause: fmt.Errorf("%q is not a cloud session id", sessionID)}
	}
	token, err := c.Credentials.AccessToken(ctx)
	if err != nil {
		// The credentials error names the store, never the token.
		return &RenameError{Tool: ToolClaude, Reason: RenameSignedOut, Cause: err}
	}
	body, err := json.Marshal(claudeCloudTitleChange{Title: title})
	if err != nil {
		return &RenameError{Tool: ToolClaude, Reason: RenameUnexpected, Cause: err}
	}
	req, err := c.newRequest(ctx, http.MethodPut, claudeCloudSessionPath+sessionID, token, bytes.NewReader(body))
	if err != nil {
		return &RenameError{Tool: ToolClaude, Reason: RenameUnreachable, Cause: err}
	}
	resp, err := c.Client.Do(req)
	if err != nil {
		return &RenameError{Tool: ToolClaude, Reason: RenameUnreachable, Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		reason, known := claudeCloudRenameRefusals[resp.StatusCode]
		if !known {
			reason = RenameRefused
		}
		return &RenameError{Tool: ToolClaude, Reason: reason, Cause: refusalCause(resp.StatusCode, resp.Body, token)}
	}
	var renamed claudeCloudRenamed
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxCloudReplyBytes)).Decode(&renamed); err != nil {
		return &RenameError{Tool: ToolClaude, Reason: RenameUnexpected, Cause: fmt.Errorf("rename answer: %w", err)}
	}
	if renamed.Session == nil || renamed.Session.Title != title {
		return &RenameError{Tool: ToolClaude, Reason: RenameUnexpected, Cause: errors.New("the rename answer does not hold the new title")}
	}
	return nil
}

// List asks for the sessions once, with no retry.
func (c *ClaudeCloud) List(ctx context.Context) ([]Conversation, error) {
	token, err := c.Credentials.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	req, err := c.newRequest(ctx, http.MethodGet, claudeCloudSessionsPath, token, nil)
	if err != nil {
		return nil, &CloudError{Tool: ToolClaude, Reason: CloudUnreachable, Cause: err}
	}
	body, err := cloudGet(c.Client, req, ToolClaude, "session")
	if err != nil {
		return nil, err
	}
	return decodeCloudList[claudeCloudList](ToolClaude, "session", body, cloudConversation)
}

// newRequest builds a request to the sessions API with the headers it expects.
func (c *ClaudeCloud) newRequest(ctx context.Context, method, path, token string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Anthropic-Version", "2023-06-01")
	req.Header.Set("Anthropic-Client-Platform", runtime.GOOS)
	req.Header.Set("User-Agent", "agnostic-ai")
	return req, nil
}

// cloudConversation turns one row into a list row. The row names no
// folder or branch this can rely on, so both stay empty.
func cloudConversation(row claudeCloudRow) (Conversation, error) {
	if !cloudSessionIDPattern.MatchString(row.ID) {
		return Conversation{}, fmt.Errorf("id %q is not a cloud session id", row.ID)
	}
	written := row.LastEventAt
	if written == "" {
		written = row.UpdatedAt
	}
	if written == "" {
		return Conversation{}, errors.New("neither last_event_at nor updated_at is set")
	}
	updated, err := cloudTime(written)
	if err != nil {
		return Conversation{}, err
	}
	started := ""
	if row.CreatedAt != "" {
		started, err = cloudTime(row.CreatedAt)
		if err != nil {
			return Conversation{}, err
		}
	}
	bucket := cloudBuckets[row.StatusBucket]
	if bucket.activity == ActivityWorking && row.WorkerStatus != claudeCloudWorkerRunning {
		bucket = cloudBucket{}
	}
	return Conversation{
		Tool:      ToolClaude,
		App:       AppCloud,
		ID:        row.ID,
		Title:     row.Title,
		StartedAt: started,
		UpdatedAt: updated,
		Live:      bucket.live,
		Activity:  bucket.activity,
		Open:      Route{Kind: RouteLink, Label: "Open on claude.ai", Link: cloudSessionLink(row.ID)},
	}, nil
}

// cloudTime writes an RFC 3339 time in whole seconds, as every row's times are.
func cloudTime(value string) (string, error) {
	at, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", fmt.Errorf("time %q: %w", value, err)
	}
	return at.UTC().Format(time.RFC3339), nil
}
