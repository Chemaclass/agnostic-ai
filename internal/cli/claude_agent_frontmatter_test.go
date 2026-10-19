package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Claude Code treats an agent file with no `name` as documentation and
// skips one with a name but no `description`, without saying so.
func TestSync_ClaudeAgentAlwaysGetsANameAndADescription(t *testing.T) {
	dir := testutil.TempCwd(t)
	silence(t)
	notes := captureNotes(t)
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "bare.md"), "Review the diff.\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "named.md"), "---\nname: named\n---\n\nReview the diff.\n")
	mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "full.md"), "---\nname: full\ndescription: Reviews a diff.\n---\n\nReview the diff.\n")
	mustSync(t)

	for name, want := range map[string]string{
		"bare":  "---\nname: bare\ndescription: bare\n---\n",
		"named": "---\nname: named\ndescription: named\n---\n",
		"full":  "---\nname: full\ndescription: Reviews a diff.\n---\n",
	} {
		got, err := os.ReadFile(filepath.Join(dir, ".claude", "agents", name+".md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(got), want) {
			t.Errorf("%s.md starts:\n%s\nwant:\n%s", name, got, want)
		}
	}
	want := "claude: 2 agents have no description, so sync wrote each name as the description (bare, named); Claude Code skips an agent without one, so write when to use it"
	if !strings.Contains(notes.String(), want) {
		t.Errorf("notes:\n%s\nwant:\n%s", notes, want)
	}
}

// Claude Code refuses to launch an agent whose tool list resolves to no
// tool, so an empty list is never written; the agent is left out with a
// note instead.
func TestSync_ClaudeLeavesOutAnAgentWithNoUsableTool(t *testing.T) {
	for _, can := range []string{"can: []", "can: [delete]", "tools: []", "tools: Delete", `tools: ""`, "x-claude:\n  tools: \"\""} {
		t.Run(can, func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			notes := captureNotes(t)
			mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude, cursor]\n")
			mustWrite(t, filepath.Join(dir, ".agnostic-ai", "agents", "idle.md"), "---\ndescription: Thinks.\n"+can+"\n---\n\nThink.\n")
			mustSync(t)

			if _, err := os.Stat(filepath.Join(dir, ".claude", "agents", "idle.md")); !os.IsNotExist(err) {
				t.Errorf("an agent with no usable tool reached Claude Code: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, ".cursor", "agents", "idle.md")); err != nil {
				t.Errorf("other targets still get the agent: %v", err)
			}
			want := "1 agent reaches claude only in the source dir (agent idle has an empty tool list, such as `can: []` or `can: [delete]`, and Claude Code refuses to launch it; list a tool it may use, or set `target:` to keep it off Claude Code)"
			if !strings.Contains(notes.String(), want) {
				t.Errorf("notes:\n%s\nwant:\n%s", notes, want)
			}
			if !strings.Contains(notes.String(), "`can: delete` on 1 agent has no effect on claude, cursor (delete has no native tool or rule, so sync leaves it out of the list)") && can == "can: [delete]" || strings.Contains(notes.String(), "`can` on 1 agent") {
				t.Errorf("the note must name the delete entry, not the whole can: list, which restricts the agent:\n%s", notes)
			}
		})
	}
}

func TestLintAgentDescriptions_WarnsOnAnAgentWithNone(t *testing.T) {
	entries := []spec.Entry{
		{Kind: spec.KindAgent, Name: "bare", Path: "agents/bare.md", Body: "b"},
		{Kind: spec.KindAgent, Name: "blank", Path: "agents/blank.md", Meta: map[string]any{"description": "  "}, Body: "b"},
		{Kind: spec.KindAgent, Name: "full", Path: "agents/full.md", Meta: map[string]any{"description": "d"}, Body: "b"},
		{Kind: spec.KindAgent, Name: "empty", Path: "agents/empty.md"},
		{Kind: spec.KindSkill, Name: "s", Path: "skills/s/SKILL.md", Body: "b"},
	}

	got := lintAgentDescriptions(entries, []string{"claude"})

	if len(got) != 2 || got[0].Path != "agents/bare.md" || got[1].Path != "agents/blank.md" {
		t.Fatalf("findings = %v", got)
	}
	if got[0].Code != "LINT048" || got[0].Severity != lintWarn || !strings.Contains(got[0].Message, "decide when to hand it a task") {
		t.Errorf("finding = %+v", got[0])
	}
}

func TestLintNearMissKeys_ClaudeAgentFields(t *testing.T) {
	cases := map[string]string{
		"maxturns":            "maxTurns",
		"max_turns":           "maxTurns",
		"permission_mode":     "permissionMode",
		"omit_claude_md":      "omitClaudeMd",
		"disallowed_tools":    "disallowedTools",
		"auto_compact_window": "autoCompactWindow",
		"initial_prompt":      "initialPrompt",
	}
	for key, want := range cases {
		entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a", Path: "agents/a.md", Meta: map[string]any{key: "x"}, Body: "b"}}

		got := lintNearMissKeys(entries, []string{"claude"})

		if len(got) != 1 || !strings.Contains(got[0].Message, "not a field Claude Code reads") || !strings.Contains(got[0].Message, "Use `"+want+":` instead.") || strings.Contains(got[0].Message, "x-junie") {
			t.Errorf("%s: want one finding naming %s, got %v", key, want, got)
		}
	}
}

func TestLintNearMissKeys_ClaudeAgentFieldsSpelledRightStaySilent(t *testing.T) {
	meta := map[string]any{}
	for _, k := range claudeNativeAgentFields {
		meta[k] = "x"
	}
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a", Path: "agents/a.md", Meta: meta, Body: "b"}}
	if got := lintNearMissKeys(entries, []string{"claude"}); len(got) != 0 {
		t.Errorf("Claude Code's own agent fields must not be flagged, got %v", got)
	}
}

func TestLintNearMissKeys_JunieAdviceStaysWithoutClaude(t *testing.T) {
	entries := []spec.Entry{{Kind: spec.KindAgent, Name: "a", Path: "agents/a.md", Meta: map[string]any{"max_turns": "x"}, Body: "b"}}
	got := lintNearMissKeys(entries, []string{"junie"})
	if len(got) != 1 || !strings.Contains(got[0].Message, "`x-junie.maxTurns:`") {
		t.Errorf("got %v", got)
	}
	if got := lintNearMissKeys(entries, []string{"cursor"}); len(got) != 1 {
		t.Errorf("a near miss still warns with neither target, got %v", got)
	}
}
