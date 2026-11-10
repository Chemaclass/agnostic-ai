package conversations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
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
// fields. A bucket not listed is not live.
var cloudBuckets = map[string]cloudBucket{
	"working":      {live: true, activity: ActivityWorking},
	"blocked":      {live: true, activity: ActivityWaiting},
	"review_ready": {live: false},
	"completed":    {live: false},
}

// CloudCredentials hands out the claude.ai sign-in token for one call.
type CloudCredentials interface {
	// AccessToken reads the token afresh, or returns a *CloudError.
	// The caller never keeps, prints or logs it.
	AccessToken(ctx context.Context) (string, error)
}

// KeychainCredentials reads the token Claude Code keeps in the macOS
// keychain. Claude Code renews it there; this reads it on each call
// and never touches the refresh token.
type KeychainCredentials struct {
	// GOOS is the running system; only darwin has a keychain to read.
	GOOS string
	// Read returns the keychain entry's JSON.
	Read func(ctx context.Context) ([]byte, error)
}

// NewKeychainCredentials reads the running user's keychain with the
// security command.
func NewKeychainCredentials() KeychainCredentials {
	return KeychainCredentials{GOOS: runtime.GOOS, Read: readClaudeKeychain}
}

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
	if k.GOOS != "darwin" {
		return "", &CloudError{Tool: ToolClaude, Reason: CloudUnsupported, Cause: fmt.Errorf("no Claude sign-in store known on %s", k.GOOS)}
	}
	raw, err := k.Read(ctx)
	if err != nil {
		return "", &CloudError{Tool: ToolClaude, Reason: CloudNoCredential, Cause: err}
	}
	var entry claudeKeychainEntry
	if err := json.Unmarshal(raw, &entry); err != nil {
		// The JSON error names a position and a type, never the text.
		return "", &CloudError{Tool: ToolClaude, Reason: CloudNoCredential, Cause: fmt.Errorf("keychain entry %q is not the JSON Claude Code writes: %w", claudeKeychainService, err)}
	}
	oauth := entry.ClaudeAiOauth
	if oauth == nil || oauth.AccessToken == "" {
		return "", &CloudError{Tool: ToolClaude, Reason: CloudNoCredential, Cause: fmt.Errorf("keychain entry %q holds no claude.ai sign-in", claudeKeychainService)}
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

type claudeCloudRow struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	StatusBucket string `json:"status_bucket"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
	LastEventAt  string `json:"last_event_at"`
}

// List asks for the sessions once, with no retry.
func (c *ClaudeCloud) List(ctx context.Context) ([]Conversation, error) {
	token, err := c.Credentials.AccessToken(ctx)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+claudeCloudSessionsPath, nil)
	if err != nil {
		return nil, &CloudError{Tool: ToolClaude, Reason: CloudUnreachable, Cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("anthropic-client-platform", runtime.GOOS)
	req.Header.Set("User-Agent", "agnostic-ai")
	resp, err := c.Client.Do(req)
	if err != nil {
		return nil, &CloudError{Tool: ToolClaude, Reason: CloudUnreachable, Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, refusal(resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCloudReplyBytes+1))
	if err != nil {
		return nil, &CloudError{Tool: ToolClaude, Reason: CloudUnreachable, Cause: err}
	}
	if len(body) > maxCloudReplyBytes {
		return nil, &CloudError{Tool: ToolClaude, Reason: CloudUnreadable, Cause: fmt.Errorf("the session list is larger than %d bytes", maxCloudReplyBytes)}
	}
	return cloudConversations(body)
}

// cloudRefusals maps the statuses that say something about the sign-in.
// Any other status is a refusal.
var cloudRefusals = map[int]CloudReason{
	http.StatusUnauthorized: CloudSignedOut,
}

// refusal turns an answer other than 200 into a CloudError that keeps
// the start of the provider's text as its cause.
func refusal(resp *http.Response) error {
	reason, known := cloudRefusals[resp.StatusCode]
	if !known {
		reason = CloudRefused
	}
	said, _ := io.ReadAll(io.LimitReader(resp.Body, maxCloudCauseBytes))
	return &CloudError{Tool: ToolClaude, Reason: reason, Cause: fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(said)))}
}

// cloudConversations reads the list answer. A row this version cannot
// read fails the whole answer rather than drop out of sight.
func cloudConversations(body []byte) ([]Conversation, error) {
	var list claudeCloudList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, &CloudError{Tool: ToolClaude, Reason: CloudUnreadable, Cause: fmt.Errorf("session list: %w", err)}
	}
	if list.Data == nil {
		return nil, &CloudError{Tool: ToolClaude, Reason: CloudUnreadable, Cause: errors.New("session list has no data field")}
	}
	conversations := make([]Conversation, 0, len(*list.Data))
	for i, row := range *list.Data {
		c, err := cloudConversation(row)
		if err != nil {
			return nil, &CloudError{Tool: ToolClaude, Reason: CloudUnreadable, Cause: fmt.Errorf("session list row %d: %w", i, err)}
		}
		conversations = append(conversations, c)
	}
	return conversations, nil
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
