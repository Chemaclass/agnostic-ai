package openhands

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/internal/emit"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Current OpenHands releases ignore the config.toml [mcp] section (#1252),
// so writing it raises a note that points at the file they do read.
func TestEmit_MCPConfigTOMLNotesItIsIgnored(t *testing.T) {
	testutil.TempCwd(t)
	emit.ResetCoverageNotes()
	t.Cleanup(emit.ResetCoverageNotes)
	buf := &strings.Builder{}
	prev := emit.Warner
	emit.Warner = buf
	t.Cleanup(func() { emit.Warner = prev })

	entries := []spec.Entry{{Kind: spec.KindMCP, Name: "docs", Path: "mcps/docs.yaml", Meta: map[string]any{"command": "docs-mcp"}}}
	if err := New().Emit(emit.NewSession(), spec.NewBundle(entries), &config.Config{}, false); err != nil {
		t.Fatal(err)
	}
	emit.FlushCoverageNotes()
	note := buf.String()
	for _, want := range []string{"config.toml [mcp]", "ignore", "~/.openhands/mcp.json", "sync --global"} {
		if !strings.Contains(note, want) {
			t.Errorf("note lacks %q: %s", want, note)
		}
	}
}
