package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/lsp"
)

func TestLSPLinter_ReportsConfigParseFailure(t *testing.T) {
	dir := newProject(t)
	path := filepath.Join(dir, config.ConfigFileName)
	writeFile(t, path, "version: 1\ntargets: [claude]\n: : :\n")
	diagnostics, err := lspLinter(dir)
	if err == nil {
		t.Fatal("loading failure must return an error")
	}
	requireLSPLoadDiagnostic(t, diagnostics, path, "AAI-004", 1)
}

func TestLSPLinter_ReportsSpecParseFailure(t *testing.T) {
	dir := newProject(t)
	path := filepath.Join(dir, ".agnostic-ai", "rules", "broken.md")
	writeFile(t, path, "---\nname: broken\n: : :\n---\nBody\n")
	diagnostics, err := lspLinter(dir)
	if err == nil {
		t.Fatal("loading failure must return an error")
	}
	requireLSPLoadDiagnostic(t, diagnostics, path, "AAI-001", 1)
}

func TestLSPLinter_ReportsLocalConfigParseFailure(t *testing.T) {
	dir := newProject(t)
	path := filepath.Join(dir, config.LocalOverrideFileName)
	writeFile(t, path, "targets: [claude]\n# local settings\n: : :\n")
	diagnostics, err := lspLinter(dir)
	if err == nil {
		t.Fatal("loading failure must return an error")
	}
	requireLSPLoadDiagnostic(t, diagnostics, path, "AAI-004", 1)
}

func TestLSPLinter_ReportsYAMLSpecParseFailure(t *testing.T) {
	dir := newProject(t)
	path := filepath.Join(dir, ".agnostic-ai", "hooks", "broken.yaml")
	writeFile(t, path, "name: broken\nevent: PreToolUse\n: : :\n")
	diagnostics, err := lspLinter(dir)
	if err == nil {
		t.Fatal("loading failure must return an error")
	}
	requireLSPLoadDiagnostic(t, diagnostics, path, "AAI-001", 1)
}

func requireLSPLoadDiagnostic(t *testing.T, diagnostics map[string][]lsp.Diagnostic, path, code string, line int) {
	t.Helper()
	got := diagnostics[path]
	if len(got) != 1 {
		t.Fatalf("%s diagnostics = %+v, want one load error", path, got)
	}
	if got[0].Code != code || got[0].Severity != lsp.SeverityError || got[0].Range.Start.Line != line {
		t.Errorf("diagnostic = %+v, want code %s, error severity and line %d", got[0], code, line)
	}
}

func TestLSPLinter_ReportsLayerResolutionFailureAsWorkspaceError(t *testing.T) {
	dir := newProject(t)
	writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\nbuiltins: [unknown-layer]\n")
	diagnostics, err := lspLinter(dir)
	if err == nil || !strings.Contains(err.Error(), "unknown-layer") {
		t.Fatalf("layer failure = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Errorf("failure without source must not invent a document: %+v", diagnostics)
	}
}

func TestLSPLinter_MergedConfigFailureDoesNotInventSourcePosition(t *testing.T) {
	dir := newProject(t)
	writeFile(t, filepath.Join(dir, config.LocalOverrideFileName), "targets: {wrong: shape}\n")
	diagnostics, err := lspLinter(dir)
	if err == nil || !strings.Contains(err.Error(), "parse merged config") {
		t.Fatalf("merged config failure = %v", err)
	}
	if len(diagnostics) != 0 {
		t.Errorf("merged YAML line is not an original source position: %+v", diagnostics)
	}
}

func TestLSPLinter_CleanProjectReturnsSuccessfulEmptyResult(t *testing.T) {
	dir := newProject(t)
	writeFile(t, filepath.Join(dir, config.ConfigFileName), "version: 1\ntargets: [claude]\n")
	diagnostics, err := lspLinter(dir)
	if err != nil || len(diagnostics) != 0 {
		t.Errorf("clean project = %+v, %v", diagnostics, err)
	}
}

func TestLSPLoadFailure_UsesSourceWithoutParserPosition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "broken.md")
	diagnostics, err := lspLoadFailure(dir, errs.Coded(errs.CodeSpecParse, "%s: cannot parse document", path))
	if err == nil {
		t.Fatal("failure must not become successful lint")
	}
	requireLSPLoadDiagnostic(t, diagnostics, path, "AAI-001", 0)
}

func TestLSPErrorLocation_ParsesWindowsPathAndColumn(t *testing.T) {
	path, position := lspErrorLocation(errs.Coded(errs.CodeSpecParse, `C:\project with spaces\rules\broken.md:4:7: bad YAML`))
	if path != `C:\project with spaces\rules\broken.md` || position != (lsp.Position{Line: 3, Character: 6}) {
		t.Errorf("location = %q, %+v", path, position)
	}
}

func TestLSPLinter_ProtocolClearsCorrectedAndDeletedSources(t *testing.T) {
	dir := newProject(t)
	configPath := filepath.Join(dir, config.ConfigFileName)
	specPath := filepath.Join(dir, ".agnostic-ai", "rules", "broken.md")
	triggerPath := filepath.Join(dir, ".agnostic-ai", "rules", "trigger.md")
	goodConfig := "version: 1\ntargets: [claude]\n"
	writeFile(t, configPath, goodConfig)
	writeFile(t, triggerPath, "---\nname: trigger\n---\nBe brief.\n")
	call := 0
	linter := func(root string) (map[string][]lsp.Diagnostic, error) {
		call++
		switch call {
		case 2:
			writeFile(t, configPath, goodConfig+": : :\n")
		case 3:
			writeFile(t, configPath, goodConfig)
		case 4:
			writeFile(t, specPath, "---\nname: broken\n: : :\n---\nBody.\n")
		case 5:
			if err := os.Remove(specPath); err != nil {
				t.Fatal(err)
			}
		}
		return lspLinter(root)
	}
	var in, out bytes.Buffer
	writer := lsp.NewWriter(&in)
	send := func(message map[string]any) {
		t.Helper()
		if err := writer.Send(message); err != nil {
			t.Fatal(err)
		}
	}
	uri := func(path string) string {
		path = filepath.ToSlash(path)
		if !strings.HasPrefix(path, "/") {
			path = "/" + path
		}
		return (&url.URL{Scheme: "file", Path: path}).String()
	}
	send(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"rootUri": uri(dir)}})
	send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didOpen", "params": map[string]any{"textDocument": map[string]any{"uri": uri(triggerPath)}}})
	for _, path := range []string{configPath, triggerPath, specPath, triggerPath} {
		send(map[string]any{"jsonrpc": "2.0", "method": "textDocument/didSave", "params": map[string]any{"textDocument": map[string]any{"uri": uri(path)}}})
	}
	send(map[string]any{"jsonrpc": "2.0", "method": "exit"})
	if err := lsp.New(&in, &out, linter).Run(); err != nil {
		t.Fatal(err)
	}
	reader := lsp.NewReader(&out)
	found, cleared := map[string]bool{}, map[string]bool{}
	for {
		message, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if message.Method != "textDocument/publishDiagnostics" {
			continue
		}
		var params struct {
			URI         string           `json:"uri"`
			Diagnostics []lsp.Diagnostic `json:"diagnostics"`
		}
		if err := json.Unmarshal(message.Params, &params); err != nil {
			t.Fatal(err)
		}
		if len(params.Diagnostics) > 0 {
			found[params.URI] = true
		} else if found[params.URI] {
			cleared[params.URI] = true
		}
	}
	for _, path := range []string{configPath, specPath} {
		if !found[uri(path)] || !cleared[uri(path)] {
			t.Errorf("%s must publish error then clear: found=%v, cleared=%v", path, found[uri(path)], cleared[uri(path)])
		}
	}
}

func TestLSPLinter_ReportsUnknownConfigKeyPosition(t *testing.T) {
	dir := newProject(t)
	path := filepath.Join(dir, config.ConfigFileName)
	writeFile(t, path, "version: 1\ntargets: [claude]\nunknown-setting: true\n")
	diagnostics, err := lspLinter(dir)
	if err == nil {
		t.Fatal("unknown setting must fail loading")
	}
	requireLSPLoadDiagnostic(t, diagnostics, path, "AAI-004", 2)
}
