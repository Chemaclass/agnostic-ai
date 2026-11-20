package cli

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const handWrittenCursorMCP = `{"mcpServers": {"linear": {"url": "https://mcp.linear.app/sse"}}}` + "\n"

func setUpCursorMCPProject(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "## Shared\n\nShared text.\n")
	mustWriteFile(t, ".agnostic-ai/mcps/foo.yaml", "name: foo\ncommand: foo-mcp\n")
	mustWriteFile(t, ".cursor/mcp.json", handWrittenCursorMCP)
}

// A sync replaced a hand-written .cursor/mcp.json it never wrote and
// dropped its servers, with no backup and no warning.
func TestSync_KeepsAHandWrittenMCPFileItHasNotImported(t *testing.T) {
	setUpCursorMCPProject(t)

	for _, args := range [][]string{{"sync"}, {"sync", "--json"}, {"sync", "--plan"}, {"sync", "--check"}} {
		_, err := runCLI(t, args...)
		if err == nil || !strings.Contains(err.Error(), ".cursor/mcp.json") || !strings.Contains(err.Error(), "agnostic-ai import cursor") {
			t.Errorf("%s: err = %v, want a stop naming the file and the import step", strings.Join(args, " "), err)
		}
	}
	if got := readFile(t, ".cursor/mcp.json"); got != handWrittenCursorMCP {
		t.Errorf(".cursor/mcp.json changed:\n%s", got)
	}
}

func TestSync_WritesAnMCPFileOnceImported(t *testing.T) {
	setUpCursorMCPProject(t)
	if out, err := runCLI(t, "import", "cursor"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	runSyncOK(t)

	got := readFile(t, ".cursor/mcp.json")
	for _, want := range []string{`"linear"`, `"foo"`} {
		if !strings.Contains(got, want) {
			t.Errorf(".cursor/mcp.json lacks %s:\n%s", want, got)
		}
	}
	// Import can read a server back in another form, such as a literal
	// key as a reference, so the first write keeps the file.
	if got := readFile(t, ".cursor/mcp.json.bak"); got != handWrittenCursorMCP {
		t.Errorf(".cursor/mcp.json.bak = %q, want the hand-written file", got)
	}
}

func TestSync_BackupReplacesAHandWrittenMCPFileAndKeepsIt(t *testing.T) {
	setUpCursorMCPProject(t)

	if out, err := runCLI(t, "sync", "--backup"); err != nil {
		t.Fatalf("sync --backup: %v\n%s", err, out)
	}

	if got := readFile(t, ".cursor/mcp.json.bak"); got != handWrittenCursorMCP {
		t.Errorf(".cursor/mcp.json.bak = %q, want the hand-written file", got)
	}
	if got := readFile(t, ".cursor/mcp.json"); !strings.Contains(got, `"foo"`) {
		t.Errorf(".cursor/mcp.json was not written:\n%s", got)
	}
}

// A file whose servers all have specs loses nothing, so sync writes it.
func TestSync_WritesAnMCPFileWhoseServersAllHaveSpecs(t *testing.T) {
	setUpCursorMCPProject(t)
	mustWriteFile(t, ".cursor/mcp.json", `{"mcpServers": {"foo": {"command": "old"}}}`+"\n")

	runSyncOK(t)

	if got := readFile(t, ".cursor/mcp.json"); !strings.Contains(got, "foo-mcp") {
		t.Errorf(".cursor/mcp.json was not written:\n%s", got)
	}
}

// A file sync wrote keeps the hand-edit backup it always had, so the stop
// leaves it alone.
func TestSync_KeepsWritingAnMCPFileItWrote(t *testing.T) {
	setUpCursorMCPProject(t)
	if err := os.Remove(".cursor/mcp.json"); err != nil {
		t.Fatal(err)
	}
	runSyncOK(t)
	mustWriteFile(t, ".agnostic-ai/mcps/bar.yaml", "name: bar\ncommand: bar-mcp\n")
	if err := os.Remove(".agnostic-ai/mcps/foo.yaml"); err != nil {
		t.Fatal(err)
	}

	runSyncOK(t)

	if got := readFile(t, ".cursor/mcp.json"); !strings.Contains(got, `"bar"`) || strings.Contains(got, `"foo"`) {
		t.Errorf(".cursor/mcp.json = %s, want only bar", got)
	}
	if _, err := os.Stat(".cursor/mcp.json.bak"); err == nil {
		t.Error("sync backed up a file it wrote and nobody edited")
	}
}

// --keep-edits leaves an imported file it never wrote in place.
func TestSync_KeepEditsLeavesAnImportedMCPFileInPlace(t *testing.T) {
	setUpCursorMCPProject(t)
	if out, err := runCLI(t, "import", "cursor"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}

	if out, err := runCLI(t, "sync", "--keep-edits"); err != nil {
		t.Fatalf("sync --keep-edits: %v\n%s", err, out)
	}

	if got := readFile(t, ".cursor/mcp.json"); got != handWrittenCursorMCP {
		t.Errorf(".cursor/mcp.json changed:\n%s", got)
	}
}

// With `<path>.bak` taken, the file stays in place, and the ledger does
// not take it for a file sync wrote, so the stop still guards it.
func TestSync_TakenBackupLeavesAnImportedMCPFileUnledgered(t *testing.T) {
	setUpCursorMCPProject(t)
	if out, err := runCLI(t, "import", "cursor"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	mustWriteFile(t, ".cursor/mcp.json.bak", "older\n")

	runSyncOK(t)

	if got := readFile(t, ".cursor/mcp.json"); got != handWrittenCursorMCP {
		t.Errorf(".cursor/mcp.json changed:\n%s", got)
	}
	if slices.Contains(readStateFile(".").Outputs, ".cursor/mcp.json") {
		t.Error("the ledger lists a file sync did not write")
	}
}

// The first sync with an MCP spec replaced a hand-written Claude Code
// .mcp.json and dropped its servers.
func TestSync_KeepsAHandWrittenClaudeMCPFile(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	const mine = `{"mcpServers": {"mine": {"command": "mine-mcp"}}}` + "\n"
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "## Shared\n\nShared text.\n")
	mustWriteFile(t, ".agnostic-ai/mcps/fs.yaml", "name: fs\ncommand: fs-mcp\n")
	mustWriteFile(t, ".mcp.json", mine)

	_, err := runCLI(t, "sync")
	if err == nil || !strings.Contains(err.Error(), "server mine") || !strings.Contains(err.Error(), "agnostic-ai import claude") {
		t.Errorf("err = %v, want a stop naming server mine and the import step", err)
	}
	if got := readFile(t, ".mcp.json"); got != mine {
		t.Errorf(".mcp.json changed:\n%s", got)
	}
}
