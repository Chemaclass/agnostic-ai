package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// runCLI executes one command against a fresh root and returns its
// combined output plus the error the process would exit on.
func runCLI(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	root := NewRootCmd("test")
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// A reporting command that always exits 0 cannot gate a CI step. These
// three printed a problem and passed, so a pinned or broken setup sat
// green for months (#617).

// newProject creates a minimal project with a config file, since every
// command loads one before it can report anything.
func newProject(t *testing.T) string {
	t.Helper()
	dir := testutil.TempCwd(t)
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	silence(t)
	return dir
}

func writeSpec(t *testing.T, dir, rel, body string) {
	t.Helper()
	path := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidate_ExitsNonZeroWhenIssuesFound(t *testing.T) {
	dir := newProject(t)
	// A hook with no event: reported today, and exits 0 today. `name:`
	// is deliberately optional on markdown specs, so it is not an issue.
	writeSpec(t, dir, ".agnostic-ai/hooks/broken.yaml", "command: echo hi\n")

	out, err := runCLI(t, "validate")

	if !strings.Contains(out, "issue(s) found") {
		t.Fatalf("expected the issue report, got:\n%s", out)
	}
	if err == nil {
		t.Error("validate reported issues and exited 0; CI cannot gate on that")
	}
}

func TestValidate_ExitsZeroWhenClean(t *testing.T) {
	dir := newProject(t)
	writeSpec(t, dir, ".agnostic-ai/rules/ok.md", "---\ndescription: fine\n---\nbody\n")

	if _, err := runCLI(t, "validate"); err != nil {
		t.Errorf("a clean project must still exit 0: %v", err)
	}
}

// --fix that resolves everything is a success; --fix that leaves
// something behind is not.
func TestValidate_FixExitsNonZeroOnlyWhenIssuesRemain(t *testing.T) {
	dir := newProject(t)
	writeSpec(t, dir, ".agnostic-ai/rules/ok.md", "---\ndescription: fine\n---\nbody\n")

	if _, err := runCLI(t, "validate", "--fix"); err != nil {
		t.Errorf("nothing to fix must exit 0: %v", err)
	}
}

func TestDoctorCheckGlobs_ExitsNonZeroOnRuleThatNeverLoads(t *testing.T) {
	dir := newProject(t)
	writeSpec(t, dir, ".agnostic-ai/rules/bad.md",
		"---\ndescription: d\nglobs: does-not-exist/**\n---\nbody\n")

	out, err := runCLI(t, "doctor", "--check-globs")

	if !strings.Contains(out, "never load") {
		t.Fatalf("expected the glob report, got:\n%s", out)
	}
	if err == nil {
		t.Fatal("a rule that can never load reported and exited 0")
	}
	// Doctor also fails on sync drift, so pin the reason: without this
	// the test passes on an unrelated failure.
	if !strings.Contains(err.Error(), "glob") {
		t.Errorf("expected the failure to name the glob problem, got: %v", err)
	}
}

// The same run with every glob matching must not fail on globs.
func TestDoctorCheckGlobs_ExitsZeroWhenEveryGlobMatches(t *testing.T) {
	dir := newProject(t)
	writeSpec(t, dir, "README.md", "# readme\n")
	writeSpec(t, dir, ".agnostic-ai/rules/good.md",
		"---\ndescription: d\nglobs: '*.md'\n---\nbody\n")

	_, err := runCLI(t, "doctor", "--check-globs")

	if err != nil && strings.Contains(err.Error(), "glob") {
		t.Errorf("every glob matches, so globs must not be the failure: %v", err)
	}
}
