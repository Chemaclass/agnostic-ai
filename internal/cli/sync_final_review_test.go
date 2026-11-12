package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const finalReviewRule = "---\nname: example\n---\nKeep changes small.\n"

// Directory settings get no sibling .bak, so one named like another's
// backup is no collision.
func TestSync_DirectorySettingNamedLikeABackup_Passes(t *testing.T) {
	for _, args := range [][]string{{"sync", "--gitignore=off", "--jobs=1"}, {"validate"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "rules", "example.md"), finalReviewRule)
			mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", "example", "SKILL.md"), "---\nname: example\ndescription: An example skill.\n---\nCheck the change.\n")
			mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    rules-dir: rules\n    skills-dir: rules.bak\n")

			if out, err := runCLI(t, args...); err != nil {
				t.Fatalf("%v: %v\n%s", args, err, out)
			}
			if args[0] != "sync" {
				return
			}
			for _, p := range []string{"rules/example.md", "rules.bak/example/SKILL.md"} {
				if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
					t.Errorf("sync must write %s: %v", p, err)
				}
			}
		})
	}
}
