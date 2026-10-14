package cli

import (
	"os"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/errs"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Import reported AAI-203 for a skill whose frontmatter parses the same
// as the native copy but is written another way (#2182).
func TestImport_EqualFrontmatterIsNeitherAConflictNorARewrite(t *testing.T) {
	const native = "---\nname: review\ndescription: Review a pull request before merge.\n---\nBody\n"
	for _, tc := range []struct {
		name   string
		source string
	}{
		{"folded scalar", "---\nname: review\ndescription: >-\n  Review a pull request before merge.\n---\nBody\n"},
		{"literal scalar", "---\nname: review\ndescription: |-\n  Review a pull request before merge.\n---\nBody\n"},
		{"double quoted", "---\nname: review\ndescription: \"Review a pull request before merge.\"\n---\nBody\n"},
		{"single quoted", "---\nname: review\ndescription: 'Review a pull request before merge.'\n---\nBody\n"},
		{"key order", "---\ndescription: Review a pull request before merge.\nname: review\n---\nBody\n"},
		{"comment", "---\n# kept by hand\nname: review\ndescription: Review a pull request before merge.\n---\nBody\n"},
	} {
		for _, tool := range []struct{ target, native string }{
			{"cursor", ".cursor/skills/review/SKILL.md"},
			{"claude", ".claude/skills/review/SKILL.md"},
		} {
			t.Run(tool.target+"/"+tc.name, func(t *testing.T) {
				testutil.Chdir(t, t.TempDir())
				silence(t)
				mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tool.target+"]\n")
				mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", tc.source)
				mustWriteFile(t, tool.native, native)
				stamp := time.Unix(1_000_000_000, 0)
				if err := os.Chtimes(".agnostic-ai/skills/review/SKILL.md", stamp, stamp); err != nil {
					t.Fatal(err)
				}

				if out, err := runCLI(t, "import", tool.target); err != nil {
					t.Fatalf("import stopped on equal content: %v\n%s", err, out)
				}

				if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != tc.source {
					t.Errorf("source = %q, want it byte-identical to %q", got, tc.source)
				}
				info, err := os.Stat(".agnostic-ai/skills/review/SKILL.md")
				if err != nil {
					t.Fatal(err)
				}
				if !info.ModTime().Equal(stamp) {
					t.Error("import rewrote the source file")
				}
			})
		}
	}
}

func TestImport_DifferentFrontmatterOrBodyStillConflicts(t *testing.T) {
	const folded = "---\nname: review\ndescription: >-\n  Review a pull request before merge.\n---\nBody\n"
	const plain = "---\nname: review\ndescription: Review a pull request before merge.\n---\nBody\n"
	for name, tc := range map[string]struct{ source, native string }{
		"description": {folded, "---\nname: review\ndescription: Review a pull request after merge.\n---\nBody\n"},
		"body":        {folded, "---\nname: review\ndescription: Review a pull request before merge.\n---\nOther\n"},
		"extra key":   {folded, "---\nname: review\ndescription: Review a pull request before merge.\nlicense: MIT\n---\nBody\n"},
		// `>` keeps the trailing newline, so the value differs from the plain scalar.
		"folded keep, not last key": {"---\ndescription: >\n  Review a pull request before merge.\nname: review\n---\nBody\n", plain},
	} {
		t.Run(name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
			mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", tc.source)
			mustWriteFile(t, ".cursor/skills/review/SKILL.md", tc.native)

			_, err := runCLI(t, "import", "cursor")

			if errs.CodeOf(err) != errs.CodeImportWouldReplace {
				t.Fatalf("import did not stop on different content: %v", err)
			}
			if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != tc.source {
				t.Errorf("source = %q, want it untouched", got)
			}
		})
	}
}

// Rules and commands are written whole instead of merged, and equal
// frontmatter has to hold there too (#2182).
func TestImport_EqualFrontmatterHoldsForRulesAndCommands(t *testing.T) {
	const folded = "---\nname: style\ndescription: >-\n  Keep the style guide.\n---\nBody\n"
	const plain = "---\nname: style\ndescription: Keep the style guide.\n---\nBody\n"
	for _, tc := range []struct{ name, target, native, nativeBody, source string }{
		{"cursor rule", "cursor", ".cursor/rules/style.mdc", "---\ndescription: Keep the style guide.\n---\nBody\n", ".agnostic-ai/rules/style.md"},
		{"claude rule", "claude", ".claude/rules/style.md", plain, ".agnostic-ai/rules/style.md"},
		{"claude command", "claude", ".claude/commands/style.md", plain, ".agnostic-ai/commands/style.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+tc.target+"]\n")
			mustWriteFile(t, tc.source, folded)
			mustWriteFile(t, tc.native, tc.nativeBody)
			stamp := time.Unix(1_000_000_000, 0)
			if err := os.Chtimes(tc.source, stamp, stamp); err != nil {
				t.Fatal(err)
			}

			if out, err := runCLI(t, "import", tc.target); err != nil {
				t.Fatalf("import stopped on equal content: %v\n%s", err, out)
			}

			if got := readFile(t, tc.source); got != folded {
				t.Errorf("source = %q, want it byte-identical to %q", got, folded)
			}
			info, err := os.Stat(tc.source)
			if err != nil {
				t.Fatal(err)
			}
			if !info.ModTime().Equal(stamp) {
				t.Error("import rewrote the source file")
			}
		})
	}
}
