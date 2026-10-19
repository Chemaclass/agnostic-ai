package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// claudeScopedProject syncs one agent with field to Claude Code and
// returns the agent file and the notes sync printed.
func claudeScopedProject(t *testing.T, config, field string) (string, string, error) {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	notes := captureNotes(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n"+config)
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Reviews a diff.\n"+field+"\n---\n\nReview the diff.\n")
	out, err := runCLI(t, "sync", "--gitignore=off")
	if err != nil {
		return "", notes.String() + out, err
	}
	got, readErr := os.ReadFile(filepath.Join(dir, ".claude", "agents", "reviewer.md"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(got), notes.String() + out, nil
}

// Claude Code reads an agent's `tools` entry by its tool name alone: a
// pattern neither limits nor pre-approves what the agent runs (live
// check on 2.1.303, #2325). Sync writes the tool it grants and says so.
func TestSync_ClaudeWritesThePlainToolForAScopedCapability(t *testing.T) {
	cases := []struct{ field, tools, note string }{
		{"can: [read, shell(git diff *)]", "tools:\n  - Read\n  - Bash\n", "shell(git diff *) becomes Bash"},
		{"can: [read(src/**), edit(src/**)]", "tools:\n  - Read\n  - Edit\n", "read(src/**) becomes Read"},
		{"can: [shell(git diff *), shell(git log *)]", "tools:\n  - Bash\n", "shell(git log *) becomes Bash"},
		{"tools: [Read, Bash(git diff *)]", "tools:\n  - Read\n  - Bash\n", "Bash(git diff *) becomes Bash"},
		{"tools: Read, Bash(git diff *)", "tools: Read, Bash\n", "Bash(git diff *) becomes Bash"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			got, notes, err := claudeScopedProject(t, "", tc.field)
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, notes)
			}
			if !strings.Contains(got, tc.tools) {
				t.Errorf("agent file:\n%s\nwant %q", got, tc.tools)
			}
			want := "claude: " + filepath.Join(".agnostic-ai", "agents", "reviewer.md") + ": " + tc.note + "; Claude Code does not limit an agent's tools to a pattern"
			if !strings.Contains(notes, want) {
				t.Errorf("notes:\n%s\nwant %q", notes, want)
			}
		})
	}
}

func TestSync_ClaudeScopedCapabilityFailsUnderOnUnsupportedError(t *testing.T) {
	_, out, err := claudeScopedProject(t, "on-unsupported: error\n", "can: [read, shell(git diff *)]")
	if err == nil || !strings.Contains(err.Error(), "shell(git diff *) becomes Bash; Claude Code does not limit an agent's tools to a pattern") {
		t.Fatalf("sync = %v\n%s", err, out)
	}
}

// Names Claude Code does read in full stay as written, without a note:
// one MCP tool, a whole server, the Agent(type) allowlist, and the
// agent's own x-claude.tools.
func TestSync_ClaudeKeepsToolNamesItReadsInFull(t *testing.T) {
	cases := []struct{ field, tools string }{
		{"can: [mcp:github, mcp:github/get_issue]", "tools:\n  - mcp__github\n  - mcp__github__get_issue\n"},
		{"tools:\n  - 'Agent(worker, researcher)'\n  - Read", "tools:\n  - Agent(worker, researcher)\n  - Read\n"},
		{"x-claude:\n  tools: [Bash(git diff *)]", "tools:\n  - Bash(git diff *)\n"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			got, notes, err := claudeScopedProject(t, "on-unsupported: error\n", tc.field)
			if err != nil {
				t.Fatalf("sync: %v\n%s", err, notes)
			}
			if !strings.Contains(got, tc.tools) {
				t.Errorf("agent file:\n%s\nwant %q", got, tc.tools)
			}
			if strings.Contains(notes, "does not limit") {
				t.Errorf("unexpected widening note:\n%s", notes)
			}
		})
	}
}

// A skill's allowed-tools pattern is Claude Code's documented
// pre-approval form, so it stays scoped.
func TestSync_ClaudeKeepsASkillsScopedAllowedTools(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	notes := captureNotes(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\non-unsupported: error\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "skills", "diff", "SKILL.md"), "---\nname: diff\ndescription: Shows the diff.\nallowed-tools: [read, shell(git diff *)]\n---\n\nShow the diff.\n")
	mustSync(t)
	got, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "diff", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "Bash(git diff *)") || strings.Contains(notes.String(), "does not limit") {
		t.Errorf("skill:\n%s\nnotes:\n%s", got, notes)
	}
}

// Importing the file sync wrote must not drop the pattern from the spec:
// other tools may still honor it.
func TestImportFromClaude_KeepsTheScopedCapabilityOfAnUneditedAgent(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	source := "---\nname: reviewer\ndescription: Reviews a diff.\ncan: [read, shell(git diff *)]\n---\nReview the diff.\n"
	writeFile(t, ".agnostic-ai/agents/reviewer.md", source)
	execCLI(t, "sync")
	execCLI(t, "import", "claude")
	if got := readFile(t, ".agnostic-ai/agents/reviewer.md"); !strings.Contains(got, "shell(git diff *)") {
		t.Errorf("import dropped the pattern:\n%s", got)
	}

	native := readFile(t, ".claude/agents/reviewer.md")
	writeFile(t, ".claude/agents/reviewer.md", strings.Replace(native, "Review the diff.", "Review the diff twice.", 1))
	execCLI(t, "import", "claude")
	got := readFile(t, ".agnostic-ai/agents/reviewer.md")
	if !strings.Contains(got, "shell(git diff *)") || !strings.Contains(got, "Review the diff twice.") {
		t.Errorf("an edited agent lost the pattern or the edit:\n%s", got)
	}

	native = readFile(t, ".claude/agents/reviewer.md")
	writeFile(t, ".claude/agents/reviewer.md", strings.Replace(native, "  - Bash\n", "", 1))
	execCLI(t, "import", "claude")
	if got := readFile(t, ".agnostic-ai/agents/reviewer.md"); strings.Contains(got, "shell") {
		t.Errorf("removing Bash natively must reach the spec:\n%s", got)
	}
}
