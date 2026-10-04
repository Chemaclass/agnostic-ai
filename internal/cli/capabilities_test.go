package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// syncedAgentTree syncs one agent spec with frontmatter field to every
// target and returns the files sync wrote, without the sources.
func syncedAgentTree(t *testing.T, field string) map[string]string {
	t.Helper()
	dir := testutil.TempCwd(t)
	silence(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+strings.Join(adapters.Names(), ", ")+"]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"),
		"---\nname: reviewer\ndescription: Reviews a diff.\n"+field+"\n---\n\nReview the diff.\n")
	mustSync(t)
	tree := snapshotTree(t, dir)
	for rel := range tree {
		if strings.HasPrefix(rel, ".agnostic-ai"+string(filepath.Separator)) {
			delete(tree, rel)
		}
	}
	return tree
}

// An agent's can: syncs to every target exactly as the tools: it stands
// for, so a spec can switch forms without a file changing.
func TestSync_CapabilitiesWriteTheSameFilesAsTheClaudeNames(t *testing.T) {
	cases := []struct{ can, tools string }{
		{"can: [read, shell(git diff *)]", "tools: [Read, Bash(git diff *)]"},
		{"can: [read, edit, write, shell, web, Grep, mcp:github, mcp:github/get_issue]",
			"tools: [Read, Edit, Write, Bash, WebFetch, WebSearch, Grep, mcp__github, mcp__github__get_issue]"},
		{"can:\n  - read\n  - 'shell(go test ./...)'", "tools:\n  - Read\n  - 'Bash(go test ./...)'"},
		{"can: []", "tools: []"},
	}
	for _, tc := range cases {
		t.Run(tc.can, func(t *testing.T) {
			neutral := syncedAgentTree(t, tc.can)
			claude := syncedAgentTree(t, tc.tools)
			if len(neutral) == 0 {
				t.Fatal("sync wrote nothing")
			}
			for rel, want := range claude {
				if got, ok := neutral[rel]; !ok || got != want {
					t.Errorf("%s differs:\n%s\nwant:\n%s", rel, got, want)
				}
			}
			for rel := range neutral {
				if _, ok := claude[rel]; !ok {
					t.Errorf("can: wrote %s, which tools: does not", rel)
				}
			}
		})
	}
}

func TestSync_KiroNotesTheAccessACategoryAdds(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	buf := captureNotes(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "writer.md"), "---\nname: writer\ndescription: Writes docs.\ncan: [read, edit]\n---\n\nWrite docs.\n")
	mustSync(t)
	if want := "kiro: .agnostic-ai/agents/writer.md: edit becomes Kiro's write category, which also allows delete_file"; !strings.Contains(buf.String(), want) {
		t.Errorf("sync output misses %q:\n%s", want, buf.String())
	}
}

func TestSync_StopsOnACapabilityItCannotRead(t *testing.T) {
	cases := []struct{ field, want string }{
		{"can: [raed]", `unknown capability "raed" for can: (did you mean read?)`},
		{"can: [read]\ntools: [Read]", "sets both can: and tools:; keep one"},
		{"can: read", "can: must be a list"},
		{"can: [shell()]", `can: "shell()" needs a command pattern`},
		{"can: [read(src/**)]", `can: "read(src/**)": only shell takes a pattern`},
		{"can: [mcp:git hub]", `can: "mcp:git hub" needs an MCP server and tool name`},
		{"x-claude:\n  can: [read]", "x-claude.can is not read"},
	}
	for _, tc := range cases {
		t.Run(tc.field, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
			mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "a.md"), "---\nname: a\ndescription: A.\n"+tc.field+"\n---\n\nBody.\n")
			out, err := runCLI(t, "sync", "--gitignore=off")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("sync: %v\n%s", err, out)
			}
			if exists(filepath.Join(dir, ".claude", "agents", "a.md")) {
				t.Error("sync must not write an agent whose can: it cannot read")
			}
			if out, err := runCLI(t, "lint"); err == nil || !strings.Contains(out, "LINT036 [error] .agnostic-ai/agents/a.md: ") {
				t.Errorf("lint: %v\n%s", err, out)
			}
			if out, err := runCLI(t, "validate"); err == nil || !strings.Contains(out, tc.want) {
				t.Errorf("validate: %v\n%s", err, out)
			}
		})
	}
}

func TestMigrate_CapabilitiesRewritesToolsInPlaceAndKeepsAliases(t *testing.T) {
	dir := migrationFixture(t, "capabilities-agent-tools")
	silence(t)

	out, err := runCLI(t, "migrate", "--only", "capabilities", "--dry-run")
	if err != nil {
		t.Fatalf("migrate --dry-run: %v\n%s", err, out)
	}
	for _, want := range []string{
		"would rewrite .agnostic-ai/agents/reviewer.md",
		"would rewrite .agnostic-ai/agents/writer.md",
		"skipped .agnostic-ai/agents/explorer.md: keeps tools: as written: no entry has a capability of its own",
		"skipped .agnostic-ai/agents/reviewer.md: keeps Grep as an alias: no capability stands for it alone",
		"skipped .agnostic-ai/agents/writer.md: keeps WebFetch as an alias: web also grants WebSearch",
		"skipped .agnostic-ai/agents/reviewer.md: keeps WebFetch and WebSearch as aliases: web stands for them together; write it in their place by hand",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("dry run misses %q:\n%s", want, out)
		}
	}
	if out, err := runCLI(t, "migrate", "--only", "capabilities"); err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	for name, want := range map[string]string{
		"reviewer.md": "---\n# Keep this comment: migrations must not reformat frontmatter.\nname: reviewer\ndescription: Reviews a diff.\n" +
			"can: [read, Grep, shell(git diff *), mcp:github, mcp:github/get_issue, WebFetch, WebSearch]   # least privilege\n---\n\nReview the diff and report findings with `file:line`.\n",
		"writer.md":   "---\nname: writer\ndescription: Writes docs.\ncan:\n  - read     # look first\n  - 'edit'\n  - \"write\"\n  - WebFetch\n  - shell\n---\n\nWrite the docs page the task asks for.\n",
		"explorer.md": "---\nname: explorer\ndescription: Maps the codebase.\ntools: [Grep, Glob]\n---\n\nList the files that matter for the task.\n",
	} {
		got, err := os.ReadFile(filepath.Join(dir, ".agnostic-ai", "agents", name))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != want {
			t.Errorf("%s =\n%s\nwant:\n%s", name, got, want)
		}
	}
}

func TestMigrate_CapabilitiesSkipsBothFormsAndLocalExtensions(t *testing.T) {
	dir := migrationFixture(t, "capabilities-agent-tools")
	silence(t)
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "local", "agents", "writer.md"), "---\nmodel: opus\n---\n")
	both := filepath.Join(dir, ".agnostic-ai", "agents", "both.md")
	mustWrite(t, both, "---\nname: both\ndescription: Both.\ntools: [Read]\ncan: [read]\n---\n\nBody.\n")

	out, err := runCLI(t, "migrate", "--only", "capabilities")
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	for _, want := range []string{
		"skipped .agnostic-ai/agents/both.md: sets both tools: and can:; keep one by hand",
		"a local/ spec extends this agent; rewrite both files by hand",
		"rewrote .agnostic-ai/agents/reviewer.md",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "rewrote .agnostic-ai/agents/writer.md") {
		t.Errorf("an extended agent must stay as written:\n%s", out)
	}
}
