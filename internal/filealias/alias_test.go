package filealias

import (
	"os"
	"path/filepath"
	"testing"
)

func caseInsensitiveDisk(t *testing.T, dir string) bool {
	t.Helper()
	probe := filepath.Join(dir, "probe")
	if err := os.WriteFile(probe, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Remove(probe) }()
	_, err := os.Stat(filepath.Join(dir, "PROBE"))
	return err == nil
}

func TestMatch_OtherLetterCaseMatchesOnCaseInsensitiveDisk(t *testing.T) {
	dir := t.TempDir()
	if !caseInsensitiveDisk(t, dir) {
		t.Skip("disk is case-sensitive")
	}
	path := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "agents.md")
	var c Cache
	got, err := c.Match(path, nil, []string{output})
	if err != nil {
		t.Fatal(err)
	}
	if got != output {
		t.Errorf("Match = %q, want %q", got, output)
	}
}

func TestMatch_HardLinksWithDifferentNamesDoNotMatch(t *testing.T) {
	dir := t.TempDir()
	if caseInsensitiveDisk(t, dir) {
		t.Skip("disk is case-insensitive, so both names are one entry")
	}
	path := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "agents.md")
	if err := os.Link(path, output); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	var c Cache
	got, err := c.Match(path, nil, []string{output})
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("Match = %q, want no match", got)
	}
}

func TestMatch_MissingPathReturnsNothing(t *testing.T) {
	dir := t.TempDir()
	var c Cache
	got, err := c.Match(filepath.Join(dir, "missing"), nil, []string{filepath.Join(dir, "MISSING")})
	if err != nil || got != "" {
		t.Errorf("Match = %q, %v; want no match and no error", got, err)
	}
}

func TestMatch_SymlinksToOneTargetDoNotMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "real"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	first, second := filepath.Join(dir, "one", "link"), filepath.Join(dir, "two", "LINK")
	for _, link := range []string{first, second} {
		if err := os.Mkdir(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join("..", "real"), link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
	}
	var c Cache
	got, err := c.Match(first, nil, []string{second})
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("Match = %q, want no match", got)
	}
}
