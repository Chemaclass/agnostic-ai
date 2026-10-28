package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters/header"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Cursor reads a rule without alwaysApply or globs as not always on, so
// import then sync must not turn it into an always-on rule (#2643).
func TestImportCursor_RuleWithoutActivationStaysOffAcrossSync(t *testing.T) {
	for _, tc := range []struct {
		name, native, wantAlways string
	}{
		{"description only", "---\ndescription: Manual only\n---\nBody here.\n", "alwaysApply: false"},
		{"empty globs", "---\ndescription: Manual only\nglobs:\n---\nBody here.\n", "alwaysApply: false"},
		{"blank globs", "---\ndescription: Manual only\nglobs: \"  \"\n---\nBody here.\n", "alwaysApply: false"},
		{"no frontmatter", "Body here.\n", "alwaysApply: false"},
		{"explicitly off", "---\ndescription: Manual only\nalwaysApply: false\n---\nBody here.\n", "alwaysApply: false"},
		{"always on", "---\ndescription: Everywhere\nalwaysApply: true\n---\nBody here.\n", "alwaysApply: true"},
		{"narrow globs", "---\ndescription: Go files\nglobs: \"*.go\"\n---\nBody here.\n", "alwaysApply: false"},
		{"catch-all globs", "---\ndescription: Every file\nglobs: \"**/*\"\n---\nBody here.\n", "alwaysApply: true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
			mustWriteFile(t, ".cursor/rules/manual.mdc", tc.native)

			if out, err := runCLI(t, "import", "cursor"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}
			spec := readFile(t, ".agnostic-ai/rules/manual.md")
			if out, err := runCLI(t, "sync"); err != nil {
				t.Fatalf("sync: %v\n%s", err, out)
			}

			synced := header.Strip(readFile(t, ".cursor/rules/manual.mdc"))
			if !strings.Contains(synced, tc.wantAlways+"\n") {
				t.Errorf("synced rule lacks %q:\n%s", tc.wantAlways, synced)
			}
			if out, err := runCLI(t, "import", "cursor"); err != nil {
				t.Fatalf("second import: %v\n%s", err, out)
			}
			if got := readFile(t, ".agnostic-ai/rules/manual.md"); got != spec {
				t.Errorf("second import rewrote the spec:\nfirst:\n%s\nsecond:\n%s", spec, got)
			}
		})
	}
}

// A rule scoped to a directory already loads only there, and the flag
// would switch it off in tools such as CodeBuddy, so import leaves it
// out. A folder that only groups rules scopes nothing.
func TestImportCursor_ScopedRuleWithoutActivationGetsNoFlag(t *testing.T) {
	for _, tc := range []struct {
		name, native, scope, spec string
		projectDir, want          bool
	}{
		{"folder names a project directory", ".cursor/rules/backend/manual.mdc", "", ".agnostic-ai/rules/backend/manual.md", true, false},
		{"folder only groups rules", ".cursor/rules/backend/manual.mdc", "", ".agnostic-ai/rules/backend/manual.md", false, true},
		{"explicit scope", ".cursor/rules/manual.mdc", "scope: backend\n", ".agnostic-ai/rules/manual.md", true, false},
		{"empty scope", ".cursor/rules/manual.mdc", "scope: \"\"\n", ".agnostic-ai/rules/manual.md", true, true},
		{"empty scope beats the folder", ".cursor/rules/backend/manual.mdc", "scope: \"\"\n", ".agnostic-ai/rules/backend/manual.md", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
			if tc.projectDir {
				if err := os.MkdirAll("backend", 0o755); err != nil {
					t.Fatal(err)
				}
			}
			mustWriteFile(t, tc.native, "---\ndescription: Manual only\n"+tc.scope+"---\nBody here.\n")

			if out, err := runCLI(t, "import", "cursor"); err != nil {
				t.Fatalf("import: %v\n%s", err, out)
			}

			got := readFile(t, tc.spec)
			if has := strings.Contains(got, "alwaysApply: false"); has != tc.want {
				t.Errorf("alwaysApply: false written = %t, want %t:\n%s", has, tc.want, got)
			}
		})
	}
}
