package openhands

import (
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func captureNotes(t *testing.T) *strings.Builder {
	t.Helper()
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	return buf
}

// OpenHands sends an API key as `Authorization: Bearer <key>`, and
// mcp.json carries that header in `headers`.
func TestUserMCPServers_APIKeyBecomesBearerHeader(t *testing.T) {
	captureNotes(t)
	servers := New().UserMCPServers([]spec.Entry{
		{Kind: spec.KindMCP, Name: "search", Meta: map[string]any{"type": "http", "url": "https://search.test/mcp", "api_key": "k1"}},
		{Kind: spec.KindMCP, Name: "docs", Meta: map[string]any{"type": "sse", "url": "https://docs.test/sse", "api_key": "k2", "headers": map[string]any{"X-Team": "a"}}},
	})
	want := map[string]any{
		"search": map[string]any{"url": "https://search.test/mcp", "transport": "http", "headers": map[string]string{"Authorization": "Bearer k1"}},
		"docs":   map[string]any{"url": "https://docs.test/sse", "transport": "sse", "headers": map[string]string{"X-Team": "a", "Authorization": "Bearer k2"}},
	}
	if !reflect.DeepEqual(servers, want) {
		t.Errorf("servers = %#v\nwant %#v", servers, want)
	}
}

// A written Authorization header is the author's choice, so it wins and
// the api_key it replaces is named.
func TestUserMCPServers_AuthorizationHeaderWinsOverAPIKey(t *testing.T) {
	buf := captureNotes(t)
	servers := New().UserMCPServers([]spec.Entry{
		{Kind: spec.KindMCP, Name: "search", Meta: map[string]any{"type": "http", "url": "https://search.test/mcp", "api_key": "k1", "headers": map[string]any{"authorization": "Token t"}}},
	})
	headers := servers["search"].(map[string]any)["headers"]
	if !reflect.DeepEqual(headers, map[string]string{"authorization": "Token t"}) {
		t.Errorf("headers = %#v", headers)
	}
	emit.FlushCoverageNotes()
	if note := buf.String(); !strings.Contains(note, "`api_key`") {
		t.Errorf("want a note naming api_key, got: %s", note)
	}
}

// mcp.json documents no per-server timeout, so a timeout is named as lost
// instead of being dropped without a word.
func TestUserMCPServers_TimeoutIsNamedAsLost(t *testing.T) {
	buf := captureNotes(t)
	servers := New().UserMCPServers([]spec.Entry{
		{Kind: spec.KindMCP, Name: "search", Meta: map[string]any{"type": "http", "url": "https://search.test/mcp", "timeout": 1800}},
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	})
	if _, ok := servers["search"].(map[string]any)["timeout"]; ok {
		t.Errorf("timeout reached mcp.json: %#v", servers["search"])
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"`timeout`", "1 mcp", "~/.openhands/mcp.json"} {
		if !strings.Contains(note, want) {
			t.Errorf("note lacks %q: %s", want, note)
		}
	}
}
