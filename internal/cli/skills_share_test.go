package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// setupSharedSkillsFixture builds a project with one plain skill and a
// config enabling sync.shared-skills for codex + cursor, whose skill
// renderers produce byte-identical folders for a spec without
// per-target overrides.
func setupSharedSkillsFixture(t *testing.T, cfgYAML string) string {
	t.Helper()
	dir := setupFixture(t)
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(cfgYAML), 0o644))
	must(os.MkdirAll(filepath.Join(dir, ".agnostic-ai", "skills"), 0o755))
	must(os.WriteFile(filepath.Join(dir, ".agnostic-ai", "skills", "greet.md"),
		[]byte("---\nname: greet\ndescription: Say hi\n---\nGreet the user.\n"), 0o644))
	return dir
}

const sharedSkillsCfg = `version: 1
targets: [codex, cursor]
sync:
  shared-skills: true
`

func runSync(t *testing.T, args ...string) error {
	t.Helper()
	root := NewRootCmd("test")
	root.SetArgs(append([]string{"sync"}, args...))
	return root.Execute()
}

func TestSync_SharedSkills_LinksIdenticalTrees(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg)
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	canonical := filepath.Join(dir, ".agents", "skills", "greet", "SKILL.md")
	data, err := os.ReadFile(canonical)
	if err != nil {
		t.Fatalf("canonical skill file missing: %v", err)
	}
	if !strings.Contains(string(data), "Say hi") {
		t.Errorf("canonical SKILL.md should carry the description, got:\n%s", data)
	}

	link := filepath.Join(dir, ".cursor", "skills", "greet")
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("cursor skill folder missing: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s should be a symlink, got mode %v", link, fi.Mode())
	}
	target, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("..", "..", ".agents", "skills", "greet")
	if target != want {
		t.Errorf("link target = %q, want %q", target, want)
	}
	through, err := os.ReadFile(filepath.Join(link, "SKILL.md"))
	if err != nil {
		t.Fatalf("read through link: %v", err)
	}
	if string(through) != string(data) {
		t.Error("content read through link should match canonical bytes")
	}
}

func TestSync_SharedSkills_DivergentRenderKeepsCopies(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg)
	testutil.Chdir(t, dir)
	silence(t)

	// x-cursor meta reaches only the cursor render, so the two folders
	// are no longer byte-identical and must stay real copies.
	if err := os.WriteFile(filepath.Join(dir, ".agnostic-ai", "skills", "greet.md"),
		[]byte("---\nname: greet\ndescription: Say hi\nx-cursor:\n  disable-model-invocation: true\n---\nGreet the user.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{
		filepath.Join(dir, ".agents", "skills", "greet"),
		filepath.Join(dir, ".cursor", "skills", "greet"),
	} {
		fi, err := os.Lstat(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		if !fi.IsDir() {
			t.Errorf("%s should stay a real directory, got mode %v", p, fi.Mode())
		}
	}
}

func TestSync_SharedSkills_SecondSyncIdempotent(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg)
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	if err := runSync(t); err != nil {
		t.Fatalf("second sync: %v", err)
	}

	fi, err := os.Lstat(filepath.Join(dir, ".cursor", "skills", "greet"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("link should survive a second sync")
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check should be clean with links in place: %v", err)
	}
}

func TestSync_SharedSkills_RemovedSkillSweepsLinkAndCanonical(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg)
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai", "skills", "greet.md")); err != nil {
		t.Fatal(err)
	}
	if err := runSync(t); err != nil {
		t.Fatalf("sync after skill removal: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, ".agents", "skills", "greet")); !os.IsNotExist(err) {
		t.Errorf("canonical folder should be swept, stat err = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, ".cursor", "skills", "greet")); !os.IsNotExist(err) {
		t.Errorf("symlink should be swept with the skill, lstat err = %v", err)
	}
}

func TestSync_SharedSkills_DisableRestoresRealTrees(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg)
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"),
		[]byte("version: 1\ntargets: [codex, cursor]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runSync(t); err != nil {
		t.Fatalf("sync with shared-skills off: %v", err)
	}

	p := filepath.Join(dir, ".cursor", "skills", "greet")
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.IsDir() {
		t.Fatalf("%s should be a real directory again, got mode %v", p, fi.Mode())
	}
	if _, err := os.Stat(filepath.Join(p, "SKILL.md")); err != nil {
		t.Errorf("real SKILL.md should be re-emitted: %v", err)
	}
}

func TestSync_SharedSkills_DryRunTouchesNothing(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg)
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t, "--dry-run"); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".agents", ".cursor"} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Errorf("dry-run should not create %s, stat err = %v", p, err)
		}
	}
}

func TestSync_SharedSkills_HandAuthoredSiblingSurvives(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg)
	testutil.Chdir(t, dir)
	silence(t)

	mine := filepath.Join(dir, ".cursor", "skills", "mine")
	if err := os.MkdirAll(mine, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mine, "SKILL.md"),
		[]byte("---\nname: mine\n---\nhand-authored\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(mine, "SKILL.md"))
	if err != nil {
		t.Fatalf("hand-authored skill should survive: %v", err)
	}
	if !strings.Contains(string(data), "hand-authored") {
		t.Error("hand-authored skill content should be untouched")
	}
	fi, err := os.Lstat(filepath.Join(dir, ".cursor", "skills", "greet"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("managed sibling should still be linked")
	}
}

func TestSync_SharedSkills_OffByDefault(t *testing.T) {
	dir := setupSharedSkillsFixture(t, "version: 1\ntargets: [codex, cursor]\n")
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, ".cursor", "skills", "greet")
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	if !fi.IsDir() {
		t.Errorf("without opt-in %s should be a real directory, got mode %v", p, fi.Mode())
	}
}

func TestSync_SharedSkills_PartialSyncKeepsLinks(t *testing.T) {
	dir := setupSharedSkillsFixture(t, sharedSkillsCfg)
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	// A partial run that skips the canonical owner must not tear the
	// link down: cursor would end up with no skills at all.
	if err := runSync(t, "-t", "cursor"); err != nil {
		t.Fatalf("partial sync: %v", err)
	}

	fi, err := os.Lstat(filepath.Join(dir, ".cursor", "skills", "greet"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Error("partial sync should keep the existing link")
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents", "skills", "greet", "SKILL.md")); err != nil {
		t.Errorf("canonical tree should survive a partial sync: %v", err)
	}
}

func TestPlanSkillLinks_GroupsByFingerprintAndPrefersAgentsSkills(t *testing.T) {
	captures := []targetCapture{
		{target: "claude", files: []adapters.CapturedFile{
			{Path: filepath.Join(".claude", "skills", "greet", "SKILL.md"), Content: "claude-flavored"},
		}},
		{target: "codex", files: []adapters.CapturedFile{
			{Path: filepath.Join(".agents", "skills", "greet", "SKILL.md"), Content: "shared"},
		}},
		{target: "cursor", files: []adapters.CapturedFile{
			{Path: filepath.Join(".cursor", "skills", "greet", "SKILL.md"), Content: "shared"},
		}},
	}

	links := planSkillLinks(captures)
	if len(links) != 1 {
		t.Fatalf("expected exactly one link (cursor -> agents), got %+v", links)
	}
	if links[0].path != filepath.Join(".cursor", "skills", "greet") {
		t.Errorf("link path = %q", links[0].path)
	}
	if links[0].canonical != filepath.Join(".agents", "skills", "greet") {
		t.Errorf("canonical = %q, want the .agents/skills tree preferred", links[0].canonical)
	}
}

func TestPlanSkillLinks_NoLinkForSingleOrDivergentFolders(t *testing.T) {
	captures := []targetCapture{
		{target: "codex", files: []adapters.CapturedFile{
			{Path: filepath.Join(".agents", "skills", "solo", "SKILL.md"), Content: "a"},
		}},
		{target: "cursor", files: []adapters.CapturedFile{
			{Path: filepath.Join(".cursor", "skills", "solo", "SKILL.md"), Content: "b"},
		}},
	}
	if links := planSkillLinks(captures); len(links) != 0 {
		t.Errorf("divergent folders must not link, got %+v", links)
	}
}
