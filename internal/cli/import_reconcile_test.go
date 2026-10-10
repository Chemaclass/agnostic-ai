package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportReconcile_MapsConcurrentSkillChangesWithoutWriting(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	writeFile(t, "agnostic-ai.yaml", "version: 1\nsources:\n  skills: specs/skills\ntargets: [cursor]\n")
	for _, name := range []string{"edit", "delete", "conflict"} {
		writeFile(t, ".cursor/skills/"+name+"/SKILL.md", "---\nname: "+name+"\n---\nOriginal\n")
	}
	writeFile(t, ".cursor/skills/edit/assets/data.txt", "old asset")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	base := git(t, dir, "rev-parse", "HEAD")
	if err := os.MkdirAll("specs", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(".cursor/skills", "specs/skills"); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "migrate")
	migrated := git(t, dir, "rev-parse", "HEAD")
	writeFile(t, "specs/skills/conflict/SKILL.md", "canonical edit\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "canonical edit")
	current := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", base)
	writeFile(t, ".cursor/skills/edit/SKILL.md", "upstream edit\n")
	writeFile(t, ".cursor/skills/edit/assets/data.txt", "new asset")
	writeFile(t, ".cursor/skills/add/SKILL.md", "new skill\n")
	writeFile(t, ".cursor/skills/conflict/SKILL.md", "upstream conflict\n")
	git(t, dir, "rm", "-qr", ".cursor/skills/delete")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "upstream")
	upstream := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", current)
	before := git(t, dir, "status", "--porcelain=v1")
	indexPath := git(t, dir, "rev-parse", "--git-path", "index")
	indexBefore, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd("dev")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"import", "reconcile", "--base", base, "--migrated", migrated, "--upstream", upstream, "--map", ".cursor/skills=specs/skills"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"add\t.cursor/skills/add/SKILL.md\tspecs/skills/add/SKILL.md",
		"remove\t.cursor/skills/delete/SKILL.md\tspecs/skills/delete/SKILL.md",
		"update\t.cursor/skills/edit/SKILL.md\tspecs/skills/edit/SKILL.md",
		"update\t.cursor/skills/edit/assets/data.txt\tspecs/skills/edit/assets/data.txt",
		"conflict\t.cursor/skills/conflict/SKILL.md\tspecs/skills/conflict/SKILL.md",
		base, migrated, upstream, current,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing %q in %s", want, &out)
		}
	}
	if after := git(t, dir, "status", "--porcelain=v1"); after != before {
		t.Errorf("status changed: %q -> %q", before, after)
	}
	indexAfter, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(indexBefore, indexAfter) {
		t.Error("index bytes changed")
	}
	if got := readFile(t, "specs/skills/edit/assets/data.txt"); got != "old asset" {
		t.Errorf("asset overwritten: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "specs/skills/add")); !os.IsNotExist(err) {
		t.Errorf("added skill written: %v", err)
	}
}

func TestImportReconcile_UpstreamAdditionsConflictWithMigrationFiles(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	writeFile(t, "native/base/SKILL.md", "base")
	writeFile(t, ".agnostic-ai/skills/new/SKILL.md", "independent canonical skill")
	writeFile(t, ".agnostic-ai/skills/base/assets/data.txt", "independent canonical asset")
	writeFile(t, ".agnostic-ai/skills/same/SKILL.md", "convergent")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	base := git(t, dir, "rev-parse", "HEAD")
	writeFile(t, ".agnostic-ai/skills/base/SKILL.md", "base")
	git(t, dir, "rm", "-qr", "native")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "migration")
	migrated := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", base)
	writeFile(t, "native/new/SKILL.md", "upstream skill")
	writeFile(t, "native/base/assets/data.txt", "upstream asset")
	writeFile(t, "native/same/SKILL.md", "convergent")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "upstream additions")
	upstream := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", migrated)
	plan, err := planSkillReconciliation(base, migrated, upstream, []string{"native=.agnostic-ai/skills"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Entries) != 2 {
		t.Fatalf("expected two conflicts with convergence omitted: %+v", plan.Entries)
	}
	for _, entry := range plan.Entries {
		if entry.Action != "conflict" {
			t.Errorf("existing canonical destination reported as %+v", entry)
		}
	}
}

func TestImportReconcile_DestinationHierarchy(t *testing.T) {
	cases := []struct {
		name, canonical, nativeBase, upstream, second string
		want                                          string
	}{
		{"canonical ancestor", "skill/assets", "", "skill/assets/data", "", "conflict"},
		{"canonical descendant", "skill/assets/data", "", "skill/assets", "", "conflict"},
		{"canonical non-skill file", "new", "", "new/SKILL.md", "", "conflict"},
		{"proposed ancestor", "", "", "skill/assets", "skill/assets/data", "conflict"},
		{"replace file with directory", "skill/assets", "skill/assets", "skill/assets/data", "", "add"},
		{"replace directory with file", "skill/assets/data", "skill/assets/data", "skill/assets", "", "add"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := setupGitRepo(t)
			testutil.Chdir(t, dir)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
			writeFile(t, "native/skill/SKILL.md", "base")
			if c.nativeBase != "" {
				writeFile(t, "native/"+c.nativeBase, "asset")
			}
			if c.second != "" {
				writeFile(t, "other/skill/SKILL.md", "base")
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-qm", "base")
			base := git(t, dir, "rev-parse", "HEAD")
			writeFile(t, ".agnostic-ai/skills/skill/SKILL.md", "base")
			if c.canonical != "" {
				writeFile(t, ".agnostic-ai/skills/"+c.canonical, "asset")
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-qm", "migration")
			migrated := git(t, dir, "rev-parse", "HEAD")
			git(t, dir, "checkout", "-q", base)
			if c.nativeBase != "" {
				git(t, dir, "rm", "-q", "native/"+c.nativeBase)
			}
			writeFile(t, "native/"+c.upstream, "upstream")
			mappings := []string{"native=.agnostic-ai/skills"}
			if c.second != "" {
				writeFile(t, "other/"+c.second, "upstream")
				mappings = append(mappings, "other=.agnostic-ai/skills")
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-qm", "upstream")
			upstream := git(t, dir, "rev-parse", "HEAD")
			git(t, dir, "checkout", "-q", migrated)
			plan, err := planSkillReconciliation(base, migrated, upstream, mappings)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, entry := range plan.Entries {
				if entry.Source == "native/"+c.upstream {
					found = true
					if entry.Action != c.want {
						t.Errorf("hierarchy: %+v, want %s", entry, c.want)
					}
				}
				if c.second != "" && entry.Action != "conflict" {
					t.Errorf("competing destinations: %+v", entry)
				}
			}
			if !found {
				t.Fatalf("upstream addition absent: %+v", plan.Entries)
			}
		})
	}
}

func TestImportReconcile_PreservesCanonicalDeletionAndRecognizesConvergence(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	for _, name := range []string{"deleted", "same"} {
		writeFile(t, "native/"+name+"/SKILL.md", "original")
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	base := git(t, dir, "rev-parse", "HEAD")
	if err := os.MkdirAll(".agnostic-ai", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename("native", ".agnostic-ai/skills"); err != nil {
		t.Fatal(err)
	}
	// The imported representation can differ from the native base.
	writeFile(t, ".agnostic-ai/skills/same/SKILL.md", "normalized")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-qm", "migrate")
	migrated := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "rm", "-qr", ".agnostic-ai/skills/deleted")
	writeFile(t, ".agnostic-ai/skills/same/SKILL.md", "converged")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "canonical changes")
	current := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", base)
	writeFile(t, "native/deleted/SKILL.md", "upstream edit")
	writeFile(t, "native/same/SKILL.md", "converged")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "upstream")
	upstream := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", current)
	writeFile(t, "unrelated.txt", "staged unrelated change")
	git(t, dir, "add", "unrelated.txt")
	before := git(t, dir, "diff", "--cached")
	var out bytes.Buffer
	cmd := NewRootCmd("dev")
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"import", "reconcile", "--base", base, "--migrated", migrated, "--upstream", upstream, "--map", "native=.agnostic-ai/skills", "--json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var plan reconciliationPlan
	if err := json.Unmarshal(out.Bytes(), &plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Entries) != 1 || plan.Entries[0].Action != "conflict" || plan.Entries[0].Source != "native/deleted/SKILL.md" {
		t.Errorf("plan: %+v", plan)
	}
	if after := git(t, dir, "diff", "--cached"); after != before {
		t.Errorf("index changed")
	}
	if _, err := os.Stat(".agnostic-ai/skills/deleted"); !os.IsNotExist(err) {
		t.Errorf("resurrected deletion: %v", err)
	}
}

func TestImportReconcile_RejectsUnsafeMappingsAndDirtySources(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	writeFile(t, "native/example/SKILL.md", "original")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	for _, mapping := range []string{"../native=.agnostic-ai/skills", "native=/tmp/skills", "native=wrong", "native=native", "native\\skills=.agnostic-ai/skills"} {
		if _, err := planSkillReconciliation("HEAD", "HEAD", "HEAD", []string{mapping}); err == nil {
			t.Errorf("accepted %q", mapping)
		}
	}
	if _, err := planSkillReconciliation("--help", "HEAD", "HEAD", []string{"native=.agnostic-ai/skills"}); err == nil {
		t.Error("accepted invalid revision")
	}
	if _, err := planSkillReconciliation("HEAD", "HEAD", "HEAD", []string{"native=.agnostic-ai/skills"}); err == nil || !strings.Contains(err.Error(), "no canonical counterpart") {
		t.Errorf("incomplete migration: %v", err)
	}
	writeFile(t, "native/example/SKILL.md", "dirty")
	if _, err := planSkillReconciliation("HEAD", "HEAD", "HEAD", []string{"native=.agnostic-ai/skills"}); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("dirty source: %v", err)
	}
}

func TestImportReconcile_TreeIncludesExecutableAssetsAndRejectsLinks(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	writeFile(t, "native/space name/SKILL.md", "skill")
	writeFile(t, "native/space name/run.sh", "echo example\n")
	writeFile(t, "native/not-a-skill/asset.txt", "unrelated")
	git(t, dir, "add", ".")
	git(t, dir, "update-index", "--chmod=+x", "native/space name/run.sh")
	git(t, dir, "commit", "-qm", "base")
	tree, err := reconciliationTree("HEAD", "native", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree) != 2 || !strings.HasPrefix(tree["space name/run.sh"], "100755 ") {
		t.Errorf("tree: %+v", tree)
	}
	// Build a symlink Git entry without requiring OS symlink privileges.
	git(t, dir, "update-index", "--add", "--cacheinfo", "120000,"+strings.Fields(tree["space name/run.sh"])[2]+",native/space name/link")
	git(t, dir, "commit", "-qm", "link")
	if _, err := reconciliationTree("HEAD", "native", true); err == nil || !strings.Contains(err.Error(), "unsupported linked") {
		t.Errorf("linked tree: %v", err)
	}
}

func TestImportReconcile_ReportsCompetingMappedSourcesAsConflicts(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
	for _, source := range []string{"left", "right"} {
		writeFile(t, source+"/example/SKILL.md", "original")
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "base")
	base := git(t, dir, "rev-parse", "HEAD")
	writeFile(t, ".agnostic-ai/skills/example/SKILL.md", "original")
	git(t, dir, "rm", "-qr", "left", "right")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "migrate")
	migrated := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", base)
	for _, source := range []string{"left", "right"} {
		writeFile(t, source+"/example/SKILL.md", source+" update")
	}
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "upstream")
	upstream := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", migrated)
	plan, err := planSkillReconciliation(base, migrated, upstream, []string{"left=.agnostic-ai/skills", "right=.agnostic-ai/skills"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Entries) != 2 {
		t.Fatalf("entries: %+v", plan.Entries)
	}
	for _, entry := range plan.Entries {
		if entry.Action != "conflict" {
			t.Errorf("entry: %+v", entry)
		}
	}
}

func TestImportReconcile_UnchangedSharedOwnersMakeDestructiveChangesConflict(t *testing.T) {
	for _, c := range []struct {
		name, file string
		remove     bool
		wantCount  int
	}{
		{"delete skill", "SKILL.md", true, 2},
		{"update skill", "SKILL.md", false, 1},
		{"delete asset", "assets/guide.md", true, 1},
		{"update asset", "assets/guide.md", false, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := setupGitRepo(t)
			testutil.Chdir(t, dir)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [cursor]\n")
			for _, source := range []string{"left", "right"} {
				writeFile(t, source+"/example/SKILL.md", "original")
				writeFile(t, source+"/example/assets/guide.md", "original asset")
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-qm", "shared base")
			base := git(t, dir, "rev-parse", "HEAD")
			writeFile(t, ".agnostic-ai/skills/example/SKILL.md", "original")
			writeFile(t, ".agnostic-ai/skills/example/assets/guide.md", "original asset")
			git(t, dir, "rm", "-qr", "left", "right")
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-qm", "shared migration")
			migrated := git(t, dir, "rev-parse", "HEAD")
			git(t, dir, "checkout", "-q", base)
			changed := "left/example/" + c.file
			if c.remove {
				git(t, dir, "rm", "-q", changed)
			} else {
				writeFile(t, changed, "upstream change")
			}
			git(t, dir, "add", ".")
			git(t, dir, "commit", "-qm", "one owner changes")
			upstream := git(t, dir, "rev-parse", "HEAD")
			git(t, dir, "checkout", "-q", migrated)
			before := git(t, dir, "ls-files", "--stage")
			for _, mappings := range [][]string{{"left=.agnostic-ai/skills", "right=.agnostic-ai/skills"}, {"right=.agnostic-ai/skills", "left=.agnostic-ai/skills"}} {
				plan, err := planSkillReconciliation(base, migrated, upstream, mappings)
				if err != nil {
					t.Fatal(err)
				}
				if len(plan.Entries) != c.wantCount {
					t.Fatalf("shared owner entries = %+v, want %d", plan.Entries, c.wantCount)
				}
				for _, entry := range plan.Entries {
					if entry.Action != "conflict" || !strings.HasPrefix(entry.Source, "left/") {
						t.Errorf("unchanged right owner must prevent a destructive left plan: %+v", entry)
					}
				}
			}
			if after := git(t, dir, "ls-files", "--stage"); after != before {
				t.Error("shared-owner planning changed the index")
			}
			if status := git(t, dir, "status", "--porcelain"); status != "" {
				t.Errorf("shared-owner planning changed files: %s", status)
			}
		})
	}
}

func TestImportReconcile_NestedProjectUsesItsConfiguredTrees(t *testing.T) {
	dir := setupGitRepo(t)
	testutil.Chdir(t, dir)
	const project = "apps/demo project"
	writeFile(t, project+"/agnostic-ai.yaml", "version: 1\ntargets: [cursor]\nsources:\n  skills: specs/skills\n")
	writeFile(t, project+"/native/example/SKILL.md", "nested original")
	writeFile(t, "native/unrelated/SKILL.md", "root original")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "nested base and unrelated root")
	base := git(t, dir, "rev-parse", "HEAD")
	writeFile(t, project+"/specs/skills/example/SKILL.md", "nested original")
	writeFile(t, "specs/skills/unrelated/SKILL.md", "root original")
	git(t, dir, "rm", "-qr", project+"/native")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "nested migration")
	migrated := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", base)
	writeFile(t, project+"/native/example/SKILL.md", "nested update")
	writeFile(t, "native/unrelated/SKILL.md", "root update")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-qm", "nested and root changes")
	upstream := git(t, dir, "rev-parse", "HEAD")
	git(t, dir, "checkout", "-q", migrated)
	writeFile(t, "native/unrelated/SKILL.md", "uncommitted unrelated root change")
	before := git(t, dir, "status", "--porcelain")
	testutil.Chdir(t, filepath.Join(dir, project))
	plan, err := planSkillReconciliation(base, migrated, upstream, []string{"native=specs/skills"})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Entries) != 1 || plan.Entries[0] != (reconciliationEntry{"update", "native/example/SKILL.md", "specs/skills/example/SKILL.md"}) {
		t.Errorf("nested plan must use project-relative trees, excluding root changes: %+v", plan.Entries)
	}
	if after := git(t, dir, "status", "--porcelain"); after != before {
		t.Error("nested planning changed files")
	}
	writeFile(t, "specs/skills/example/SKILL.md", "uncommitted project change")
	if _, err := planSkillReconciliation(base, migrated, upstream, []string{"native=specs/skills"}); err == nil || !strings.Contains(err.Error(), "uncommitted") {
		t.Errorf("nested mapped dirty tree must still be rejected: %v", err)
	}
}
