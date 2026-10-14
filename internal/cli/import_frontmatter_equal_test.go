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
	const source = "---\nname: review\ndescription: >-\n  Review a pull request before merge.\n---\nBody\n"
	for name, native := range map[string]string{
		"description": "---\nname: review\ndescription: Review a pull request after merge.\n---\nBody\n",
		"body":        "---\nname: review\ndescription: Review a pull request before merge.\n---\nOther\n",
		"extra key":   "---\nname: review\ndescription: Review a pull request before merge.\nlicense: MIT\n---\nBody\n",
	} {
		t.Run(name, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			silence(t)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
			mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", source)
			mustWriteFile(t, ".cursor/skills/review/SKILL.md", native)

			_, err := runCLI(t, "import", "cursor")

			if errs.CodeOf(err) != errs.CodeImportWouldReplace {
				t.Fatalf("import did not stop on different content: %v", err)
			}
			if got := readFile(t, ".agnostic-ai/skills/review/SKILL.md"); got != source {
				t.Errorf("source = %q, want it untouched", got)
			}
		})
	}
}
