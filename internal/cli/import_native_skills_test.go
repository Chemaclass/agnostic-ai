package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImport_PreservesSharedNativeSkillsAndAssets(t *testing.T) {
	for _, target := range []string{"zed", "warp", "antigravity"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\n")
			const body = "---\nname: deploy\ndescription: Deploy the service.\n---\n\nRead references/checks.txt.\n"
			writeFile(t, ".agents/skills/deploy/SKILL.md", body)
			writeFile(t, ".agents/skills/deploy/references/checks.txt", "Check the service status.\n")
			var output bytes.Buffer
			previous := logOut
			logOut = &output
			t.Cleanup(func() { logOut = previous })

			execCLI(t, "import", target)

			if got := readFile(t, ".agnostic-ai/skills/deploy/SKILL.md"); got != body {
				t.Errorf("skill body = %q, want %q", got, body)
			}
			if got := readFile(t, filepath.Join(dir, ".agnostic-ai/skills/deploy/references/checks.txt")); got != "Check the service status.\n" {
				t.Errorf("skill asset = %q", got)
			}
			if !strings.Contains(output.String(), "1 skills") {
				t.Errorf("summary does not count the skill: %s", output.String())
			}
		})
	}
}

func TestImportAntigravity_PrefersCurrentSkillsAndFallsBackToLegacy(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [antigravity]\n")
	writeFile(t, ".agent/skills/deploy/SKILL.md", "Legacy skill.\n")

	execCLI(t, "import", "antigravity")
	if got := readFile(t, ".agnostic-ai/skills/deploy/SKILL.md"); got != "Legacy skill.\n" {
		t.Errorf("legacy fallback = %q", got)
	}

	writeFile(t, ".agents/skills/deploy/SKILL.md", "Current skill.\n")
	execCLI(t, "import", "antigravity")
	if got := readFile(t, ".agnostic-ai/skills/deploy/SKILL.md"); got != "Current skill.\n" {
		t.Errorf("preferred skill = %q", got)
	}
}
