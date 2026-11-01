package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func seedDoubleStarProject(t *testing.T, unmanaged string) {
	t.Helper()
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
	for _, want := range []string{"**/.cursor/BUGBOT.md", "matches exactly one folder"} {
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
	if !strings.Contains(got, "matches exactly one folder") {
		t.Errorf("note lacks the explanation:\n%s", got)
	}
}
