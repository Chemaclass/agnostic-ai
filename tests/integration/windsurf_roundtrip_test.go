package integration

import (
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestWindsurfRoundTrip_SyncImportSyncIsByteEqual is the windsurf
// audit's byte-stability gate from #336 acceptance criterion C. Skills
// live under the shared .agents/skills/ tree (folder-per-skill), not
// .devin/rules/, since a flat file there never loads as a skill
// (docs.devin.ai/desktop/cascade/skills); the snapshot and wipe steps
// below cover both trees. Workflows excluded: the shared rules-dir
// importer reclassifies by filename prefix, so a workflow at
// .windsurf/workflows/<agent>.md would re-import as a rule and double
// the spec set (same carve-out as the cline audit). MCP servers land
// in .devin/mcp_config.json (#587): the fixture seeds one stdio and
// one remote server so the transport-vs-type rename `import windsurf`
// applies on the way in (see importWindsurfMCP) round-trips too.
func TestWindsurfRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedWindsurfRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "windsurf")
	first := snapshotWindsurfEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no windsurf output")
	}
	if !anyPathUnder(first, ".agents/skills/") {
		t.Fatalf("first sync produced no windsurf skill folders: %v", sortedKeys(first))
	}
	mcpBody, ok := first[".devin/mcp_config.json"]
	if !ok {
		t.Fatalf("first sync produced no .devin/mcp_config.json: %v", sortedKeys(first))
	}
	if !strings.Contains(mcpBody, `"transport": "http"`) {
		t.Fatalf("first sync's mcp file missing transport: http:\n%s", mcpBody)
	}
	if _, ok := first["backend/.devin/rules/auth.md"]; !ok {
		t.Fatalf("first sync produced no scoped rule at backend/.devin/rules/auth.md: %v", sortedKeys(first))
	}
	if body := first[".devin/rules/globbed.md"]; !strings.Contains(body, "trigger: glob") {
		t.Fatalf("first sync's non-always-on rule missing trigger frontmatter:\n%s", body)
	}

	for _, sub := range []string{"agents", "skills", "rules", "mcps"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "windsurf")

	// `backend` holds the scoped rules dir, so it is emit output too and
	// has to go before the second sync rebuilds from the imported specs.
	for _, sub := range []string{".devin", ".agents", "backend"} {
		if err := os.RemoveAll(filepath.Join(dir, sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "sync", "-t", "windsurf")
	second := snapshotWindsurfEmit(t, dir)

	firstPaths := sortedKeys(first)
	secondPaths := sortedKeys(second)
	if !equalStringSlice(firstPaths, secondPaths) {
		t.Fatalf("emit path set changed across round-trip\nfirst:  %v\nsecond: %v",
			firstPaths, secondPaths)
	}
	for _, p := range firstPaths {
		if first[p] != second[p] {
			t.Errorf("byte mismatch at %s (first=%d bytes, second=%d bytes)\n%s",
				p, len(first[p]), len(second[p]), unifiedDiffLines(first[p], second[p]))
		}
	}
}

func TestWindsurfRoundTrip_EveryNativeSkillPathPreservesTriggers(t *testing.T) {
	for _, skillsDir := range []string{".agents/skills", ".devin/skills", ".windsurf/skills"} {
		t.Run(strings.ReplaceAll(skillsDir, "/", "_"), func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			must(t, os.WriteFile("agnostic-ai.yaml", []byte("version: 1\nsources:\n  skills: .agnostic-ai/skills\ntargets: [windsurf]\noutputs:\n  windsurf:\n    skills-dir: "+skillsDir+"\ngitignore:\n  enabled: false\n"), 0o644))
			must(t, os.MkdirAll(".agnostic-ai/skills/security/scripts", 0o755))
			must(t, os.WriteFile(".agnostic-ai/skills/security/SKILL.md", []byte("---\nname: security\ndescription: Security review\nx-windsurf:\n  triggers: [user, model]\n---\n\nReview security.\n"), 0o644))
			must(t, os.WriteFile(".agnostic-ai/skills/security/scripts/check.sh", []byte("#!/bin/sh\nexit 0\n"), 0o755))

			runCmd(t, "sync", "-t", "windsurf")
			firstSkill := readBytes(t, filepath.Join(skillsDir, "security", "SKILL.md"))
			firstAsset := readBytes(t, filepath.Join(skillsDir, "security", "scripts", "check.sh"))
			must(t, os.RemoveAll(".agnostic-ai/skills"))
			runCmd(t, "import", "windsurf")
			must(t, os.RemoveAll(strings.Split(skillsDir, "/")[0]))
			runCmd(t, "sync", "-t", "windsurf")

			if got := readBytes(t, filepath.Join(skillsDir, "security", "SKILL.md")); string(got) != string(firstSkill) {
				t.Errorf("SKILL.md changed across %s round-trip:\n%s", skillsDir, unifiedDiffLines(string(firstSkill), string(got)))
			}
			if got := readBytes(t, filepath.Join(skillsDir, "security", "scripts", "check.sh")); string(got) != string(firstAsset) {
				t.Errorf("asset changed across %s round-trip", skillsDir)
			}
			info, err := os.Stat(filepath.Join(skillsDir, "security", "scripts", "check.sh"))
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
				t.Errorf("asset lost executable mode: %o", info.Mode().Perm())
			}
		})
	}
}

// TestWindsurfRoundTrip_HiddenScopeRoundTrips is the import->sync
// regression for #1123's first finding: `windsurfScopedRulesDirs`
// pruned every hidden directory before ever looking inside it, so a
// rule scoped to `.github` (accepted the same as any other name by
// `CheckScopePath`) emitted fine but never imported back, and a later
// full sync could orphan-sweep the only native copy.
func TestWindsurfRoundTrip_HiddenScopeRoundTrips(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - windsurf
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, ".github", ".devin", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".github", ".devin", "rules", "release.md"),
		[]byte("# release\n\nrelease body\n"), 0o644))

	runCmd(t, "import", "windsurf")

	imported := filepath.Join(dir, ".agnostic-ai", "rules", ".github", "release.md")
	if _, err := os.Stat(imported); err != nil {
		t.Fatalf("missing imported spec %s: %v", imported, err)
	}

	must(t, os.RemoveAll(filepath.Join(dir, ".github")))
	runCmd(t, "sync", "-t", "windsurf")

	raw, err := os.ReadFile(filepath.Join(dir, ".github", ".devin", "rules", "release.md"))
	if err != nil {
		t.Fatalf("scope .github did not re-emit at .github/.devin/rules/release.md: %v", err)
	}
	if !strings.Contains(string(raw), "release body") {
		t.Errorf("expected the scoped body to round-trip, got:\n%s", raw)
	}
}

// TestWindsurfRoundTrip_VendorScopeRoundTrips is the same #1123
// regression for a `vendor`-named scope, which the earlier
// `windsurfScopedRulesDirs` pruned via a hardcoded name list rather
// than a hidden-dir prefix.
func TestWindsurfRoundTrip_VendorScopeRoundTrips(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - windsurf
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "vendor", ".devin", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "vendor", ".devin", "rules", "pkg.md"),
		[]byte("# pkg\n\npkg body\n"), 0o644))

	runCmd(t, "import", "windsurf")

	imported := filepath.Join(dir, ".agnostic-ai", "rules", "vendor", "pkg.md")
	if _, err := os.Stat(imported); err != nil {
		t.Fatalf("missing imported spec %s: %v", imported, err)
	}

	must(t, os.RemoveAll(filepath.Join(dir, "vendor")))
	runCmd(t, "sync", "-t", "windsurf")

	raw, err := os.ReadFile(filepath.Join(dir, "vendor", ".devin", "rules", "pkg.md"))
	if err != nil {
		t.Fatalf("scope vendor did not re-emit at vendor/.devin/rules/pkg.md: %v", err)
	}
	if !strings.Contains(string(raw), "pkg body") {
		t.Errorf("expected the scoped body to round-trip, got:\n%s", raw)
	}
}

// TestWindsurfRoundTrip_NestedScopeMatchingSourceRootNameRoundTrips is
// the import->sync regression for #1123's second finding: pruning by
// directory basename at every depth, not just the exact root-relative
// source path, treated `packages/api/config` as the configured
// `config/rules` source root and pruned it, so
// `packages/api/config/.devin/rules/auth.md` never imported.
func TestWindsurfRoundTrip_NestedScopeMatchingSourceRootNameRoundTrips(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: config/rules
targets:
  - windsurf
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "packages", "api", "config", ".devin", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "packages", "api", "config", ".devin", "rules", "auth.md"),
		[]byte("# auth\n\nauth body\n"), 0o644))

	runCmd(t, "import", "windsurf")

	imported := filepath.Join(dir, "config", "rules", "packages", "api", "config", "auth.md")
	if _, err := os.Stat(imported); err != nil {
		t.Fatalf("missing imported spec %s: %v", imported, err)
	}

	must(t, os.RemoveAll(filepath.Join(dir, "packages")))
	runCmd(t, "sync", "-t", "windsurf")

	raw, err := os.ReadFile(filepath.Join(dir, "packages", "api", "config", ".devin", "rules", "auth.md"))
	if err != nil {
		t.Fatalf("scope packages/api/config did not re-emit: %v", err)
	}
	if !strings.Contains(string(raw), "auth body") {
		t.Errorf("expected the scoped body to round-trip, got:\n%s", raw)
	}
}

// TestWindsurfRoundTrip_NonDefaultRulesDirRootAndScopedRoundTrips is
// the import->sync regression for #1123's second finding on both the
// root (unscoped) rules dir and a scoped copy of it:
// `importFromWindsurf` never received `cfg`, so it read only
// `.devin/rules/` and the legacy `.windsurf/rules/`, and a project that
// set `outputs.windsurf.rules-dir` imported nothing from either path
// sync actually wrote.
func TestWindsurfRoundTrip_NonDefaultRulesDirRootAndScopedRoundTrips(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
sources:
  rules: .agnostic-ai/rules
targets:
  - windsurf
outputs:
  windsurf:
    rules-dir: custom/rules
gitignore:
  enabled: false
`), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "custom", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "custom", "rules", "house.md"),
		[]byte("# house\n\nhouse body\n"), 0o644))
	must(t, os.MkdirAll(filepath.Join(dir, "backend", "custom", "rules"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "backend", "custom", "rules", "auth.md"),
		[]byte("# auth\n\nauth body\n"), 0o644))

	runCmd(t, "import", "windsurf")

	for _, p := range []string{
		filepath.Join(".agnostic-ai", "rules", "house.md"),
		filepath.Join(".agnostic-ai", "rules", "backend", "auth.md"),
	} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("missing imported spec %s: %v", p, err)
		}
	}

	must(t, os.RemoveAll(filepath.Join(dir, "custom")))
	must(t, os.RemoveAll(filepath.Join(dir, "backend")))
	runCmd(t, "sync", "-t", "windsurf")

	root, err := os.ReadFile(filepath.Join(dir, "custom", "rules", "house.md"))
	if err != nil {
		t.Fatalf("custom rules-dir did not re-emit at custom/rules/house.md: %v", err)
	}
	if !strings.Contains(string(root), "house body") {
		t.Errorf("expected the root body to round-trip, got:\n%s", root)
	}
	scoped, err := os.ReadFile(filepath.Join(dir, "backend", "custom", "rules", "auth.md"))
	if err != nil {
		t.Fatalf("scope backend did not re-emit at backend/custom/rules/auth.md: %v", err)
	}
	if !strings.Contains(string(scoped), "auth body") {
		t.Errorf("expected the scoped body to round-trip, got:\n%s", scoped)
	}
}

func seedWindsurfRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
targets:
  - windsurf
gitignore:
  enabled: false
`), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/agents"), 0o755))
	for _, n := range []string{"alpha", "beta", "gamma"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/agents", n+".md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/skills"), 0o755))
	for _, n := range []string{"uno", "dos", "tres"} {
		must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/skills", n), 0o755))
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/skills", n, "SKILL.md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" skill body\n"), 0o644))
	}

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/rules"), 0o755))
	for _, n := range []string{"r1", "r2", "r3"} {
		must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules", n+".md"),
			[]byte("---\nname: "+n+"\n---\n\n"+n+" body\n"), 0o644))
	}
	// A scoped rule emits to backend/.devin/rules/ and a non-always-on
	// one carries `trigger` frontmatter (#628). Both are new discovery
	// and translation paths on the import side, so the round-trip has
	// to cover them.
	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/rules/backend"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules/backend/auth.md"),
		[]byte("---\nname: auth\n---\n\nauth body\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/rules/globbed.md"),
		[]byte("---\nname: globbed\ndescription: Test conventions\nglobs: '**/*.test.ts'\nalwaysApply: false\n---\n\nglobbed body\n"), 0o644))

	must(t, os.MkdirAll(filepath.Join(dir, ".agnostic-ai/mcps"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/fs.yaml"),
		[]byte("name: fs\ncommand: npx\nargs: [\"-y\", \"@modelcontextprotocol/server-filesystem\"]\n"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, ".agnostic-ai/mcps/remote.yaml"),
		[]byte("name: remote\ntype: http\nurl: https://mcp.example.test/mcp\n"), 0o644))
}

// snapshotWindsurfEmit reads every file under a .devin/ (rules, agents)
// or .agents/ (skill folders) directory anywhere in the tree and
// returns a relative-path -> bytes map. Scoped rules put a .devin/ dir
// in a project sub-directory (#628), so matching only at the root would
// miss them. The spec tree is skipped: it is input, not output.
func snapshotWindsurfEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".agnostic-ai" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		slash := "/" + filepath.ToSlash(rel)
		if !strings.Contains(slash, "/.devin/") && !strings.Contains(slash, "/.agents/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[strings.TrimPrefix(slash, "/")] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return out
}
