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

// A target that emits a subset of a skill's frontmatter must not delete
// the rest of it on the way back: sync then import leaves the spec as it
// found it, keys the target cannot express included.
func TestImport_SyncThenImportKeepsSpecFrontmatter(t *testing.T) {
	const spec = "---\nname: gh-issues\ndescription: Walk the open issues.\nargument-hint: \"[--limit N]\"\nallowed-tools: \"Read, Bash(gh *)\"\n---\n\nWalk every open issue.\n"
	for _, target := range []string{"cursor", "copilot", "opencode", "zed"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\n")
			writeFile(t, ".agnostic-ai/skills/gh-issues/SKILL.md", spec)

			execCLI(t, "sync")
			execCLI(t, "import", target)

			if got := readFile(t, ".agnostic-ai/skills/gh-issues/SKILL.md"); got != spec {
				t.Errorf("spec after sync and import:\ngot:\n%s\nwant:\n%s", got, spec)
			}
		})
	}
}

// The same guarantee on the agent surface: every importer that writes an
// agent spec over one already on disk keeps the keys its target drops.
func TestImport_SyncThenImportKeepsAgentSpecFrontmatter(t *testing.T) {
	const spec = "---\nname: reviewer\ndescription: Review the diff.\ntools: [Read, Grep]\neffort: high\n---\n\nReview what changed.\n"
	for _, target := range []string{"antigravity", "cline", "kiro", "qoder", "warp"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\n")
			writeFile(t, ".agnostic-ai/agents/reviewer.md", spec)

			execCLI(t, "sync")
			execCLI(t, "import", target)

			got := readFile(t, ".agnostic-ai/agents/reviewer.md")
			for _, key := range []string{"tools:", "effort:"} {
				if !strings.Contains(got, key) {
					t.Errorf("%s dropped from the spec after sync and import:\n%s", key, got)
				}
			}
		})
	}
}
