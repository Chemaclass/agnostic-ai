package trae

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestEmit_MCP_StdioWritesTraeMCPJSON(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command": "npx",
				"args":    []any{"-y", "@modelcontextprotocol/server-filesystem", "."},
				"env":     map[string]any{"ALLOWED_PATHS": "."},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".trae/mcp.json"))
	if err != nil {
		t.Fatalf("missing .trae/mcp.json: %v", err)
	}
	body := string(got)
	for _, want := range []string{`"mcpServers"`, `"fs"`, `"command": "npx"`, `"ALLOWED_PATHS"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
	if strings.Contains(body, `"type"`) {
		t.Errorf("trae's docs key no type field on either transport; got:\n%s", body)
	}
}

func TestEmit_MCP_HTTPWritesURLAndHeadersWithNoType(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "linear",
			Meta: map[string]any{
				"type":    "http",
				"url":     "https://mcp.linear.app",
				"headers": map[string]any{"Authorization": "Bearer x"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".trae/mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(got)
	for _, want := range []string{`"url": "https://mcp.linear.app"`, `"Authorization"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in %s", want, body)
		}
	}
	// The internal spec's own "type" meta selects the branch below but
	// must never itself reach the file: docs.trae.ai/ide/add-mcp-servers
	// has no `type` key in either of its two worked examples.
	if strings.Contains(body, `"type"`) {
		t.Errorf("http transport must not leak a type key, got:\n%s", body)
	}
}

// TestEmit_MCP_DisabledHasNoFileBasedEffect locks the evidence this
// adapter is built on: docs.trae.ai/ide/add-mcp-servers documents no
// per-server disable key, only a project-level MCP toggle under
// Settings > MCP, so a spec's disabled: true must not reach the file.
func TestEmit_MCP_DisabledHasNoFileBasedEffect(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx", "disabled": true}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".trae/mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "disabled") {
		t.Errorf("disabled must not reach .trae/mcp.json: %s", got)
	}

	emit.FlushCoverageNotes()
	if !strings.Contains(buf.String(), "`disabled` on 1 mcp has no effect on trae") {
		t.Errorf("expected a field no-op note, got: %s", buf.String())
	}
}

// TestEmit_MCP_AutoApproveNeverEmitted guards against reintroducing a
// Cline/Roo-Code-style autoApprove key: it appears in some real-world
// .trae-adjacent files but never in Trae's own docs, and this adapter
// only ever writes the fields buildMCPServer explicitly sets, so a spec
// author cannot smuggle it in through Meta.
func TestEmit_MCP_AutoApproveNeverEmitted(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP,
			Name: "fs",
			Meta: map[string]any{
				"command":     "npx",
				"autoApprove": []any{"read_file"},
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".trae/mcp.json"))
	if strings.Contains(got, "autoApprove") {
		t.Errorf("autoApprove is not documented for trae and must not be emitted:\n%s", got)
	}
}

func TestEmit_MCP_FileOverride(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	cfg := &config.Config{
		Outputs: map[string]config.Output{
			"trae": {MCPFile: "vendor/trae-mcp.json"},
		},
	}
	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), cfg, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "vendor/trae-mcp.json")); err != nil {
		t.Errorf("expected override path written: %v", err)
	}
}

func TestEmit_NoMCPJSONWhenNoMCPs(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{{Kind: spec.KindRule, Name: "r1", Body: "x"}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".trae/mcp.json")); !os.IsNotExist(err) {
		t.Errorf("expected no .trae/mcp.json when no MCP entries, err=%v", err)
	}
}

// TestEmit_MCP_EntryMissingRequiredFieldSkipped confirms an entry with
// nothing to run or connect to (no command, no url) renders empty and
// is dropped rather than writing a dead {} block.
func TestEmit_MCP_EntryMissingRequiredFieldSkipped(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "empty-stdio", Meta: map[string]any{}},
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".trae/mcp.json"))
	if strings.Contains(got, "empty-stdio") {
		t.Errorf("expected the fieldless entry to be dropped:\n%s", got)
	}
	if !strings.Contains(got, `"fs"`) {
		t.Errorf("expected the valid entry to still be written:\n%s", got)
	}
}

// swapNoteWarner redirects emit's coverage-note output to a buffer for
// the duration of the test and resets both the warner and the note
// buffer afterward.
func swapNoteWarner(t *testing.T) *strings.Builder {
	t.Helper()
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	return buf
}
