package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The first sync shows what each tool now reads; later ones do not (#1614).
func TestSync_FirstRunShowsWhatEachToolReads(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review a PR.\n---\nReview.\n")
	log := captureLog(t)

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	for _, want := range []string{"claude now reads", "codex now reads", ".agents/skills/", "review"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("first sync output is missing %q:\n%s", want, log.String())
		}
	}

	log.Reset()
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("second sync: %v\n%s", err, out)
	}
	if strings.Contains(log.String(), "now reads") {
		t.Errorf("a later sync repeated the summary:\n%s", log.String())
	}
}

// use for a tool already in targets prints nothing of its own, so a first
// sync still lists what each tool reads.
func TestUse_AlreadyInUseStillShowsTheFirstSyncList(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	log := captureLog(t)

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}
	if !strings.Contains(log.String(), "codex now reads") {
		t.Errorf("first sync under use lost its list:\n%s", log.String())
	}
}

func TestSync_FirstRunListIsQuietUnderQuiet(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	log := captureLog(t)

	if out, err := runCLI(t, "sync", "--quiet"); err != nil {
		t.Fatalf("sync --quiet: %v\n%s", err, out)
	}
	if strings.Contains(log.String(), "now reads") {
		t.Errorf("--quiet printed the first-sync list:\n%s", log.String())
	}
}

// A first sync under use lists every target, not only the added one.
func TestUse_FirstSyncListsEveryTarget(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	log := captureLog(t)

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}
	for _, want := range []string{"claude now reads", "codex now reads"} {
		if n := strings.Count(log.String(), want); n != 1 {
			t.Errorf("%q printed %d times, want once:\n%s", want, n, log.String())
		}
	}
}

// A target sync could not load writes nothing, so the list leaves it out.
func TestSync_FirstRunListLeavesOutATargetThatDidNotEmit(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, acme-tool]\n")
	log := captureLog(t)

	_, _ = runCLI(t, "sync")

	if !strings.Contains(log.String(), "codex now reads") || strings.Contains(log.String(), "acme-tool now reads") {
		t.Errorf("list should name codex and not acme-tool:\n%s", log.String())
	}
}

// A kind the target cannot load is not listed as read.
func TestSync_FirstRunListLeavesOutUnsupportedKinds(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [jules]\n")
	mustWriteFile(t, ".agnostic-ai/rules/style.md", "---\nname: style\ndescription: Style.\n---\nKeep it short.\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review a PR.\n---\nReview.\n")
	log := captureLog(t)

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(log.String(), "jules now reads") || strings.Contains(log.String(), "1 skill") {
		t.Errorf("list should name jules without the unsupported skill:\n%s", log.String())
	}
}

// A first sync for some targets leaves the rest to be listed by a later one.
func TestSync_APartialFirstSyncLeavesTheOtherToolsToALaterOne(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	log := captureLog(t)

	if out, err := runCLI(t, "sync", "-t", "claude"); err != nil {
		t.Fatalf("sync -t claude: %v\n%s", err, out)
	}
	if strings.Contains(log.String(), "codex now reads") {
		t.Errorf("a claude-only sync listed codex:\n%s", log.String())
	}
	log.Reset()
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(log.String(), "codex now reads") || strings.Contains(log.String(), "claude now reads") {
		t.Errorf("the full sync should list codex only:\n%s", log.String())
	}
}

// A project synced before targets were recorded as listed shows no list.
func TestSync_AnOlderLedgerShowsNoList(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	mustWriteFile(t, ".agnostic-ai/.sync-state", `{"outputs":["AGENTS.md"]}`)
	log := captureLog(t)

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if strings.Contains(log.String(), "now reads") {
		t.Errorf("an older ledger listed tools:\n%s", log.String())
	}
}

// use on a project whose earlier sync listed claude lists codex once.
func TestUse_ListsAnAddedToolOnce(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	log := captureLog(t)

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}
	if n := strings.Count(log.String(), "codex now reads"); n != 1 {
		t.Errorf("codex listed %d times, want once:\n%s", n, log.String())
	}
	if strings.Contains(log.String(), "claude now reads") {
		t.Errorf("claude was listed again:\n%s", log.String())
	}
}

// A first sync that shows nothing, quiet or JSON, leaves the list for a
// later one.
func TestSync_AQuietOrJSONFirstSyncLeavesTheListForLater(t *testing.T) {
	for name, args := range map[string][]string{"quiet": {"sync", "--quiet"}, "json": {"sync", "--json"}} {
		t.Run(name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			isolateGit(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
			if out, err := runCLI(t, args...); err != nil {
				t.Fatalf("%v: %v\n%s", args, err, out)
			}
			log := captureLog(t)

			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}
			if !strings.Contains(log.String(), "codex now reads") {
				t.Errorf("the later sync did not list codex:\n%s", log.String())
			}
		})
	}
}

// A JSON sync between two plain ones keeps what was listed.
func TestSync_AJSONSyncKeepsTheListedTools(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	for _, args := range [][]string{{"sync"}, {"sync", "--json"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, claude]\n")
	log := captureLog(t)

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(log.String(), "claude now reads") || strings.Contains(log.String(), "codex now reads") {
		t.Errorf("want claude listed and codex not:\n%s", log.String())
	}
}

// A server sync leaves out for one target is not listed as read by it
// (#1666).
func TestSync_FirstRunListLeavesOutAnMCPServerATargetDidNotGet(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, zed]\n")
	mustWriteFile(t, ".agnostic-ai/mcps/gh.yaml", "name: gh\ncommand: gh-mcp\nargs: [--token, \"${TOKEN}\"]\n")
	mustWriteFile(t, ".agnostic-ai/mcps/plain.yaml", "name: plain\ncommand: npx\nargs: [-y, server]\n")
	log := captureLog(t)
	notes := captureNotes(t)

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	assertMCPReads(t, log.String(), 2)
	if !strings.Contains(notes.String(), "server gh reads ${TOKEN} in `args`") {
		t.Errorf("the coverage note no longer names gh:\n%s", notes.String())
	}
}

// use lists a tool sync listed before, after its unchanged notes were
// hidden.
func TestUse_ListLeavesOutAnMCPServerATargetDidNotGet(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, zed]\n")
	mustWriteFile(t, ".agnostic-ai/mcps/gh.yaml", "name: gh\ncommand: gh-mcp\nargs: [--token, \"${TOKEN}\"]\n")
	mustWriteFile(t, ".agnostic-ai/mcps/plain.yaml", "name: plain\ncommand: npx\nargs: [-y, server]\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	log := captureLog(t)

	if out, err := runCLI(t, "use", "zed"); err != nil {
		t.Fatalf("use zed: %v\n%s", err, out)
	}
	if !strings.Contains(log.String(), "zed now reads") {
		t.Fatalf("use did not list zed:\n%s", log.String())
	}
	if !strings.Contains(log.String(), "coverage note unchanged since last sync") {
		t.Fatalf("the sync under use should hide its unchanged note:\n%s", log.String())
	}
	assertMCPReads(t, log.String(), 1)
}

// assertMCPReads checks each MCP line of the list, and that it printed
// lines of them.
func assertMCPReads(t *testing.T, log string, lines int) {
	t.Helper()
	listed := 0
	for line := range strings.Lines(log) {
		switch {
		case strings.Contains(line, "MCP server") && strings.Contains(line, ".zed/"):
			listed++
			if strings.Contains(line, "gh") || !strings.Contains(line, "1 MCP server ") || !strings.Contains(line, "plain") {
				t.Errorf("zed's line should list plain only:\n%s", line)
			}
		case strings.Contains(line, "MCP server") && strings.Contains(line, ".mcp.json"):
			listed++
			if !strings.Contains(line, "2 MCP servers") || !strings.Contains(line, "gh, plain") {
				t.Errorf("claude's line should list gh and plain:\n%s", line)
			}
		}
	}
	if listed != lines {
		t.Errorf("want %d MCP lines, got %d:\n%s", lines, listed, log)
	}
}
