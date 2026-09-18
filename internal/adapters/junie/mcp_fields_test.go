package junie

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// junie.jetbrains.com/docs/junie-cli-mcp-configuration.html documents
// neither key in its mcp.json structure block, and a server read from
// that file is "imported to the list of MCP servers and enabled by
// default". A written `disabled: true` therefore claimed a state Junie
// never enters: the user reads their own spec and believes the server
// is off while Junie runs it (#858).
func TestEmit_MCP_StripsDisabledAndDescription(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{
			Kind: spec.KindMCP, Name: "fs",
			Meta: map[string]any{
				"command":     "npx",
				"args":        []any{"-y", "@modelcontextprotocol/server-filesystem"},
				"disabled":    true,
				"description": "local filesystem",
			},
		},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(dir, ".junie/mcp/mcp.json"))
	for _, unwanted := range []string{`"disabled"`, `"description"`, "local filesystem"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("%s must not reach .junie/mcp/mcp.json:\n%s", unwanted, got)
		}
	}
	for _, want := range []string{`"command": "npx"`, `"args"`} {
		if !strings.Contains(got, want) {
			t.Errorf("documented keys must still emit, missing %q in:\n%s", want, got)
		}
	}

	emit.FlushCoverageNotes()
	notes := buf.String()
	if !strings.Contains(notes, "`disabled` on 1 mcp has no effect on junie") {
		t.Errorf("expected a disabled no-op note, got: %s", notes)
	}
	if !strings.Contains(notes, "`description` on 1 mcp has no effect on junie") {
		t.Errorf("expected a description no-op note, got: %s", notes)
	}
}

// Neither key set means neither note fires.
func TestEmit_MCP_NoNotesWhenNeitherKeySet(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	buf := swapNoteWarner(t)

	entries := []spec.Entry{
		{Kind: spec.KindMCP, Name: "fs", Meta: map[string]any{"command": "npx"}},
	}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	if strings.Contains(buf.String(), "mcp") {
		t.Errorf("expected no MCP coverage note, got: %s", buf.String())
	}
}

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
