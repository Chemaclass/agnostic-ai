package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/mcpprobe"
	"github.com/chemaclass/agnostic-ai/internal/overview"
)

// fakeMCP answers initialize and tools/list over streamable HTTP when the
// request carries the expected bearer token.
func fakeMCP(t *testing.T, token string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.ID == nil {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		result := map[string]any{"protocolVersion": mcpprobe.ProtocolVersion}
		if req.Method == "tools/list" {
			result = map[string]any{"tools": []any{map[string]any{"name": "send_email", "description": "Send one email."}}}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": *req.ID, "result": result})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestMCPTools_ListsAndCachesToolsTheOverviewThenReadsOffline(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
	source := filepath.Join(home, "source")
	t.Setenv("AGNOSTIC_AI_HOME", source)
	t.Setenv("FAKE_MCP_TOKEN", "s3cret")
	srv := fakeMCP(t, "s3cret")
	mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude]\n")
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Shared\n")
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "mail.yaml"), fmt.Sprintf("name: mail\ntype: http\nurl: %s\nheaders:\n  Authorization: Bearer ${FAKE_MCP_TOKEN}\n", srv.URL))
	mustWriteGlobalTest(t, filepath.Join(source, "mcps", "unset.yaml"), fmt.Sprintf("name: unset\ntype: http\nurl: %s\nheaders:\n  Authorization: Bearer ${NOT_SET_ANYWHERE_TOKEN}\n", srv.URL))

	out, err := runCLI(t, "mcp", "tools", "--global")
	if err != nil {
		t.Fatalf("mcp tools: %v\n%s", err, out)
	}
	for _, want := range []string{"mail: 1 tool(s): send_email", "unset: not listed: NOT_SET_ANYWHERE_TOKEN is not set"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "s3cret") {
		t.Error("output shows the resolved credential")
	}

	srv.Close() // The overview must not connect.
	in, err := globalOverviewInputs()
	if err != nil {
		t.Fatal(err)
	}
	tools := map[string]*overview.ToolListing{}
	for _, s := range overview.Build(in).Sections {
		for _, it := range s.Items {
			if it.Tools != nil {
				tools[it.Name] = it.Tools
			}
		}
	}
	if l := tools["mail"]; l == nil || l.State != overview.ToolsListed || len(l.Tools) != 1 || l.Tools[0].Name != "send_email" || l.Command != "agnostic-ai mcp tools --global" {
		t.Errorf("mail = %+v", l)
	}
	if l := tools["unset"]; l == nil || l.State != overview.ToolsFailed {
		t.Errorf("unset = %+v", l)
	}
}
