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
