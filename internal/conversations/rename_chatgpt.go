package conversations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// This file is the only code that renames a ChatGPT chat. It asks
// chatgpt.com to change the chat's title, as chatgpt.com's own page
// does, signed in with the ChatGPT login the Codex command keeps. The
// route is undocumented and can change at any time. The ChatGPT app
// shows the new title once it syncs.

// chatGPTBase is where a title change is sent. The sign-in token is
// sent nowhere else.
const chatGPTBase = "https://chatgpt.com"

// chatGPTConversationPath is one chat's route; the id follows it.
const chatGPTConversationPath = "/backend-api/conversation/"

// chatGPTTitles renames ChatGPT chats on chatgpt.com.
type chatGPTTitles struct {
	// baseURL is chatgpt.com; tests point it at a fake.
	baseURL     string
	client      *http.Client
	credentials CodexCredentials
}

func newChatGPTTitles() *chatGPTTitles {
	return &chatGPTTitles{baseURL: chatGPTBase, client: newCloudClient(), credentials: NewCodexAuthFile()}
}

type chatGPTTitleChange struct {
	Title string `json:"title"`
}

// chatGPTTitleChanged is chatgpt.com's answer to a title change, which
// says {"success":true} once the chat has the title.
type chatGPTTitleChanged struct {
	Success *bool `json:"success"`
}

// chatGPTRenameRefusals maps an answer to a reason; any other answer
// that is not 2xx is a refusal.
var chatGPTRenameRefusals = map[int]RenameReason{
	http.StatusUnauthorized: RenameSignedOut,
}

// SetTitle asks once, with no retry.
func (c *chatGPTTitles) SetTitle(ctx context.Context, chatID, title string) error {
	if !chatGPTIDPattern.MatchString(chatID) {
		return &RenameError{Tool: ToolChatGPT, Reason: RenameUnexpected, Cause: fmt.Errorf("%q is not a ChatGPT chat id", chatID)}
	}
	login, found, err := c.credentials.Login()
	if err != nil {
		// The login error names the file, never the token.
		return &RenameError{Tool: ToolChatGPT, Reason: RenameSignedOut, Cause: err}
	}
	if !found {
		return &RenameError{Tool: ToolChatGPT, Reason: RenameSignedOut, Cause: errors.New("no Codex login with ChatGPT on this machine")}
	}
	body, err := json.Marshal(chatGPTTitleChange{Title: title})
	if err != nil {
		return &RenameError{Tool: ToolChatGPT, Reason: RenameUnexpected, Cause: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+chatGPTConversationPath+chatID, bytes.NewReader(body))
	if err != nil {
		return &RenameError{Tool: ToolChatGPT, Reason: RenameUnreachable, Cause: err}
	}
	req.Header.Set("Authorization", "Bearer "+login.AccessToken)
	req.Header.Set("Chatgpt-Account-Id", login.AccountID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "agnostic-ai")
	resp, err := c.client.Do(req)
	if err != nil {
		return &RenameError{Tool: ToolChatGPT, Reason: RenameUnreachable, Cause: err}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		reason, known := chatGPTRenameRefusals[resp.StatusCode]
		if !known {
			reason = RenameRefused
		}
		return &RenameError{Tool: ToolChatGPT, Reason: reason, Cause: refusalCause(resp.StatusCode, resp.Body, login.AccessToken)}
	}
	var answer chatGPTTitleChanged
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxRenameAnswerBytes)).Decode(&answer); err != nil {
		return &RenameError{Tool: ToolChatGPT, Reason: RenameUnexpected, Cause: fmt.Errorf("rename answer: %w", err)}
	}
	switch {
	case answer.Success == nil:
		return &RenameError{Tool: ToolChatGPT, Reason: RenameUnexpected, Cause: errors.New("the rename answer does not say whether the title changed")}
	case !*answer.Success:
		return &RenameError{Tool: ToolChatGPT, Reason: RenameRefused, Cause: errors.New("the rename answer says the title did not change")}
	}
	return nil
}
