package cli

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestValidate_AbsoluteSourceNotesOnlyNameMissingDirectories(t *testing.T) {
	dir := testutil.TempCwd(t)
	external := t.TempDir()
	rules := filepath.Join(external, "rules")
	missing := filepath.Join(external, "hooks")
	mustWriteFile(t, filepath.Join(rules, "demo.md"), "---\ndescription: demo\n---\nExternal rule.\n")
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), fmt.Sprintf(
		"version: 1\nsources:\n  rules: %q\n  hooks: %q\ntargets: [claude]\n",
		filepath.ToSlash(rules), filepath.ToSlash(missing)))

	out, err := runCLI(t, "validate")
	if err != nil {
		t.Fatalf("validate failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "note: "+filepath.ToSlash(rules)+": directory not found") {
		t.Errorf("existing absolute source was reported missing:\n%s", out)
	}
	if !strings.Contains(out, "note: "+filepath.ToSlash(missing)+": directory not found (no hooks will be emitted)") {
		t.Errorf("missing absolute source was not reported:\n%s", out)
	}
	if !strings.Contains(out, "loaded 1 entries.") {
		t.Errorf("external rule was not loaded:\n%s", out)
	}
}

// A declared source whose directory is missing is reported; a declared
// source that exists, and an undeclared (defaulted) kind, are not.
func TestMissingSourceNotes_FlagsDeclaredMissingDirs(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\nsources:\n  rules: .agnostic-ai/rules\n  skills: .agnostic-ai/skills\n")
	// Only rules/ exists on disk.
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "r.md"), "body\n")

	got := missingSourceNotes(dir)
	if len(got) != 1 {
		t.Fatalf("expected 1 note (skills missing), got %d: %+v", len(got), got)
	}
	if got[0].Field != "sources.skills" {
		t.Errorf("Field = %q, want sources.skills", got[0].Field)
	}
	if got[0].Path != ".agnostic-ai/skills" {
		t.Errorf("Path = %q, want .agnostic-ai/skills", got[0].Path)
	}
}

// When no sources are declared, defaults apply and nothing is flagged
// even though the convention dirs do not exist.
func TestMissingSourceNotes_IgnoresUndeclaredDefaults(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets:\n  - claude\n")

	if got := missingSourceNotes(dir); len(got) != 0 {
		t.Errorf("expected no notes for undeclared sources, got %+v", got)
	}
}

// Git never tracks an empty directory, so a fresh clone of a project
// whose sources list an empty hooks/ lacks it. validate names the path
// but still passes (#1491).
func TestValidate_MissingSourceDirIsNoteNotFailure(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\nsources:\n  rules: .agnostic-ai/rules\n  hooks: .agnostic-ai/hooks\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "demo.md"), "---\ndescription: demo\n---\nBody.\n")

	out, err := runCLI(t, "validate")
	if err != nil {
		t.Fatalf("validate failed on a missing source dir: %v\n%s", err, out)
	}
	if !strings.Contains(out, "note: .agnostic-ai/hooks: directory not found (no hooks will be emitted)") {
		t.Errorf("expected a note naming the missing dir, got:\n%s", out)
	}
	if strings.Contains(out, "issue(s) found") {
		t.Errorf("missing source dir counted as an issue:\n%s", out)
	}
}

// With no specs at all, the missing dirs still print as notes and do
// not fail the run.
func TestValidate_MissingSourceDirsWithNoSpecsPass(t *testing.T) {
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\nsources:\n  hooks: .agnostic-ai/hooks\n  mcps: .agnostic-ai/mcps\n")

	out, err := runCLI(t, "validate")
	if err != nil {
		t.Fatalf("validate failed on missing source dirs: %v\n%s", err, out)
	}
	for _, want := range []string{".agnostic-ai/hooks", ".agnostic-ai/mcps"} {
		if !strings.Contains(out, "note: "+want+": directory not found") {
			t.Errorf("expected a note for %s, got:\n%s", want, out)
		}
	}
}

// A config with no file at all yields no notes (handled elsewhere).
func TestMissingSourceNotes_NoConfig(t *testing.T) {
	if got := missingSourceNotes(t.TempDir()); len(got) != 0 {
		t.Errorf("expected no notes without a config file, got %+v", got)
	}
}
