package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportOpencode_PreservesV2CompatibilitySkillWinner(t *testing.T) {
	for _, native := range []bool{false, true} {
		name := "compatibility"
		if native {
			name = "native"
		}
		t.Run(name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
			for _, root := range []string{".claude", ".agents", ".opencode"} {
				if root == ".opencode" && !native {
					continue
				}
				writeFile(t, root+"/skills/review/SKILL.md", "---\nname: review\ndescription: Review changes\n---\n"+root+" winner\n")
				writeFile(t, root+"/skills/review/guide.txt", root+" guide\n")
			}
			execCLI(t, "import", "opencode")
			winner := ".agents"
			if native {
				winner = ".opencode"
			}
			for _, path := range []string{".agnostic-ai/skills/review/SKILL.md", ".agnostic-ai/skills/review/guide.txt"} {
				if got := readFile(t, path); !strings.Contains(got, winner) {
					t.Errorf("%s lost %s winner: %s", path, winner, got)
				}
			}
			execCLI(t, "sync")
			if got := readFile(t, ".opencode/skills/review/SKILL.md"); !strings.Contains(got, winner+" winner") {
				t.Errorf("sync changed winner: %s", got)
			}
			execCLI(t, "sync", "--check")
			execCLI(t, "lint")
		})
	}
}
