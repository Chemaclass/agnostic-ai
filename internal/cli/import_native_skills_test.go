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
	for _, target := range []string{"cursor", "copilot", "opencode", "zed", "codex"} {
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
			var before map[string]string
			if target == "kiro" {
				before = snapshotEmitted(t, dir)
			}
			execCLI(t, "import", target)

			got := readFile(t, ".agnostic-ai/agents/reviewer.md")
			for _, key := range []string{"tools:", "effort:"} {
				if !strings.Contains(got, key) {
					t.Errorf("%s dropped from the spec after sync and import:\n%s", key, got)
				}
			}
			if target == "kiro" {
				execCLI(t, "sync")
				assertEmittedEqual(t, before, snapshotEmitted(t, dir))
			}
		})
	}
}

func TestImportCodex_ImportsEditsWithoutGeneratedSkillHeader(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	writeFile(t, ".agnostic-ai/skills/example/SKILL.md", "---\nname: example\ndescription: An example.\n---\n\nShared instructions.\n")
	execCLI(t, "sync")
	path := ".agents/skills/example/SKILL.md"
	native := readFile(t, path)
	writeFile(t, path, strings.Replace(native, "Shared instructions.", "Edited Codex instructions.", 1))
	execCLI(t, "import", "codex")
	canonical := readFile(t, ".agnostic-ai/skills/example/SKILL.md")
	if !strings.Contains(canonical, "Edited Codex instructions.") || !strings.Contains(canonical, "Shared instructions.") {
		t.Errorf("import lost authored instructions: %s", canonical)
	}
	if strings.Contains(canonical, "Generated by agnostic-ai") {
		t.Errorf("import included generated provenance in the source: %s", canonical)
	}
}

func TestImportCodex_SyncThenImportKeepsOmittedSkillFields(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	const path = ".agnostic-ai/skills/review/SKILL.md"
	writeFile(t, path, "---\nname: review\ndescription: Review code.\nargument-hint: '[file]'\nallowed-tools: [Read]\neffort: high\n---\n\nReview.\n")
	execCLI(t, "sync")
	execCLI(t, "import", "codex")
	got := readFile(t, path)
	for _, field := range []string{"argument-hint: '[file]'", "allowed-tools: [Read]", "effort: high"} {
		if !strings.Contains(got, field) {
			t.Errorf("omitted skill field %s missing after import:\n%s", field, got)
		}
	}
}
