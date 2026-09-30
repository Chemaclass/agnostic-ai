package spec

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFindClaudeSyntax_ReportsEachShapeWithItsBodyLine(t *testing.T) {
	t.Parallel()
	body := "## Context\n\n!`git log main..HEAD --oneline`\n- Diff: !`git diff`\n\nUse $ARGUMENTS as the issue number.\nCompare $0 with $ARGUMENTS[1].\n\n```!\nnode --version\n```\n"
	got := FindClaudeSyntax(body)
	want := []ClaudeSyntaxUse{
		{Syntax: ClaudeShell, Line: 3},
		{Syntax: ClaudeShell, Line: 4},
		{Syntax: ClaudeArguments, Line: 6},
		{Syntax: ClaudePositional, Line: 7},
		{Syntax: ClaudeShell, Line: 9},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestFindClaudeSyntax_SkipsCodeBlocksEscapesAndInlineBang(t *testing.T) {
	t.Parallel()
	body := "```bash\nawk '{print $1}'\necho !`date`\n```\n\nKEY=!`cmd` stays text.\nPrice: \\$1.00, and \\$ARGUMENTS stays.\n"
	if got := FindClaudeSyntax(body); len(got) != 0 {
		t.Errorf("want no uses, got %+v", got)
	}
}

func TestFindClaudeSyntax_ArgumentsCountInsideCodeBlocks(t *testing.T) {
	t.Parallel()
	body := "```sh\ngh issue view $ARGUMENTS\n```\n"
	want := []ClaudeSyntaxUse{{Syntax: ClaudeArguments, Line: 2}}
	if got := FindClaudeSyntax(body); !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFindClaudeSyntax_RecordsTheTargetFenceAUseSitsIn(t *testing.T) {
	t.Parallel()
	body := "::target claude\n!`git status`\n::end\n::target codex\nRun `git status` first.\n::end\nUse $ARGUMENTS.\n"
	got := FindClaudeSyntax(body)
	if len(got) != 2 {
		t.Fatalf("want 2 uses, got %+v", got)
	}
	if got[0].ReachesTarget("codex") || !got[0].ReachesTarget("claude") {
		t.Errorf("fenced use reaches the wrong targets: %+v", got[0])
	}
	if !got[1].ReachesTarget("codex") {
		t.Errorf("unfenced use must reach every target: %+v", got[1])
	}
}

func TestLoad_BodyLinePointsAtTheFileLineWhereTheBodyStarts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pr.md")
	if err := os.WriteFile(path, []byte("---\nname: pr\n---\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e, err := parseMarkdown(path)
	if err != nil {
		t.Fatal(err)
	}
	if e.BodyLine != 5 {
		t.Errorf("BodyLine = %d, want 5", e.BodyLine)
	}
	if got := e.BodyLocation(1); got != path+":5" {
		t.Errorf("BodyLocation(1) = %q", got)
	}
}

func TestBodyLocation_FallsBackToThePathWhenTheBodyWasRewritten(t *testing.T) {
	t.Parallel()
	e := Entry{Path: "skills/pr.md", Body: "::target codex\nx\n::end\n$ARGUMENTS\n", BodyLine: 4}
	filtered := filterEntriesFor([]Entry{e}, "claude")[0]
	if filtered.BodyLine != 0 {
		t.Errorf("a fence-filtered body keeps BodyLine %d", filtered.BodyLine)
	}
	if got := filtered.BodyLocation(1); got != "skills/pr.md" {
		t.Errorf("BodyLocation = %q", got)
	}
}
