package emit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeLocalInstructions(t *testing.T, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(ProjectLocalEntryPointPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ProjectLocalEntryPointPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadLocalInstructions_EmptyWhenFileAbsent(t *testing.T) {
	testutilChdir(t, t.TempDir())

	got, err := ReadLocalInstructions()
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected no local instructions, got %q", got)
	}
}

func TestReadLocalInstructions_TrimsTheBody(t *testing.T) {
	testutilChdir(t, t.TempDir())
	writeLocalInstructions(t, "\n\nPrefer tabs.\n\n")

	got, err := ReadLocalInstructions()
	if err != nil {
		t.Fatal(err)
	}
	if got != "Prefer tabs." {
		t.Errorf("got %q, want %q", got, "Prefer tabs.")
	}
}

func TestAppendLocalInstructions_WrapsTheTextAfterTheBody(t *testing.T) {
	got := AppendLocalInstructions("# Shared\n", "Prefer tabs.")

	want := "# Shared\n\n" + LocalStartMarker + "\n\nPrefer tabs.\n\n" + LocalEndMarker + "\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestAppendLocalInstructions_NoOpWithoutLocalText(t *testing.T) {
	if got := AppendLocalInstructions("# Shared\n", ""); got != "# Shared\n" {
		t.Errorf("got %q", got)
	}
}

func TestAppendLocalInstructions_ReplacesAnEarlierBlock(t *testing.T) {
	once := AppendLocalInstructions("# Shared\n", "old")
	twice := AppendLocalInstructions(once, "new")

	if strings.Count(twice, LocalStartMarker) != 1 {
		t.Errorf("blocks stacked:\n%s", twice)
	}
	if strings.Contains(twice, "old") || !strings.Contains(twice, "new") {
		t.Errorf("stale local text survived:\n%s", twice)
	}
}

func TestStripGeneratedAppendices_DropsTheLocalBlock(t *testing.T) {
	body := AppendLocalInstructions("# Shared\n", "Private note.")
	body = AppendRulesAppendix(body, wrapRulesBlock("### r1\n\nrule\n"))

	got := StripGeneratedAppendices(body)
	if got != "# Shared\n" {
		t.Errorf("got %q, want the shared body alone", got)
	}
}
