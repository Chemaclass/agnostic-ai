package cli

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const fencedDemoSkill = "---\nname: demo\ndescription: Demo skill.\n---\n\n# demo\n\n::target claude\n\nClaude only line.\n\n::end\n\nShared line.\n"

// With memory.personal: repo the preview ran in a copy without .git, so
// the repo store did not resolve there and the preview stopped seeing the
// generated skill files as sync output (#2181).
func TestImport_DryRunMatchesImportWithRepoMemory(t *testing.T) {
	for _, mode := range []string{"checkout", "repo"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "repo" {
				repoMemoryProject(t, true)
			} else {
				testutil.TempCwd(t)
				if out, err := exec.Command("git", "init", "-q").CombinedOutput(); err != nil {
					t.Fatalf("git init: %v\n%s", err, out)
				}
				silence(t)
				mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, cursor]\nbuiltins: [memory]\ngitignore:\n  enabled: true\n")
			}
			mustWriteFile(t, ".agnostic-ai/skills/demo/SKILL.md", fencedDemoSkill)
			if err := runSync(t); err != nil {
				t.Fatal(err)
			}

			var previewErr error
			out := captureStdout(t, func() {
				_, previewErr = runCLI(t, "import", "cursor", "--dry-run", "--diff")
			})
			if previewErr != nil {
				t.Errorf("import --dry-run --diff: %v", previewErr)
			}
			if strings.Contains(out, "  change ") || strings.Contains(out, "::target claude") {
				t.Errorf("preview reports changes the import does not make:\n%s", out)
			}
			if !strings.Contains(out, "unchanged .agnostic-ai/skills/demo/SKILL.md") {
				t.Errorf("preview does not list the skill as unchanged:\n%s", out)
			}

			if out, err := runCLI(t, "import", "cursor"); err != nil {
				t.Fatalf("import cursor: %v\n%s", err, out)
			}
			if got := readFile(t, ".agnostic-ai/skills/demo/SKILL.md"); got != fencedDemoSkill {
				t.Errorf("import rewrote the skill:\n%s", got)
			}
			if _, err := os.Stat(".git"); err != nil {
				t.Errorf(".git: %v", err)
			}
		})
	}
}
