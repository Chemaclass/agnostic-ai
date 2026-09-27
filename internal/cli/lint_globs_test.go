package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLint_RejectsGlobsThatAreNotAStringOrAList(t *testing.T) {
	dir := budgetProject(t, "targets: [cursor]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "bad.md"), "---\nname: bad\nalwaysApply: false\nglobs: {go: true}\n---\nBody.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "mixed.md"), "---\nname: mixed\nalwaysApply: false\nx-cursor:\n  globs: [\"*.go\", 3]\n---\nBody.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "good.md"), "---\nname: good\nalwaysApply: false\nglobs: [\"*.go\", \"*.mod\"]\n---\nBody.\n")

	out, err := runCLI(t, "lint")
	if err == nil {
		t.Errorf("lint must fail on a malformed globs:\n%s", out)
	}
	lines := findingLines(out, "LINT013")
	if len(lines) != 2 {
		t.Fatalf("expected two LINT013 lines, got:\n%s", out)
	}
	for _, want := range []string{"bad.md: globs", "mixed.md: x-cursor.globs"} {
		if !strings.Contains(strings.Join(lines, "\n"), want) {
			t.Errorf("LINT013 lacks %q:\n%s", want, out)
		}
	}
	if !strings.Contains(lines[0], `globs: "*.go,*.mod"`) {
		t.Errorf("LINT013 should name the fix:\n%s", lines[0])
	}

	out, err = runCLI(t, "validate")
	if err == nil || !strings.Contains(out, "bad.md") || strings.Contains(out, "good.md") {
		t.Errorf("validate must reject only the malformed globs, got %v:\n%s", err, out)
	}
}

func TestCompileGlob_MatchesBraceSets(t *testing.T) {
	re, err := compileGlob("src/**/*.{ts,tsx}")
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]bool{"src/a/b.ts": true, "src/b.tsx": true, "src/b.js": false} {
		if got := re.MatchString(path); got != want {
			t.Errorf("src/**/*.{ts,tsx} on %s = %v, want %v (%s)", path, got, want, re)
		}
	}
}

func TestDoctorCheckGlobs_KeepsABraceSetWhole(t *testing.T) {
	dir := budgetProject(t, "targets: [cursor]\n")
	mustWriteFile(t, filepath.Join(dir, "src", "app.tsx"), "x\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "ts.md"), "---\nname: ts\nalwaysApply: false\nglobs: [\"src/**/*.{ts,tsx}\"]\n---\nBody.\n")

	n, err := reportUnmatchedGlobs(NewRootCmd("test"), dir)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("src/**/*.{ts,tsx} matches src/app.tsx, got %d unmatched rule(s)", n)
	}
}

// A malformed globs reads as none, so sync names LINT013 instead of
// writing an always-on rule with no word.
func TestSync_WarnsOnMalformedGlobs(t *testing.T) {
	dir := budgetProject(t, "targets: [cursor]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "bad.md"), "---\nname: bad\nalwaysApply: false\nglobs: {go: true}\n---\nBody.\n")
	var buf strings.Builder
	prev := logOut
	logOut = &buf
	defer func() { logOut = prev }()

	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(buf.String(), "LINT013") || !strings.Contains(buf.String(), "bad.md") {
		t.Errorf("sync must warn about the malformed globs, got:\n%s", buf.String())
	}
}

func TestCompileGlob_UnpairedBraceStaysLiteral(t *testing.T) {
	re, err := compileGlob("src/{a.go")
	if err != nil {
		t.Fatal(err)
	}
	if !re.MatchString("src/{a.go") {
		t.Errorf("an unpaired brace must match itself (%s)", re)
	}
}

// Kiro writes several patterns as a list; import reads it back as the
// comma form, which syncs to the same list.
func TestKiro_ListFileMatchPatternRoundTrips(t *testing.T) {
	dir := budgetProject(t, "targets: [kiro]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go.md"), "---\nname: go\nalwaysApply: false\nglobs: [\"*.go\", \"src/**/*.{ts,tsx}\"]\n---\nGo body.\n")
	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	steering := filepath.Join(dir, ".kiro", "steering", "go.md")
	first := readFile(t, steering)
	if !strings.Contains(first, "fileMatchPattern:\n") {
		t.Fatalf("kiro must list both patterns:\n%s", first)
	}

	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go.md"), "")
	if out, err := runCLI(t, "import", "kiro"); err != nil {
		t.Fatalf("import kiro: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "go.md")); !strings.Contains(got, "*.go,src/**/*.{ts,tsx}") {
		t.Errorf("import must read the list back as the comma form:\n%s", got)
	}
	if out, err := runCLI(t, "sync", "--gitignore=off"); err != nil {
		t.Fatalf("second sync: %v\n%s", err, out)
	}
	if got := readFile(t, steering); got != first {
		t.Errorf("round trip changed the steering file:\n--- first ---\n%s\n--- second ---\n%s", first, got)
	}
}
