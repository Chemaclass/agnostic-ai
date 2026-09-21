package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// captureNotes routes the adapters warner into a buffer and clears the
// coverage-note buffer on both ends, so a test only sees its own notes.
func captureNotes(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	adapters.ResetCoverageNotes()
	adapters.SetWarner(buf)
	t.Cleanup(func() {
		adapters.ResetCoverageNotes()
		adapters.SetWarner(os.Stderr)
	})
	return buf
}

// Devin reads `.agents/agents/` as well as its own `.devin/agents/`,
// and the targets owning that tree write an agent at a different path
// inside it, so the content collision check never sees the overlap. One
// spec becomes two profiles claiming the same name, and only the
// `.devin/` copy carries allowed-tools (#863).
func TestWarnSharedAgentsTreeReaders(t *testing.T) {
	cases := []struct {
		name    string
		owners  map[string][]string
		targets []string
		want    string
	}{
		{
			name: "antigravity nests a profile Devin also reads",
			owners: map[string][]string{
				".agents/agents/reviewer/agent.md": {"antigravity"},
				".devin/agents/reviewer.md":        {"windsurf"},
			},
			targets: []string{"antigravity", "windsurf"},
			want:    "1 agent file in .agents/agents/ written by antigravity",
		},
		{
			name: "goose and openhands write a flat one",
			owners: map[string][]string{
				".agents/agents/reviewer.md": {"goose", "openhands"},
				".devin/agents/reviewer.md":  {"windsurf"},
			},
			targets: []string{"goose", "openhands", "windsurf"},
			want:    "written by goose, openhands",
		},
		{
			name: "no windsurf, no overlap to report",
			owners: map[string][]string{
				".agents/agents/reviewer.md": {"goose"},
			},
			targets: []string{"goose"},
			want:    "",
		},
		{
			name: "windsurf alone owns nothing in that tree",
			owners: map[string][]string{
				".devin/agents/reviewer.md": {"windsurf"},
			},
			targets: []string{"windsurf"},
			want:    "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			summary := captureSummary(t)
			notes := captureNotes(t)
			warnSharedAgentsTreeReaders(c.owners, c.targets)
			if summary.Len() != 0 {
				t.Errorf("note bypassed the coverage-note buffer: %q", summary.String())
			}
			adapters.FlushCoverageNotes()
			out := notes.String()
			if c.want == "" {
				if out != "" {
					t.Errorf("expected no note, got %q", out)
				}
				return
			}
			if !strings.Contains(out, c.want) {
				t.Errorf("note missing %q:\n%s", c.want, out)
			}
			if !strings.Contains(out, "allowed-tools") {
				t.Errorf("note does not say what is actually lost:\n%s", out)
			}
		})
	}
}

// Many agents in the shared tree fold into one line instead of one line
// per emitted file.
func TestWarnSharedAgentsTreeReaders_OneLineForManyAgents(t *testing.T) {
	captureSummary(t)
	notes := captureNotes(t)
	warnSharedAgentsTreeReaders(map[string][]string{
		".agents/agents/a.md":       {"goose", "openhands"},
		".agents/agents/a/agent.md": {"antigravity"},
		".agents/agents/b.md":       {"goose", "openhands"},
		".agents/agents/b/agent.md": {"antigravity"},
		".devin/agents/a.md":        {"windsurf"},
		".devin/agents/b.md":        {"windsurf"},
	}, []string{"antigravity", "goose", "openhands", "windsurf"})
	adapters.FlushCoverageNotes()

	out := notes.String()
	if n := strings.Count(out, "\n"); n != 1 {
		t.Fatalf("want one note line, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "4 agent files in .agents/agents/ written by antigravity, goose, openhands") {
		t.Errorf("note does not aggregate files and writers:\n%s", out)
	}
}

// A second sync with the same specs must honor the same
// unchanged-since-last-sync suppression every other note gets, instead
// of re-printing the shared-tree note on every run.
func TestRunSyncOnce_SharedAgentsTreeNoteSuppressedWhenUnchanged(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets: [goose, windsurf]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Reviews code.\n---\n\nBody.\n")
	silence(t)
	log := captureLog(t)
	notes := captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notes.String(), "also read by Devin") {
		t.Fatalf("first sync did not report the shared tree:\n%s", notes.String())
	}

	notes.Reset()
	log.Reset()
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(notes.String()+log.String(), "also read by Devin") {
		t.Errorf("unchanged note re-printed on second sync:\n%s%s", log.String(), notes.String())
	}
	if !strings.Contains(log.String(), "unchanged since last sync") {
		t.Errorf("second sync did not count the suppressed note:\n%s", log.String())
	}
}
