package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func seedDoubleStarProject(t *testing.T, unmanaged string) {
	t.Helper()
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [cursor]\ngitignore:\n  enabled: false\nsync:\n  unmanaged:\n    - "+unmanaged+"\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "AGNOSTIC_AI.md"), "Root\n")
	silence(t)
}

func TestLint_WarnsOnDoubleStarUnmanagedEntry(t *testing.T) {
	seedDoubleStarProject(t, `"**/.cursor/BUGBOT.md"`)

	out, _ := runCLI(t, "lint")
	lines := findingLines(out, "LINT057")
	if len(lines) != 1 {
		t.Fatalf("want one LINT057 line, got:\n%s", out)
	}
	for _, want := range []string{"**/.cursor/BUGBOT.md", "acts like *", "misses the root and deeper folders"} {
		if !strings.Contains(lines[0], want) {
			t.Errorf("LINT057 line lacks %q:\n%s", want, lines[0])
		}
	}
}

func TestDoctor_WarnsOnDoubleStarUnmanagedEntry(t *testing.T) {
	seedDoubleStarProject(t, `"**/.cursor/BUGBOT.md"`)

	out, _ := runCLI(t, "doctor")
	if !strings.Contains(out, "LINT057") || !strings.Contains(out, "**/.cursor/BUGBOT.md") {
		t.Fatalf("doctor does not name the entry:\n%s", out)
	}
}

func TestLint_NoDoubleStarWarningWithoutDoubleStar(t *testing.T) {
	seedDoubleStarProject(t, `"*/.cursor/BUGBOT.md"`)

	out, _ := runCLI(t, "lint")
	if strings.Contains(out, "LINT057") {
		t.Errorf("unexpected LINT057:\n%s", out)
	}
}

func TestSync_PrintsDoubleStarUnmanagedWarning(t *testing.T) {
	seedDoubleStarProject(t, `"**/.cursor/BUGBOT.md"`)
	notes := captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	got := notes.String()
	if n := strings.Count(got, "**/.cursor/BUGBOT.md"); n != 1 {
		t.Fatalf("want one note naming the entry, got %d:\n%s", n, got)
	}
	if !strings.Contains(got, "acts like *") {
		t.Errorf("note lacks the explanation:\n%s", got)
	}
}

func TestDoubleStarAdvice_NamesTheFixForEachShape(t *testing.T) {
	cases := map[string]string{
		".claude/skills/**":    "write .claude/skills/ to own everything under it",
		"*/.cursor/**":         "one entry per depth",
		"**/.cursor/BUGBOT.md": "misses the root and deeper folders",
	}
	for entry, want := range cases {
		if got := doubleStarAdvice(entry); !strings.Contains(got, want) {
			t.Errorf("advice for %q = %q, want %q", entry, got, want)
		}
	}
	if got := doubleStarAdvice("*/.cursor/**"); strings.Contains(got, "write */.cursor/") {
		t.Errorf("a wildcard folder must not be offered as a trailing-slash entry: %q", got)
	}
}

const localDoubleStarEntry = ".cursor/**/legacy.mdc"

func TestLint_DoubleStarFindingNamesTheLocalFile(t *testing.T) {
	seedDoubleStarProject(t, ".claude/agents/keep.md")
	mustWriteFile(t, "agnostic-ai.local.yaml", "sync:\n  unmanaged:\n    - "+localDoubleStarEntry+"\n")

	out, _ := runCLI(t, "lint")
	lines := findingLines(out, "LINT057")
	if len(lines) != 1 {
		t.Fatalf("want one LINT057 line, got:\n%s", out)
	}
	if !strings.Contains(lines[0], "agnostic-ai.local.yaml") || strings.Contains(lines[0], "agnostic-ai.yaml:") {
		t.Errorf("LINT057 should name the local file:\n%s", lines[0])
	}
}

func TestSync_DoubleStarNoteNamesTheLocalFile(t *testing.T) {
	seedDoubleStarProject(t, ".claude/agents/keep.md")
	mustWriteFile(t, "agnostic-ai.local.yaml", "sync:\n  unmanaged:\n    - "+localDoubleStarEntry+"\n")
	notes := captureNotes(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	if got := notes.String(); !strings.Contains(got, "agnostic-ai.local.yaml: sync.unmanaged") || strings.Contains(got, "agnostic-ai.yaml: ") {
		t.Errorf("note should name the local file:\n%s", got)
	}
}

func TestLint_DoubleStarFindingNamesTheBaseWhenLocalSetsOtherSyncKeys(t *testing.T) {
	seedDoubleStarProject(t, `"**/.cursor/BUGBOT.md"`)
	mustWriteFile(t, "agnostic-ai.local.yaml", "sync:\n  output-manifest: true\n")

	out, _ := runCLI(t, "lint")
	lines := findingLines(out, "LINT057")
	if len(lines) != 1 {
		t.Fatalf("want one LINT057 line, got:\n%s", out)
	}
	if !strings.Contains(lines[0], "agnostic-ai.yaml") || strings.Contains(lines[0], "agnostic-ai.local.yaml") {
		t.Errorf("LINT057 should name the base file:\n%s", lines[0])
	}
}
