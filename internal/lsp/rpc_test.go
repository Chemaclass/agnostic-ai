package lsp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTrip_SimpleMessage(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	w := NewWriter(&buf)
	msg := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"}
	if err := w.Send(msg); err != nil {
		t.Fatalf("Send: %v", err)
	}

	r := NewReader(&buf)
	got, err := r.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Method != "initialize" {
		t.Errorf("method = %q, want %q", got.Method, "initialize")
	}
}

func TestReader_MultipleMessages(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	w := NewWriter(&buf)
	for i := range 3 {
		_ = w.Send(map[string]any{"jsonrpc": "2.0", "id": i, "method": "ping"})
	}
	r := NewReader(&buf)
	for i := range 3 {
		msg, err := r.Read()
		if err != nil {
			t.Fatalf("msg %d: %v", i, err)
		}
		if msg.Method != "ping" {
			t.Errorf("msg %d method = %q", i, msg.Method)
		}
	}
}

func TestServer_Initialize(t *testing.T) {
	t.Parallel()
	var in, out bytes.Buffer
	w := NewWriter(&in)
	_ = w.Send(map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{"rootUri": "file:///tmp/proj"},
	})
	_ = w.Send(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown"})
	// no exit; Run terminates via io.EOF after buffer is drained

	srv := New(&in, &out, nil)
	_ = srv.Run()

	r := NewReader(strings.NewReader(out.String()))
	resp, err := r.Read()
	if err != nil {
		t.Fatalf("read initialize response: %v", err)
	}
	raw, _ := json.Marshal(resp.Result)
	if !strings.Contains(string(raw), "textDocumentSync") {
		t.Errorf("initialize result missing capabilities: %s", raw)
	}
}

func TestURIConversion(t *testing.T) {
	t.Parallel()
	cases := []struct{ uri, path string }{
		{"file:///tmp/foo.md", "/tmp/foo.md"},
		{"file:///home/user/proj/rules/x.md", "/home/user/proj/rules/x.md"},
	}
	for _, c := range cases {
		got := uriToPath(c.uri)
		if got != filepath.FromSlash(c.path) {
			t.Errorf("uriToPath(%q) = %q, want %q", c.uri, got, c.path)
		}
	}
}

func TestServer_ClearsResolvedDiagnosticsAcrossDocuments(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a.md"), filepath.Join(root, "b.md")
	calls := 0
	linter := func(string) (map[string][]Diagnostic, error) {
		calls++
		if calls == 1 {
			return map[string][]Diagnostic{a: {{Severity: SeverityWarning, Code: "LINT001", Message: "old finding"}}}, nil
		}
		return map[string][]Diagnostic{}, nil
	}
	messages := runDiagnosticProtocol(t, root, linter, pathToURI(a), pathToURI(b))
	found, cleared := false, false
	for _, message := range messages {
		if message.Method != "textDocument/publishDiagnostics" {
			continue
		}
		var params struct {
			URI         string       `json:"uri"`
			Diagnostics []Diagnostic `json:"diagnostics"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params.URI != pathToURI(a) {
			continue
		}
		if len(params.Diagnostics) > 0 {
			found = true
		} else if found {
			cleared = true
		}
	}
	if !found || !cleared {
		t.Errorf("file A finding must publish and clear after saving B: found=%v, cleared=%v", found, cleared)
	}
}

func runDiagnosticProtocol(t *testing.T, root string, linter Linter, uris ...string) []*Message {
	t.Helper()
	var in, out bytes.Buffer
	writer := NewWriter(&in)
	send := func(message map[string]any) {
		t.Helper()
		if err := writer.Send(message); err != nil {
			t.Fatal(err)
		}
	}
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"rootUri": pathToURI(root)}})
	for _, uri := range uris {
		send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didSave", "params": map[string]any{"textDocument": map[string]any{"uri": uri}}})
	}
	send(map[string]any{"jsonrpc": "2.0", "method": "exit"})
	if err := New(&in, &out, linter).Run(); err != nil {
		t.Fatal(err)
	}
	reader := NewReader(&out)
	var messages []*Message
	for {
		message, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message)
	}
	return messages
}

func TestServer_FailedAnalysisPreservesEarlierDiagnostics(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a.md"), filepath.Join(root, "b.md")
	calls := 0
	linter := func(string) (map[string][]Diagnostic, error) {
		calls++
		if calls == 1 {
			return map[string][]Diagnostic{a: {{Code: "LINT001", Message: "earlier finding"}}}, nil
		}
		return map[string][]Diagnostic{b: {{Severity: SeverityError, Code: "AAI-001", Message: "load failed"}}, a: {}}, fmt.Errorf("analysis failed")
	}
	messages := runDiagnosticProtocol(t, root, linter, pathToURI(a), pathToURI(b))
	loadPublished := false
	for _, message := range messages {
		if message.Method != "textDocument/publishDiagnostics" {
			continue
		}
		var params struct {
			URI         string       `json:"uri"`
			Diagnostics []Diagnostic `json:"diagnostics"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			t.Fatal(err)
		}
		if params.URI == pathToURI(a) && len(params.Diagnostics) == 0 {
			t.Error("failed analysis cleared an earlier finding")
		}
		if params.URI == pathToURI(b) && len(params.Diagnostics) == 1 && params.Diagnostics[0].Code == "AAI-001" {
			loadPublished = true
		}
	}
	if !loadPublished {
		t.Error("known-source load failure was not published")
	}
}

func TestServer_UnlocatedFailureReportsWorkspaceErrorWithoutClearing(t *testing.T) {
	root := t.TempDir()
	a, b := filepath.Join(root, "a.md"), filepath.Join(root, "b.md")
	calls := 0
	linter := func(string) (map[string][]Diagnostic, error) {
		calls++
		if calls == 1 {
			return map[string][]Diagnostic{a: {{Message: "earlier finding"}}}, nil
		}
		return nil, fmt.Errorf("layer resolution failed")
	}
	messages := runDiagnosticProtocol(t, root, linter, pathToURI(a), pathToURI(b))
	workspaceError := false
	for _, message := range messages {
		if message.Method == "window/showMessage" {
			var params struct {
				Type    int    `json:"type"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(message.Params, &params); err != nil {
				t.Fatal(err)
			}
			workspaceError = params.Type == 1 && strings.Contains(params.Message, "layer resolution failed") && strings.Contains(params.Message, "save a project file")
		}
		if message.Method == "textDocument/publishDiagnostics" {
			var params struct {
				URI         string       `json:"uri"`
				Diagnostics []Diagnostic `json:"diagnostics"`
			}
			if err := json.Unmarshal(message.Params, &params); err != nil {
				t.Fatal(err)
			}
			if params.URI == pathToURI(a) && len(params.Diagnostics) == 0 {
				t.Error("unlocated failure cleared earlier finding")
			}
			if params.URI == pathToURI(b) {
				t.Error("failed analysis published a clean trigger")
			}
		}
	}
	if !workspaceError {
		t.Error("missing actionable workspace error")
	}
}

func TestURIConversion_RoundTripsEscapedFileNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rule with spaces #100%.md")
	uri := pathToURI(path)
	if strings.Contains(uri, " ") || strings.Contains(uri, "#") || !strings.Contains(uri, "%25") {
		t.Errorf("file URI did not escape path: %s", uri)
	}
	if got := uriToPath(uri); got != path {
		t.Errorf("URI roundtrip = %q, want %q", got, path)
	}
}

func TestURIConversion_PreservesNetworkAuthority(t *testing.T) {
	uri := "file://server/share/rule%20one.md"
	if got := pathToURI(uriToPath(uri)); got != uri {
		t.Errorf("network URI roundtrip = %q, want %q", got, uri)
	}
}

func TestServer_UsesClientDocumentURI(t *testing.T) {
	for _, uri := range []string{"file://localhost/tmp/rule%20one.md", "file://server/share/rule%20one.md"} {
		t.Run(uri, func(t *testing.T) {
			linter := func(string) (map[string][]Diagnostic, error) {
				return map[string][]Diagnostic{uriToPath(uri): {{Message: "finding"}}}, nil
			}
			messages := runDiagnosticProtocol(t, t.TempDir(), linter, uri)
			found := false
			for _, message := range messages {
				if message.Method != "textDocument/publishDiagnostics" {
					continue
				}
				var params struct {
					URI         string       `json:"uri"`
					Diagnostics []Diagnostic `json:"diagnostics"`
				}
				if err := json.Unmarshal(message.Params, &params); err != nil {
					t.Fatal(err)
				}
				if params.URI != uri {
					t.Errorf("published URI = %q, want client URI %q", params.URI, uri)
				}
				if len(params.Diagnostics) == 0 {
					t.Error("analysis cleared the document despite its finding")
				} else {
					found = true
				}
			}
			if !found {
				t.Error("client document finding was not published")
			}
		})
	}
}

func TestServer_OpeningEquivalentURIKeepsItsFinding(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "rule@one.md")
	canonicalURI := pathToURI(path)
	clientURI := strings.ReplaceAll(canonicalURI, "@", "%40")
	otherURI := pathToURI(filepath.Join(root, "other.md"))
	linter := func(string) (map[string][]Diagnostic, error) {
		return map[string][]Diagnostic{path: {{Message: "finding"}}}, nil
	}
	messages := runDiagnosticProtocol(t, root, linter, otherURI, clientURI)
	found := false
	for _, message := range messages {
		if message.Method != "textDocument/publishDiagnostics" {
			continue
		}
		var params struct {
			URI         string       `json:"uri"`
			Diagnostics []Diagnostic `json:"diagnostics"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			t.Fatal(err)
		}
		if uriToPath(params.URI) != path {
			continue
		}
		if len(params.Diagnostics) == 0 {
			t.Errorf("equivalent URI %q cleared a document with a finding", params.URI)
		}
		if params.URI == clientURI && len(params.Diagnostics) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("new client URI did not receive its finding")
	}
}

func TestServer_DeclaresOpenSaveWithoutBufferChanges(t *testing.T) {
	var in, out bytes.Buffer
	writer := NewWriter(&in)
	_ = writer.Send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize"})
	if err := New(&in, &out, nil).Run(); err != nil {
		t.Fatal(err)
	}
	response, err := NewReader(&out).Read()
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Capabilities struct {
			TextDocumentSync struct {
				OpenClose bool `json:"openClose"`
				Change    int  `json:"change"`
				Save      *struct {
					IncludeText bool `json:"includeText"`
				} `json:"save"`
			} `json:"textDocumentSync"`
		} `json:"capabilities"`
	}
	resultJSON, err := json.Marshal(response.Result)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		t.Fatalf("saved-file synchronization contract: %v", err)
	}
	sync := result.Capabilities.TextDocumentSync
	if !sync.OpenClose || sync.Change != 0 || sync.Save == nil || sync.Save.IncludeText {
		t.Errorf("saved-file synchronization = %+v", sync)
	}
}

func TestServer_IgnoresUnsavedBufferChanges(t *testing.T) {
	var in, out bytes.Buffer
	writer := NewWriter(&in)
	calls := 0
	for _, method := range []string{"textDocument/didOpen", "textDocument/didChange", "textDocument/didSave"} {
		_ = writer.Send(map[string]any{"jsonrpc": "2.0", "method": method, "params": map[string]any{"textDocument": map[string]any{"uri": "file:///tmp/project/rules/a.md"}, "contentChanges": []any{map[string]any{"text": "Unsaved body"}}}})
	}
	linter := func(string) (map[string][]Diagnostic, error) { calls++; return map[string][]Diagnostic{}, nil }
	if err := New(&in, &out, linter).Run(); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Errorf("lint calls = %d, want open and save only", calls)
	}
}
