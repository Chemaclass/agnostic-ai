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

func specRefsProject(t *testing.T) string {
	t.Helper()
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex, cursor, copilot, gemini]\n")
	writeAgnosticFile(t, "# Shared\n")
	writeFile(t, filepath.Join(".agnostic-ai", "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Reviews diffs.\n---\n\nReview.\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "commit", "SKILL.md"), "---\nname: commit\ndescription: Writes commits.\n---\n\nCommit.\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "release", "SKILL.md"), "---\nname: release\ndescription: Cuts a release.\n---\n\nFinish with {{$SKILL:commit}}.\n")
	rule := filepath.Join(".agnostic-ai", "rules", "push.md")
	writeFile(t, rule, "---\nname: push\n---\n\nBefore pushing, run {{$AGENT:reviewer}} and {{$SKILL:commit}}. Keep {{AGENT:reviewer}}.\n")
	return rule
}

func TestRender_ReferencesUseEachTargetsInvocationPhrase(t *testing.T) {
	rule := specRefsProject(t)
	for target, want := range map[string]string{
		"claude":  "run the reviewer subagent and /commit.",
		"codex":   "run the reviewer agent and the commit skill.",
		"cursor":  "run the reviewer subagent and /commit.",
		"copilot": "run the reviewer agent and the /commit skill.",
		"gemini":  "run the reviewer agent and the commit skill.",
	} {
		t.Run(target, func(t *testing.T) {
			out, err := runCLI(t, "render", rule, "-t", target)
			if err != nil {
				t.Fatalf("render: %v\n%s", err, out)
			}
			if !strings.Contains(out, "Before pushing, "+want) {
				t.Errorf("want %q in:\n%s", want, out)
			}
			if !strings.Contains(out, "Keep {{AGENT:reviewer}}.") {
				t.Errorf("a reference without the sigil must stay verbatim:\n%s", out)
			}
		})
	}
}

// Codex reads rules only from the shared AGENTS.md, so its skill form
// shows in a skill body.
func TestRender_CodexMentionsASkillWithItsSigil(t *testing.T) {
	specRefsProject(t)

	out, err := runCLI(t, "render", filepath.Join(".agnostic-ai", "skills", "release", "SKILL.md"), "-t", "codex")
	if err != nil {
		t.Fatalf("render: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Finish with $commit.") {
		t.Errorf("want the $commit mention in:\n%s", out)
	}
}

func TestSync_NotesTargetsThatRenderTheNeutralPhrase(t *testing.T) {
	specRefsProject(t)

	var warned bytes.Buffer
	adapters.ResetCoverageNotes()
	adapters.SetWarner(&warned)
	t.Cleanup(func() { adapters.ResetCoverageNotes(); adapters.SetWarner(os.Stderr) })

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	out := warned.String()
	var notes []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "note:") && strings.Contains(line, "{{$") {
			notes = append(notes, line)
		}
	}
	joined := strings.Join(notes, "\n")
	for _, want := range []string{
		"`{{$AGENT:<name>}}` on 1 rule has no effect on codex, gemini",
		"`{{$SKILL:<name>}}` on 1 rule has no effect on codex, gemini",
		"`{{$SKILL:<name>}}` on 1 skill has no effect on gemini",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("want note %q, got:\n%s", want, out)
		}
	}
	if len(notes) != 3 {
		t.Errorf("want one note per keyword and kind, got:\n%s", joined)
	}
}

// Several tools read a nested AGENTS.md, so a scoped rule there takes
// the neutral phrase instead of failing on per-tool text.
func TestSync_ScopedRuleInASharedAGENTSMdUsesTheNeutralPhrase(t *testing.T) {
	testutil.TempCwd(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, cursor, opencode]\n")
	writeAgnosticFile(t, "# Shared\n")
	writeFile(t, filepath.Join("services", "api", "main.go"), "package main\n")
	writeFile(t, filepath.Join(".agnostic-ai", "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Reviews diffs.\n---\n\nReview.\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "commit", "SKILL.md"), "---\nname: commit\ndescription: Writes commits.\n---\n\nCommit.\n")
	writeFile(t, filepath.Join(".agnostic-ai", "rules", "api.md"), "---\nname: api\nscope: services/api\n---\n\nRun {{$AGENT:reviewer}} and {{$SKILL:commit}}.\n")
	writeFile(t, filepath.Join(".agnostic-ai", "reviews", "commits.md"), "---\nname: commits\n---\n\nFlag commits not made with {{$SKILL:commit}}.\n")

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}

	nested := readFile(t, filepath.Join("services", "api", "AGENTS.md"))
	if !strings.Contains(nested, "Run the reviewer agent and the commit skill.") {
		t.Errorf("want the neutral phrase in the nested AGENTS.md:\n%s", nested)
	}
	if root := readFile(t, "AGENTS.md"); !strings.Contains(root, "Flag commits not made with the commit skill.") {
		t.Errorf("want the neutral phrase in the root review section:\n%s", root)
	}
}
