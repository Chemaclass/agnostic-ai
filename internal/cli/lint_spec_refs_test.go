package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLint_FailsOnAReferenceToAnUnknownAgent(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Reviews diffs.\n---\n\nReview.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "agents", "planner.md"), "---\nname: planner\ndescription: Plans work.\n---\n\nPlan.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "push.md"),
		"---\nname: push\n---\n\nBefore pushing, run {{$AGENT:reviewer}}, then {{$AGENT:typo}} and {{$SKILL:commit}}.\n")

	out, err := runCLI(t, "lint")
	if err == nil {
		t.Fatalf("LINT033 is an error, lint passed:\n%s", out)
	}
	lines := strings.Join(findingLines(out, "LINT033"), "\n")
	for _, want := range []string{
		`{{$AGENT:typo}} names no agent "typo"; known agents: planner, reviewer`,
		`{{$SKILL:commit}} names no skill "commit"; known skills: none`,
	} {
		if !strings.Contains(lines, want) {
			t.Errorf("LINT033 lacks %q:\n%s", want, out)
		}
	}
	if strings.Contains(lines, "AGENT:reviewer") {
		t.Errorf("a known agent must not be flagged:\n%s", out)
	}
}
