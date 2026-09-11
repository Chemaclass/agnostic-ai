package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImportIgnore_ReadsHandAuthoredFileOnEverySevenTarget pins #754's
// read side: each of the seven targets with an ignore-file convention
// reconstructs a spec from the file the user wrote by hand. Before this
// there was no importer at all, so patterns a sync destroyed could not
// be recovered.
func TestImportIgnore_ReadsHandAuthoredFileOnEverySevenTarget(t *testing.T) {
	for target, file := range ignoreFileByTarget {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)

			writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+target+"]\n")
			writeFile(t, filepath.Join(dir, filepath.FromSlash(file)),
				"# hand-authored by the team\nmy-secrets/\n*.key\n")

			execCLI(t, "import", target)

			spec := readFile(t, filepath.Join(dir, ".agnostic-ai", "ignore", target+".md"))
			for _, want := range []string{"name: " + target, file, "my-secrets/", "*.key"} {
				if !strings.Contains(spec, want) {
					t.Errorf("imported ignore spec missing %q:\n%s", want, spec)
				}
			}
		})
	}
}

// A file agnostic-ai generated is not user content: the specs behind it
// are already the source of truth. Re-importing it would emit every
// pattern twice on the next sync.
func TestImportIgnore_SkipsItsOwnGeneratedFile(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "ignore", "secrets.md"),
		"---\nname: secrets\n---\n\nmy-secrets/\n")
	execCLI(t, "sync", "-t", "kiro")
	execCLI(t, "import", "kiro")

	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai", "ignore", "kiro.md")); !os.IsNotExist(err) {
		t.Errorf("a generated .kiroignore must not import back into a spec: %v", err)
	}
}

// The import summary counts ignores. Its absence is what made the
// silent overwrite invisible: `import kiro` reported four kinds and
// never mentioned the file it could not read.
func TestImportIgnore_SummaryCountsIgnores(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, ".kiroignore"), "my-secrets/\n")

	var buf bytes.Buffer
	prev := logOut
	logOut = &buf
	defer func() { logOut = prev }()

	execCLI(t, "import", "kiro")

	if !strings.Contains(buf.String(), "1 ignores") {
		t.Errorf("import summary should count ignores, got %q", buf.String())
	}
}

// TestImportIgnore_RescuesAFileSyncRefusesToOverwrite is the end-to-end
// shape of #754: a hand-authored ignore file plus an ignore spec. sync
// refuses rather than destroying the patterns, `import` copies them
// into a spec, and the next sync writes a file holding both sets.
func TestImportIgnore_RescuesAFileSyncRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [kiro]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "ignore", "build.md"),
		"---\nname: build\n---\n\ndist/\nnode_modules/\n")
	const handAuthored = "# hand-authored by the team\nmy-secrets/\n*.key\n"
	writeFile(t, filepath.Join(dir, ".kiroignore"), handAuthored)

	root := NewRootCmd("test")
	root.SetArgs([]string{"sync", "-t", "kiro"})
	if err := root.Execute(); err == nil {
		t.Fatal("sync must refuse to overwrite a hand-authored ignore file")
	}
	if got := readFile(t, filepath.Join(dir, ".kiroignore")); got != handAuthored {
		t.Fatalf("sync must leave the hand-authored file untouched, got:\n%s", got)
	}

	execCLI(t, "import", "kiro")
	execCLI(t, "sync", "-t", "kiro")

	got := readFile(t, filepath.Join(dir, ".kiroignore"))
	for _, want := range []string{"my-secrets/", "*.key", "dist/", "node_modules/"} {
		if !strings.Contains(got, want) {
			t.Errorf("rescued .kiroignore lost %q:\n%s", want, got)
		}
	}
}
