package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestNewScopeLocal_WritesANewSpecUnderLocal(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	if _, _, err := runSplit(t, "new", "rule", "scratch", "--scope", "local"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(".agnostic-ai/local/rules/scratch.md")
	if err != nil {
		t.Fatal(err)
	}
	if want := newSpecTemplate("rule", "scratch"); string(body) != want {
		t.Errorf("a new local name must get the full scaffold, got:\n%s", body)
	}
	if _, err := os.Stat(".agnostic-ai/rules/scratch.md"); err == nil {
		t.Error("--scope local wrote a shared rule")
	}
}

func TestNewScopeLocal_OverrideOfASharedSpecStartsWithItsName(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	if _, _, err := runSplit(t, "new", "rule", "style"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runSplit(t, "new", "hook", "fmt"); err != nil {
		t.Fatal(err)
	}
	shared, _ := os.ReadFile(".agnostic-ai/rules/style.md")

	for _, tc := range []struct{ kind, name, path, want string }{
		{"rule", "style", ".agnostic-ai/local/rules/style.md", "---\n# Local override: write only the fields you change. The shared spec keeps the rest.\nname: style\n---\n"},
		{"hook", "fmt", ".agnostic-ai/local/hooks/fmt.yaml", "# Local override: write only the fields you change. The shared spec keeps the rest.\nname: fmt\n"},
	} {
		if _, _, err := runSplit(t, "new", tc.kind, tc.name, "--scope", "local"); err != nil {
			t.Fatalf("%s: %v", tc.kind, err)
		}
		body, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != tc.want {
			t.Errorf("%s override:\n%s\nwant:\n%s", tc.kind, body, tc.want)
		}
	}
	if after, _ := os.ReadFile(".agnostic-ai/rules/style.md"); string(after) != string(shared) {
		t.Error("--scope local changed the shared rule")
	}
}

// The override must load: sync keeps the shared fields and body.
func TestNewScopeLocal_OverrideSyncsOnTopOfTheSharedSpec(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	writeFile(t, ".agnostic-ai/rules/style.md", "---\nname: style\ndescription: Team style.\nalwaysApply: true\n---\n\nShared body.\n")
	if _, _, err := runSplit(t, "new", "rule", "style", "--scope", "local"); err != nil {
		t.Fatal(err)
	}
	if out, errOut, err := runSplit(t, "sync", "--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s\n%s", err, out, errOut)
	}
	out, _, err := runSplit(t, "list")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "rule\tstyle\t"+layerNameProjectUser) {
		t.Errorf("list must show the local layer winning:\n%s", out)
	}
}

func TestNewScopeLocal_SkillFolderIsOverriddenByTheSameFolder(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	writeFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review.\n---\n\nBody.\n")
	if _, _, err := runSplit(t, "new", "skill", "review", "--scope", "local"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(".agnostic-ai/local/skills/review/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "name: review") || strings.Contains(string(body), "description:") {
		t.Errorf("unexpected override: %s", body)
	}
}

// A local spec merges into a shared one by name, wherever the shared
// file sits. A full scaffold there would replace the team's rule with
// placeholders, so the override must be found by name.
func TestNewScopeLocal_FindsTheSharedSpecByNameNotByPath(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	writeFile(t, ".agnostic-ai/rules/backend/style.md", "---\nname: style\ndescription: Backend style.\nglobs: \"src/**/*.go\"\n---\n\nUse tabs.\n")
	writeFile(t, ".agnostic-ai/agents/code-reviewer.md", "---\nname: reviewer\ndescription: Reviews.\n---\n\nReview.\n")
	if out, errOut, err := runSplit(t, "sync", "--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s\n%s", err, out, errOut)
	}
	before, err := os.ReadFile(".claude/rules/style.md")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ kind, name, path string }{
		{"rule", "style", ".agnostic-ai/local/rules/backend/style.md"},
		{"agent", "reviewer", ".agnostic-ai/local/agents/code-reviewer.md"},
	} {
		if _, _, err := runSplit(t, "new", tc.kind, tc.name, "--scope", "local"); err != nil {
			t.Fatalf("%s: %v", tc.kind, err)
		}
		body, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatalf("%s: want the override beside where the shared spec sits: %v", tc.kind, err)
		}
		if !strings.Contains(string(body), "name: "+tc.name) || strings.Contains(string(body), "TODO") {
			t.Errorf("%s: want a name-only override, got:\n%s", tc.kind, body)
		}
	}
	if out, errOut, err := runSplit(t, "sync", "--only", "claude"); err != nil {
		t.Fatalf("sync: %v\n%s\n%s", err, out, errOut)
	}
	if after, _ := os.ReadFile(".claude/rules/style.md"); string(after) != string(before) {
		t.Errorf("an untouched override changed the emitted rule:\n%s\nwas:\n%s", after, before)
	}
}

// A file named after a spec that has another name is not that spec.
func TestNewScopeLocal_AFileStemIsNotASpecName(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	writeFile(t, ".agnostic-ai/agents/code-reviewer.md", "---\nname: reviewer\ndescription: Reviews.\n---\n\nReview.\n")
	if _, _, err := runSplit(t, "new", "agent", "code-reviewer", "--scope", "local"); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(".agnostic-ai/local/agents/code-reviewer.md")
	if string(body) != newSpecTemplate("agent", "code-reviewer") {
		t.Errorf("no shared agent has this name, so it is a new spec with the full scaffold, got:\n%s", body)
	}
}

func TestNewScopeLocal_RefusesASecondLocalSpecOfTheSameName(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	writeFile(t, ".agnostic-ai/skills/dep/SKILL.md", "---\nname: dep\ndescription: Dep.\n---\n\nBody.\n")
	writeFile(t, ".agnostic-ai/local/skills/dep.md", "---\nname: dep\n---\n")
	_, _, err := runSplit(t, "new", "skill", "dep", "--scope", "local")
	if err == nil || !strings.Contains(err.Error(), ".agnostic-ai/local/skills/dep.md already exists") {
		t.Errorf("err=%v, want the existing local skill named", err)
	}
	if _, statErr := os.Stat(".agnostic-ai/local/skills/dep/SKILL.md"); statErr == nil {
		t.Error("a second local skill of the same name was written")
	}
}

func TestNewScopeLocal_NeverOverwritesAndDryRunWritesNothing(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	if _, _, err := runSplit(t, "new", "rule", "a", "--scope", "local", "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(".agnostic-ai/local"); err == nil {
		t.Error("--dry-run created the local folder")
	}
	writeFile(t, ".agnostic-ai/local/rules/a.md", "mine\n")
	if _, _, err := runSplit(t, "new", "rule", "a", "--scope", "local"); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("want an existing local spec refused, got %v", err)
	}
	if body, _ := os.ReadFile(".agnostic-ai/local/rules/a.md"); string(body) != "mine\n" {
		t.Errorf("existing local spec was overwritten: %s", body)
	}
}

func TestNewScopeLocal_WarnsWhenGitWouldCommitThePersonalLayer(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	isolateGit(t)
	gitInit(t, dir)
	_, stderr, err := runSplit(t, "new", "rule", "a", "--scope", "local")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr, ".agnostic-ai/local/") || !strings.Contains(stderr, ".gitignore") {
		t.Errorf("stderr=%q, want a warning that Git does not ignore the local folder", stderr)
	}

	writeFile(t, ".gitignore", "/.agnostic-ai/local/\nagnostic-ai.local.yaml\n")
	_, stderr, err = runSplit(t, "new", "rule", "b", "--scope", "local")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stderr, "gitignore") {
		t.Errorf("ignored local folder still warned: %q", stderr)
	}
}

func TestNewScopeUser_WritesToTheUserHomeFromAnyDirectory(t *testing.T) {
	_, source := globalAgentTestHome(t)
	testutil.Chdir(t, t.TempDir())
	summary := captureSummary(t)
	if _, _, err := runSplit(t, "new", "rule", "my-style", "--scope", "user"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(source, "rules", "my-style.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "name: my-style") || strings.Contains(string(body), "globs:") || strings.Contains(string(body), "alwaysApply:") {
		t.Errorf("a user rule applies everywhere, so it carries no condition: %s", body)
	}
	if !strings.Contains(summary.String(), "agnostic-ai sync --scope user") {
		t.Errorf("the summary must name the user-level sync:\n%s", summary)
	}
	if entries, _ := os.ReadDir("."); len(entries) != 0 {
		t.Errorf("--scope user wrote into the working directory: %v", entries)
	}
}

func TestNewScopeUser_TakesOnlyTheKindsUserSyncLoads(t *testing.T) {
	_, source := globalAgentTestHome(t)
	testutil.Chdir(t, t.TempDir())
	for _, kind := range []string{"command", "review", "environment", "ignore"} {
		_, _, err := runSplit(t, "new", kind, "x", "--scope", "user")
		if err == nil || !strings.Contains(err.Error(), "--scope user has no "+kind+" specs") || !strings.Contains(err.Error(), "agent, skill, rule, hook, mcp, settings") {
			t.Errorf("%s: err=%v, want the kind refused with the supported list", kind, err)
		}
	}
	for _, kind := range []string{"agent", "skill", "rule", "hook", "mcp", "settings"} {
		if _, _, err := runSplit(t, "new", kind, "ok", "--scope", "user", "--dry-run"); err != nil {
			t.Errorf("%s: %v", kind, err)
		}
	}
	if _, err := os.Stat(source); err == nil {
		if entries, _ := os.ReadDir(source); len(entries) != 0 {
			t.Errorf("--dry-run wrote into the home: %v", entries)
		}
	}
}

func TestNewScopeUser_RejectsARuleDirectory(t *testing.T) {
	globalAgentTestHome(t)
	testutil.Chdir(t, t.TempDir())
	if _, _, err := runSplit(t, "new", "rule", "x", "--scope", "user", "--dir", "services"); err == nil || !strings.Contains(err.Error(), "--dir cannot be combined with --scope user") {
		t.Errorf("want --dir refused at user scope, got %v", err)
	}
}

// A new user spec must pass the checks the user-level sync runs.
func TestNewScopeUser_ScaffoldPassesUserValidation(t *testing.T) {
	globalConfigTestHome(t)
	testutil.Chdir(t, t.TempDir())
	silence(t)
	if _, _, err := runSplit(t, "new", "rule", "my-style", "--scope", "user"); err != nil {
		t.Fatal(err)
	}
	if out, errOut, err := runSplit(t, "validate", "--scope", "user"); err != nil {
		t.Errorf("validate --scope user: %v\n%s\n%s", err, out, errOut)
	}
}

func TestNewScope_ProjectIsTheDefaultAndStillRefusesTheUserHome(t *testing.T) {
	_, source := globalAgentTestHome(t)
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, source)
	for _, args := range [][]string{{"new", "rule", "x"}, {"new", "rule", "x", "--scope", "project"}, {"new", "rule", "x", "--scope", "local"}} {
		if _, _, err := runSplit(t, args...); err == nil || !strings.Contains(err.Error(), "global home") || !strings.Contains(err.Error(), "new <kind> <name> --scope user") {
			t.Errorf("%v inside the user home: err=%v, want the refusal to name new --scope user", args, err)
		}
	}
}

func TestNewScope_HelpAndCompletionOfferTheThreeScopes(t *testing.T) {
	out, _, err := runSplit(t, "new", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--scope", "--dir", "local", "user"} {
		if !strings.Contains(out, want) {
			t.Errorf("help lacks %q:\n%s", want, out)
		}
	}
	out, _, err = runSplit(t, "__completeNoDesc", "new", "rule", "x", "--scope", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{specScopeProject, specScopeLocal, specScopeUser} {
		if !strings.Contains(out, want+"\n") {
			t.Errorf("completion %q lacks %q", out, want)
		}
	}
}

func TestDoctor_WarnsWhenGitWouldCommitThePersonalLayer(t *testing.T) {
	dir := setupEmptyProject(t)
	testutil.Chdir(t, dir)
	silence(t)
	isolateGit(t)
	gitInit(t, dir)
	const marker = "Git would commit"

	_, stderr, _ := runSplit(t, "doctor")
	if strings.Contains(stderr, marker) {
		t.Errorf("no personal layer, yet doctor warned: %q", stderr)
	}
	writeFile(t, ".agnostic-ai/local/rules/mine.md", "---\nname: mine\ndescription: Mine.\nalwaysApply: true\n---\n\nMine.\n")
	_, stderr, _ = runSplit(t, "doctor")
	if !strings.Contains(stderr, marker) || !strings.Contains(stderr, ".agnostic-ai/local/") {
		t.Errorf("stderr=%q, want the personal layer named", stderr)
	}
	if _, stderr, _ = runSplit(t, "doctor", "--checks", "project"); strings.Contains(stderr, marker) {
		t.Errorf("--checks project warned about personal files: %q", stderr)
	}
	stdout, stderr, _ := runSplit(t, "doctor", "--json")
	if strings.Contains(stdout+stderr, marker) {
		t.Errorf("--json must stay unchanged: %q %q", stdout, stderr)
	}
	writeFile(t, ".gitignore", "/.agnostic-ai/local/\nagnostic-ai.local.yaml\n")
	if _, stderr, _ = runSplit(t, "doctor"); strings.Contains(stderr, marker) {
		t.Errorf("ignored personal layer still warned: %q", stderr)
	}
}

func TestDoctor_ScopeWithASpecScopeSaysItIsTheOlderSpellingOfChecks(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	for _, scope := range []string{"user", "local", "runtime"} {
		_, err := runDoctor(t, "--scope", scope)
		if err == nil || !strings.Contains(err.Error(), "older spelling of --checks") || !strings.Contains(err.Error(), "all or project") {
			t.Errorf("doctor --scope %s: err=%v", scope, err)
		}
	}
}

// A pack spec in a subfolder is overridden in the same subfolder, as a
// project spec is, so the override keeps the rule's directory.
func TestNewScopeLocal_OverrideOfAPackSpecKeepsItsSubfolder(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	writeFile(t, "agnostic.packs.lock", "version: 1\npacks:\n  - name: team\n")
	writeFile(t, ".agnostic-ai/packs/team/rules/svc/packrule.md", "---\nname: packrule\ndescription: Pack rule.\nalwaysApply: true\n---\n\nFrom the pack.\n")
	if _, _, err := runSplit(t, "new", "rule", "packrule", "--scope", "local"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(".agnostic-ai/local/rules/svc/packrule.md")
	if err != nil {
		t.Fatalf("want the override in the pack rule's subfolder: %v", err)
	}
	if strings.Contains(string(body), "TODO") {
		t.Errorf("want a name-only override, got:\n%s", body)
	}
}

// A listed pack that is not installed hides its specs. A scaffold over
// one of their names would replace the pack's spec once it is installed.
func TestNewScopeLocal_RefusesWhileAPackIsMissing(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	writeFile(t, "agnostic.packs.lock", "version: 1\npacks:\n  - name: team\n")
	if _, _, err := runSplit(t, "new", "skill", "deep", "--scope", "local"); err == nil || !strings.Contains(err.Error(), "team") {
		t.Errorf("err=%v, want the missing pack named", err)
	}
	if _, err := os.Stat(".agnostic-ai/local"); err == nil {
		t.Error("a local spec was written while a pack was missing")
	}
	if _, _, err := runSplit(t, "new", "skill", "deep"); err != nil {
		t.Errorf("a project spec does not depend on the pack: %v", err)
	}
}

func TestNewScopeLocal_SaysWhyABrokenSpecStopsIt(t *testing.T) {
	testutil.Chdir(t, setupEmptyProject(t))
	silence(t)
	writeFile(t, ".agnostic-ai/rules/bad.md", "---\nname: [bad\n---\n")
	_, _, err := runSplit(t, "new", "rule", "mine", "--scope", "local")
	if err == nil || !strings.Contains(err.Error(), "reads the project's specs") || !strings.Contains(err.Error(), "bad.md") {
		t.Errorf("err=%v, want the reason and the broken file", err)
	}
}
