package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
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

// Sync names the target once when on-unsupported: error stops on a
// widening, for every adapter that reports one.
func TestSync_WideningErrorNamesTheTargetOnce(t *testing.T) {
	for target, can := range map[string]string{"claude": "can: [read, shell(git diff *)]", "kiro": "can: [edit]"} {
		t.Run(target, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\non-unsupported: error\n")
			mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Reviews a diff.\n"+can+"\n---\n\nReview.\n")
			_, err := runCLI(t, "sync", "--gitignore=off")
			want := target + ": " + filepath.Join(".agnostic-ai", "agents", "reviewer.md") + ": "
			if err == nil || !strings.HasPrefix(err.Error(), want) {
				t.Fatalf("error = %v, want it to start %q", err, want)
			}
		})
	}
}

// sync --global honors on-unsupported for the widening, as project sync
// does.
func TestSyncGlobal_ClaudeScopedCapabilityHonorsOnUnsupported(t *testing.T) {
	for _, mode := range []string{"warn", "silent", "error"} {
		t.Run(mode, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			notes := captureNotes(t)
			mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "on-unsupported: "+mode+"\n")
			mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Reviews a diff.\ncan: [read, shell(git diff *)]\n---\nReview.\n")
			_, warnings, err := runGlobalAgentTest("--only", "claude")
			const line = "shell(git diff *) becomes Bash; Claude Code does not limit an agent's tools to a pattern"
			if mode == "error" {
				if err == nil || !strings.Contains(err.Error(), "claude: ") || !strings.Contains(err.Error(), line) {
					t.Fatalf("error = %v", err)
				}
				if _, statErr := os.Stat(filepath.Join(home, ".claude", "agents", "reviewer.md")); !os.IsNotExist(statErr) {
					t.Errorf("agent written despite the error: %v", statErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("sync --global: %v\n%s", err, warnings)
			}
			if got := strings.Contains(notes.String()+warnings, line); got != (mode == "warn") {
				t.Errorf("%s: note shown = %v\n%s%s", mode, got, notes, warnings)
			}
		})
	}
}

// Sync drops delete for Claude Code, so import must compare against the
// same list.
func TestImportFromClaude_KeepsTheScopedCapabilityBesideDelete(t *testing.T) {
	dir := setupFixture(t)
	testutil.Chdir(t, dir)
	silence(t)
	logs := captureLogOut(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, ".agnostic-ai/agents/reviewer.md", "---\nname: reviewer\ndescription: Reviews a diff.\ncan: [read, delete, shell(git diff *)]\n---\nReview the diff.\n")
	execCLI(t, "sync")
	native := readFile(t, ".claude/agents/reviewer.md")
	writeFile(t, ".claude/agents/reviewer.md", strings.Replace(native, "Review the diff.", "Review the diff twice.", 1))
	execCLI(t, "import", "claude")

	got := readFile(t, ".agnostic-ai/agents/reviewer.md")
	if !strings.Contains(got, "shell(git diff *)") || !strings.Contains(got, "delete") {
		t.Errorf("import lost the spec's capabilities:\n%s", got)
	}
	want := "note: " + filepath.Join(".agnostic-ai", "agents", "reviewer.md") + " keeps its scoped can:; Claude Code gets the plain tools. To widen it for every tool, edit the spec"
	if !strings.Contains(logs.String(), want) {
		t.Errorf("import must say it kept the scoped spec:\n%s\nwant %q", logs, want)
	}
}

func TestExplainCapabilities_ClaudeAgentShowsThePlainTool(t *testing.T) {
	entry := spec.Entry{Kind: spec.KindAgent, Meta: map[string]any{"can": []any{"read", "shell(git diff *)"}}}
	got := explainCapabilities(entry, &config.Config{Targets: []string{"claude"}})
	var out bytes.Buffer
	printExplainCapabilities(&out, got)
	for _, want := range []string{"[claude] can: shell(git diff *) → Bash", "shell(git diff *) becomes Bash; Claude Code does not limit an agent's tools to a pattern"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "→ Bash(git diff *)") {
		t.Errorf("explain shows a pattern sync no longer writes:\n%s", out.String())
	}
}
