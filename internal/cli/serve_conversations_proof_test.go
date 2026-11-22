package cli

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/conversations"
)

// proofCase is a header a request proves itself with, or none.
type proofCase struct{ header, value string }

// withProof requires the companion secret or the session key on h, and
// returns the proofs it takes and the ones it refuses.
func withProof(h *conversationsHandler) (valid, refused map[string]proofCase) {
	session := &webSession{key: strings.Repeat("k", 64)}
	settings := &settingsHandler{secret: testCompanionSecret, session: session}
	h.proven, h.canProve = settings.proven, settings.canProve
	return map[string]proofCase{
		"the companion secret": {companionSecretHeader, string(testCompanionSecret)},
		"the session key":      {sessionHeader, session.key},
	}, map[string]proofCase{
		"no proof":            {},
		"a wrong secret":      {companionSecretHeader, strings.Repeat("0", len(testCompanionSecret))},
		"a wrong session key": {sessionHeader, strings.Repeat("0", len(session.key))},
	}
}

func postProven(t *testing.T, h http.Handler, path, body string, proof proofCase) *httptest.ResponseRecorder {
	t.Helper()
	return postEdit(t, h, path, body, func(r *http.Request) {
		if proof.header != "" {
			r.Header.Set(proof.header, proof.value)
		}
	})
}

// withoutProofSource leaves h with no secret or session key at all.
func withoutProofSource(h *conversationsHandler) {
	h.proven, h.canProve = (&settingsHandler{}).proven, (&settingsHandler{}).canProve
}

func TestConversationsHandler_DeletesOnlyWithProof(t *testing.T) {
	body := `{"tool":"claude","id":"` + inProject + `"}`
	h, _, _ := conversationsFixture(t, false)
	transcript := archiveForDelete(t, h)
	valid, refused := withProof(h)
	for name, proof := range refused {
		if rec := postProven(t, h, conversationDeletePath, body, proof); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Nothing was deleted") {
			t.Errorf("delete with %s = %d %s, want 403", name, rec.Code, rec.Body)
		}
	}
	if _, err := os.Stat(transcript); err != nil {
		t.Fatalf("a delete without proof removed the transcript: %v", err)
	}
	withoutProofSource(h)
	if rec := postProven(t, h, conversationDeletePath, body, valid["the companion secret"]); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "This server cannot delete conversations") {
		t.Errorf("delete on a server with no proof source = %d %s", rec.Code, rec.Body)
	}

	for name := range valid {
		t.Run(name, func(t *testing.T) {
			h, _, _ := conversationsFixture(t, false)
			transcript := archiveForDelete(t, h)
			valid, _ := withProof(h)
			if rec := postProven(t, h, conversationDeletePath, body, valid[name]); rec.Code != http.StatusOK {
				t.Errorf("delete with %s = %d %s", name, rec.Code, rec.Body)
			}
			if _, err := os.Stat(transcript); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("the transcript is still there: %v", err)
			}
		})
	}
}

// askedStopProcesses counts the process reads a stop makes.
type askedStopProcesses struct {
	stillProcesses
	asked *atomic.Int32
}

func (p askedStopProcesses) AliveAmong([]conversations.ProcessRef) (map[int]bool, error) {
	p.asked.Add(1)
	return nil, errors.New("failed to read processes")
}

func TestConversationsHandler_StopsASessionOnlyWithProof(t *testing.T) {
	body := `{"tool":"claude","id":"` + inProject + `","pid":4242,"procStart":"Sat Nov  7 11:00:00 2026"}`
	h, _, _ := conversationsFixture(t, true)
	var asked atomic.Int32
	h.scanner.Processes = askedStopProcesses{asked: &asked}
	valid, refused := withProof(h)
	for name, proof := range refused {
		if rec := postProven(t, h, conversationStopSessionPath, body, proof); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Nothing was stopped") {
			t.Errorf("stop with %s = %d %s, want 403", name, rec.Code, rec.Body)
		}
	}
	if got := asked.Load(); got != 0 {
		t.Fatalf("a stop without proof read the processes %d times", got)
	}
	for name, proof := range valid {
		if rec := postProven(t, h, conversationStopSessionPath, body, proof); rec.Code == http.StatusForbidden {
			t.Errorf("stop with %s = %d %s, want it past the proof check", name, rec.Code, rec.Body)
		}
	}
	withoutProofSource(h)
	if rec := postProven(t, h, conversationStopSessionPath, body, valid["the companion secret"]); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "This server cannot stop sessions") {
		t.Errorf("stop on a server with no proof source = %d %s", rec.Code, rec.Body)
	}
}

func TestConversationsHandler_RenamesAnOnlineConversationOnlyWithProof(t *testing.T) {
	const body = `{"tool":"claude","id":"cse_01AbCd","title":"Parser fix"}`
	h, _, _ := conversationsFixture(t, true)
	h.scanner.MarksPath = filepath.Join(t.TempDir(), "conversation-marks.json")
	h.scanner.Cloud = oneCloudSession{conversations.Conversation{Tool: conversations.ToolClaude, App: conversations.AppCloud, ID: "cse_01AbCd", Title: "Fix the parser", UpdatedAt: "2026-11-07T11:30:00Z", Open: conversations.Route{Kind: conversations.RouteLink, Label: "Open on claude.ai", Link: "https://claude.ai/code/session_01AbCd"}}}
	var renamed atomic.Int32
	h.renameInTool = func(context.Context, conversations.Conversation, string) error {
		renamed.Add(1)
		return nil
	}
	valid, refused := withProof(h)
	for name, proof := range refused {
		if rec := postProven(t, h, conversationRenamePath, body, proof); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "The name did not change") {
			t.Errorf("rename with %s = %d %s, want 403", name, rec.Code, rec.Body)
		}
	}
	if got := renamed.Load(); got != 0 {
		t.Fatalf("a rename without proof reached the cloud %d times", got)
	}
	for name, proof := range valid {
		if rec := postProven(t, h, conversationRenamePath, body, proof); rec.Code != http.StatusNoContent {
			t.Errorf("rename with %s = %d %s", name, rec.Code, rec.Body)
		}
	}
	if got := renamed.Load(); got != 2 {
		t.Errorf("renamed %d times, want only the two proven renames", got)
	}
	withoutProofSource(h)
	if rec := postProven(t, h, conversationRenamePath, body, valid["the companion secret"]); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "This server cannot rename online conversations") {
		t.Errorf("rename on a server with no proof source = %d %s", rec.Code, rec.Body)
	}

	local := `{"tool":"claude","id":"` + inProject + `","title":"Parser fix"}`
	if rec := postProven(t, h, conversationRenamePath, local, proofCase{}); rec.Code != http.StatusNoContent {
		t.Errorf("a local rename without proof = %d %s, want 204", rec.Code, rec.Body)
	}
}

func TestRenameReachesCloud_NamesTheOnlineKinds(t *testing.T) {
	for _, tc := range []struct {
		c     conversations.Conversation
		cloud bool
	}{
		{conversations.Conversation{Tool: conversations.ToolClaude, App: conversations.AppCloud}, true},
		{conversations.Conversation{Tool: conversations.ToolChatGPT, App: conversations.AppChat}, true},
		{conversations.Conversation{Tool: conversations.ToolClaude, App: conversations.AppCLI}, false},
		{conversations.Conversation{Tool: conversations.ToolCodex, App: conversations.AppCLI}, false},
		{conversations.Conversation{Tool: conversations.ToolCursor, App: conversations.AppIDE}, false},
	} {
		if got := conversations.RenameReachesCloud(tc.c); got != tc.cloud {
			t.Errorf("%s %s: cloud = %v, want %v", tc.c.Tool, tc.c.App, got, tc.cloud)
		}
	}
}

// countedProcesses counts the process reads a request makes.
type countedProcesses struct {
	stillProcesses
	asked *atomic.Int32
}

func (p countedProcesses) Table() ([]conversations.ProcessInfo, error) {
	p.asked.Add(1)
	return nil, errors.New("failed to read processes")
}

func (p countedProcesses) AliveAmong([]conversations.ProcessRef) (map[int]bool, error) {
	p.asked.Add(1)
	return nil, errors.New("failed to read processes")
}

func TestConversationsHandler_StopsAShellOnlyWithProof(t *testing.T) {
	body := `{"tool":"claude","id":"` + inProject + `","pid":4242,"procStart":"Sat Nov  7 11:00:00 2026"}`
	h, _, _ := conversationsFixture(t, true)
	var asked atomic.Int32
	h.scanner.Processes = countedProcesses{asked: &asked}
	valid, refused := withProof(h)
	for name, proof := range refused {
		if rec := postProven(t, h, conversationStopShellPath, body, proof); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Nothing was stopped") {
			t.Errorf("stop shell with %s = %d %s, want 403", name, rec.Code, rec.Body)
		}
	}
	if got := asked.Load(); got != 0 {
		t.Fatalf("a shell stop without proof read the processes %d times", got)
	}
	for name, proof := range valid {
		if rec := postProven(t, h, conversationStopShellPath, body, proof); rec.Code == http.StatusForbidden {
			t.Errorf("stop shell with %s = %d %s, want it past the proof check", name, rec.Code, rec.Body)
		}
	}
	withoutProofSource(h)
	if rec := postProven(t, h, conversationStopShellPath, body, valid["the companion secret"]); rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "This server cannot stop shells") {
		t.Errorf("stop shell on a server with no proof source = %d %s", rec.Code, rec.Body)
	}
}
