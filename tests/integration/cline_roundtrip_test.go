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

// TestClineRoundTrip_SyncImportSyncIsByteEqual is the cline audit's
// byte-stability gate from #328 acceptance criterion C:
//
//	sync cline -> snapshot .clinerules/* and .cline/*
//	           -> wipe source specs
//	           -> import cline
//	           -> wipe emit
//	           -> sync cline
//	           -> assert byte-for-byte identical
//
// The fixture covers every cline-supported kind (agents, skills,
// rules) with three specimens each. Rules default to `.clinerules/`,
// the only project rules path any Cline surface reads (#853); agents
// and skills stay under `.cline/`. snapshotClineEmit walks both. The
// workflows-dir branch is intentionally left off: the
// importer reclassifies every .md it finds under the rules dir by
// filename prefix, so a workflow at `<rules-dir>/workflows/<agent>.md`
// would re-import as a rule named after the agent, doubling the spec
// set. Workflow round-trip needs a separate harness (importer would
// need to learn the workflows-dir layout).
func TestClineRoundTrip_SyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedClineRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "cline")
	first := snapshotClineEmit(t, dir)
	if len(first) == 0 {
		t.Fatalf("first sync produced no cline output")
	}
	if !anyPathUnder(first, ".cline/skills/") {
		t.Fatalf("first sync produced no cline skill folders: %v", sortedKeys(first))
	}
	if !anyPathUnder(first, ".clinerules/") || !anyPathUnder(first, ".cline/agents/") {
		t.Fatalf("first sync did not default to .clinerules/ and .cline/agents/: %v", sortedKeys(first))
	}
	if anyPathUnder(first, ".cline/rules/") {
		t.Fatalf("no rule may land at the unread .cline/rules/: %v", sortedKeys(first))
	}

	for _, sub := range []string{"agents", "skills", "rules"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "cline")

	for _, sub := range []string{".clinerules", ".cline"} {
		if err := os.RemoveAll(filepath.Join(dir, sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "sync", "-t", "cline")
	second := snapshotClineEmit(t, dir)

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

func seedClineRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  agents: .agnostic-ai/agents
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
targets:
  - cline
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
}

// TestClineRoundTrip_UnreadRulesDirSyncImportSyncIsByteEqual covers a
// project that explicitly opted into outputs.cline.rules-dir:
// .cline/rules, the path the vendor config page shows and no Cline
// loader reads (target-audit 2026-09-18, #853). Agents still emit
// natively at .cline/agents/ regardless of the rules-dir override
// (only the rules destination is configurable), so this fixture skips
// agents and exercises rules + skills, the two kinds
// outputs.cline.rules-dir actually affects.
func TestClineRoundTrip_UnreadRulesDirSyncImportSyncIsByteEqual(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)

	seedClineUnreadRulesDirRoundTripFixture(t, dir)

	runCmd(t, "sync", "-t", "cline")
	first := snapshotClineEmit(t, dir)
	if !anyPathUnder(first, ".cline/rules/") {
		t.Fatalf("first sync did not honor outputs.cline.rules-dir: %v", sortedKeys(first))
	}
	if anyPathUnder(first, ".clinerules/") {
		t.Fatalf("first sync must not also write the default rules dir: %v", sortedKeys(first))
	}

	for _, sub := range []string{"skills", "rules"} {
		if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai", sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "import", "cline")

	for _, sub := range []string{".clinerules", ".cline"} {
		if err := os.RemoveAll(filepath.Join(dir, sub)); err != nil {
			t.Fatal(err)
		}
	}

	runCmd(t, "sync", "-t", "cline")
	second := snapshotClineEmit(t, dir)

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

func TestClineRoundTrip_EveryNativeSkillPath(t *testing.T) {
	for _, skillsDir := range []string{".cline/skills", ".clinerules/skills", ".claude/skills"} {
		t.Run(strings.ReplaceAll(skillsDir, "/", "_"), func(t *testing.T) {
			dir := t.TempDir()
			testutil.Chdir(t, dir)
			must(t, os.WriteFile("agnostic-ai.yaml", []byte("version: 1\nsources:\n  skills: .agnostic-ai/skills\ntargets: [cline]\noutputs:\n  cline:\n    skills-dir: "+skillsDir+"\ngitignore:\n  enabled: false\n"), 0o644))
			must(t, os.MkdirAll(".agnostic-ai/skills/review/scripts", 0o755))
			must(t, os.WriteFile(".agnostic-ai/skills/review/SKILL.md", []byte("---\nname: review\ndescription: Review changes\n---\n\nReview the diff.\n"), 0o644))
			must(t, os.WriteFile(".agnostic-ai/skills/review/scripts/check.sh", []byte("#!/bin/sh\nexit 0\n"), 0o755))

			runCmd(t, "sync", "-t", "cline")
			firstSkill := readBytes(t, filepath.Join(skillsDir, "review", "SKILL.md"))
			firstAsset := readBytes(t, filepath.Join(skillsDir, "review", "scripts", "check.sh"))
			must(t, os.RemoveAll(".agnostic-ai/skills"))
			runCmd(t, "import", "cline")
			must(t, os.RemoveAll(strings.Split(skillsDir, "/")[0]))
			runCmd(t, "sync", "-t", "cline")

			if got := readBytes(t, filepath.Join(skillsDir, "review", "SKILL.md")); string(got) != string(firstSkill) {
				t.Errorf("SKILL.md changed across %s round-trip:\n%s", skillsDir, unifiedDiffLines(string(firstSkill), string(got)))
			}
			if got := readBytes(t, filepath.Join(skillsDir, "review", "scripts", "check.sh")); string(got) != string(firstAsset) {
				t.Errorf("asset changed across %s round-trip", skillsDir)
			}
			info, err := os.Stat(filepath.Join(skillsDir, "review", "scripts", "check.sh"))
			if err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" && info.Mode().Perm()&0o111 == 0 {
				t.Errorf("asset lost executable mode: %o", info.Mode().Perm())
			}
		})
	}
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func seedClineUnreadRulesDirRoundTripFixture(t *testing.T, dir string) {
	t.Helper()
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte(`version: 1
sources:
  skills: .agnostic-ai/skills
  rules: .agnostic-ai/rules
targets:
  - cline
outputs:
  cline:
    rules-dir: .cline/rules
gitignore:
  enabled: false
`), 0o644))

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
}

// snapshotClineEmit reads every file under .clinerules/ (the default
// rules directory, and the only one Cline reads) and .cline/ (agents,
// skills, and the opt-in unread rules path) and returns a
// relative-path -> bytes map.
func snapshotClineEmit(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, sub := range []string{".clinerules", ".cline"} {
		full := filepath.Join(root, sub)
		info, err := os.Stat(full)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatalf("stat %s: %v", full, err)
		}
		if !info.IsDir() {
			continue
		}
		err = filepath.WalkDir(full, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = string(data)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", full, err)
		}
	}
	return out
}

// anyPathUnder reports whether any key of snapshot has prefix. Used to
// assert a round-trip snapshot actually exercised a given subtree
// instead of passing vacuously because that tree was empty.
func anyPathUnder(snapshot map[string]string, prefix string) bool {
	for p := range snapshot {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}
