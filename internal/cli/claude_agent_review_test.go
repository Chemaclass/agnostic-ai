package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The description sync falls back to is not the user's, so importing an
// edited native file must not copy it into the spec.
func TestImportFromClaude_KeepsTheIdentityFallbackOutOfTheSpec(t *testing.T) {
	for name, source := range map[string]string{
		"model only":     "---\nmodel: sonnet\n---\nReview.\n",
		"no frontmatter": "Review.\n",
	} {
		t.Run(name, func(t *testing.T) {
			dir := setupFixture(t)
			testutil.Chdir(t, dir)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
			writeFile(t, ".agnostic-ai/agents/m.md", source)
			execCLI(t, "sync")
			native := readFile(t, ".claude/agents/m.md")
			writeFile(t, ".claude/agents/m.md", strings.Replace(native, "Review.", "Review twice.", 1))
			execCLI(t, "import", "claude")

			got := readFile(t, ".agnostic-ai/agents/m.md")
			if strings.Contains(got, "description:") {
				t.Errorf("sync's fallback description reached the spec:\n%s", got)
			}
			if !strings.Contains(got, "Review twice.") {
				t.Errorf("the native edit was lost:\n%s", got)
			}
		})
	}
}

// A user-written name or description equal to the fallback stays.
func TestImportFromClaude_KeepsAnIdentityTheSpecDeclares(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".agnostic-ai/agents/m.md", "---\nname: m\ndescription: m\n---\nReview.\n")
	execCLI(t, "sync")
	writeFile(t, ".claude/agents/m.md", "---\nname: m\ndescription: m\n---\nReview twice.\n")
	execCLI(t, "import", "claude")

	if got := readFile(t, ".agnostic-ai/agents/m.md"); !strings.Contains(got, "name: m\n") || !strings.Contains(got, "description: m\n") {
		t.Errorf("declared identity lost:\n%s", got)
	}
}

// Gemini CLI reads `max_turns` and `mcp_servers` at the top level, so
// they are not Claude near misses while gemini is a target.
func TestLintNearMissKeys_LeavesKeysAnotherTargetReads(t *testing.T) {
	for _, key := range []string{"max_turns", "mcp_servers", "timeout_mins"} {
		entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a", Path: "agents/a.md", Meta: map[string]any{key: "x"}, Body: "b"}}
		if got := lintNearMissKeys(entries, []string{"claude", "gemini"}); len(got) != 0 {
			t.Errorf("%s: gemini reads it, got %v", key, got)
		}
	}
}

func TestLintNearMissKeys_SkipsTheClaudeCheckForAnAgentClaudeNeverGets(t *testing.T) {
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a", Path: "agents/a.md", Meta: map[string]any{"maxturns": "x", "target": "cursor"}, Body: "b"}}
	for _, f := range lintNearMissKeys(entries, []string{"claude", "cursor"}) {
		if strings.Contains(f.Message, "Claude Code") {
			t.Errorf("agent never reaches Claude Code: %v", f)
		}
	}
}

func TestLintAgentDescriptions_NamesClaudeOnlyWhenTheAgentGoesThere(t *testing.T) {
	bare := spec.Entry{Kind: spec.KindAgent, Name: "bare", Path: "agents/bare.md", Body: "b"}
	elsewhere := bare
	elsewhere.Meta = map[string]any{"target": "gemini"}
	for name, c := range map[string]struct {
		entry   spec.Entry
		targets []string
		claude  bool
	}{
		"claude target":   {bare, []string{"claude"}, true},
		"cursor only":     {bare, []string{"cursor"}, false},
		"agent elsewhere": {elsewhere, []string{"claude", "gemini"}, false},
	} {
		got := lintAgentDescriptions([]spec.Entry{c.entry}, c.targets)
		if len(got) != 1 {
			t.Fatalf("%s: findings = %v", name, got)
		}
		if strings.Contains(got[0].Message, "Claude Code") != c.claude {
			t.Errorf("%s: message %q", name, got[0].Message)
		}
	}
}

// An agent sync leaves out of Claude Code does not hide a global agent
// of the same name there.
func TestSync_NoNameClashForAnAgentClaudeLeavesOut(t *testing.T) {
	globalNameClashProject(t, "claude",
		map[string]string{"agents/idle.md": agentSpec("idle")},
		map[string]string{"agents/idle.md": "---\nname: idle\ndescription: Thinks.\ncan: []\n---\nThink.\n"})
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "also exists in") {
		t.Errorf("claude never loads the project agent, so nothing clashes:\n%s", buf.String())
	}
}

// A hand-written native file for an agent sync leaves out is not
// adoptable: import already has its spec, so doctor must not loop on it.
func TestDoctor_ExplainsAHandWrittenAgentSyncLeavesOut(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".claude", "agents", "idle.md"), "---\nname: idle\ndescription: Thinks.\ntools: []\n---\nThink.\n")
	execCLI(t, "import", "claude")
	mustSync(t)
	if _, err := os.Stat(filepath.Join(dir, ".claude", "agents", "idle.md")); err != nil {
		t.Fatalf("sync must not delete a file it never wrote: %v", err)
	}

	out, _ := runCLI(t, "doctor")

	if strings.Contains(out, "✗ .claude/agents/idle.md") {
		t.Errorf("doctor offers an import that changes nothing:\n%s", out)
	}
	if !strings.Contains(out, ".claude/agents/idle.md: sync leaves agent idle out of Claude Code because its tool list is empty; give it a tool, or delete the file") {
		t.Errorf("doctor must say why the file is not managed:\n%s", out)
	}
}

func TestCompare_GivesTheReasonForDeleteAndALeftOutAgent(t *testing.T) {
	dir := setupCompareFixture(t)
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "cleaner.md"), "---\nname: cleaner\ndescription: Cleans.\ncan: [read, delete]\n---\nClean.\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "idle.md"), "---\nname: idle\ndescription: Thinks.\ncan: []\n---\nThink.\n")
	testutil.Chdir(t, dir)
	silence(t)
	out := compareJSON(t, "claude", "cursor")

	if r := findCompareResult(t, out, ".agnostic-ai/agents/cleaner.md", "can", "claude"); !strings.Contains(r.Reason, "delete") {
		t.Errorf("claude can: reason %q must name delete", r.Reason)
	}
	for _, s := range out.Specs {
		if s.Path != ".agnostic-ai/agents/idle.md" {
			continue
		}
		for _, f := range s.Fields {
			for _, r := range f.Results {
				if r.Target == "claude" && (r.Status == statusUnknown || !strings.Contains(r.Reason, "empty tool list")) {
					t.Errorf("claude %s: %+v", f.Field, r)
				}
			}
		}
	}
}
