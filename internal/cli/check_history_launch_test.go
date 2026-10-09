package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestHistoryDefaultAbsence_HandwrittenFilesSkipRenderWithoutChangingClassification(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	writeBenchProject(t, dir, 3)
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	mustWriteFile(t, ".gitignore", "# >>> agnostic-ai (managed) >>>\n.claude/settings.json\n# <<< agnostic-ai (managed) <<<\n")
	git("add", "-A")
	git("commit", "-q", "-m", "manual launch")
	cfg, err := config.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	emitted := map[string]bool{}
	got := unledgeredReport(cfg, emitted, syncStateFile{}, strandedOutput(cfg, emitted, syncStateFile{}))
	if len(got.Leftover) != 0 || len(got.Orphaned) != 0 {
		t.Errorf("manual launch classified as generated: %+v", got)
	}
	h := &historyRenderer{sources: configuredSources(cfg)}
	if h.provesAtHEAD(filepath.FromSlash(".claude/launch.json")) || h.proves(filepath.FromSlash(".claude/launch.json")) {
		t.Error("manual launch proved owned")
	}
	if h.provesAtHEAD(filepath.FromSlash(".gitignore")) || h.proves(filepath.FromSlash(".gitignore")) {
		t.Error("managed ignore block proved captured output")
	}
	if len(h.renders) != 0 {
		t.Errorf("full renders=%d, want zero", len(h.renders))
	}
	if len(h.absentDefaultOutputs) != 1 {
		t.Errorf("absence cache commits=%d, want one", len(h.absentDefaultOutputs))
	}
	if len(h.admitted) != 1 {
		t.Errorf("history admissions=%d, want one", len(h.admitted))
	}
	full, err := plannedOutputs()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{".claude/launch.json", ".gitignore"} {
		if _, present := full[key]; present {
			t.Errorf("full renderer emits %s", key)
		}
	}
	const other = ".claude/settings.json"
	content, present := full[other]
	if !present {
		t.Fatal("full renderer has no settings output")
	}
	mustWriteFile(t, other, content)
	if !h.provesAtHEAD(filepath.FromSlash(other)) {
		t.Error("other JSON no longer proved owned after negative results")
	}
	if len(h.renders) != 1 {
		t.Errorf("other JSON full renders=%d, want one", len(h.renders))
	}
	if h.renders[h.head][other] != content {
		t.Error("historical settings bytes differ from ordinary renderer")
	}
	ignore, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatal(err)
	}
	h.renders[h.head][".gitignore"] = string(ignore)
	if !h.provesAtHEAD(filepath.FromSlash(".gitignore")) {
		t.Error("cached full map did not take precedence over cached absence")
	}
	lexical := &historyRenderer{sources: configuredSources(cfg)}
	if lexical.provesAtHEAD("./.gitignore") || len(lexical.renders) != 1 || len(lexical.absentDefaultOutputs) != 0 {
		t.Error("non-exact path used absence proof or changed ownership")
	}
}

func TestHistoryDefaultAbsence_UnknownInputsKeepFullRender(t *testing.T) {
	for _, tc := range []struct{ name, config, path, body string }{
		{"environment", "", ".agnostic-ai/environments/dev.yaml", "name: dev\n"},
		{"local environment", "", ".agnostic-ai/local/environments/dev.yaml", "name: dev\n"},
		{"helper", "", ".agnostic-ai/overlays/claude/launch.json", "{}\n"},
		{"pack", "", ".agnostic-ai/packs/demo/environments/dev.yaml", "name: dev\n"},
		{"pack lock", "", "agnostic.packs.lock", "{}\n"},
		{"local config", "", "agnostic-ai.local.yaml", "version: 1\n"},
		{"output alias", "outputs: {aider: {file: .claude/launch.json}}\n", "", ""},
		{"custom source", "sources: {environments: custom}\n", "", ""},
		{"builtin", "builtins: [handoff]\n", "", ""},
		{"unknown target", "targets: [unknown-target]\n", "", ""},
		{"unicode", "", ".agnostic-ai/rules/müller.md", "hello\n"},
		{"case alias", "", ".agnostic-ai/Environments/dev.yaml", "name: dev\n"},
		{"trailing dot alias", "", ".agnostic-ai/environments./dev.yaml", "name: dev\n"},
		{"short name ambiguity", "", ".agnostic-ai/skills/demo~/SKILL.md", "skill\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, git := launchGitRepo(t)
			testutil.Chdir(t, dir)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\n"+tc.config)
			mustWriteFile(t, ".claude/launch.json", "{}\n")
			if tc.path != "" {
				mustWriteFile(t, tc.path, tc.body)
			}
			git("add", "-A")
			git("commit", "-q", "-m", "guard")
			for _, key := range []string{".claude/launch.json", ".gitignore"} {
				h := &historyRenderer{}
				h.provesAtHEAD(filepath.FromSlash(key))
				if len(h.renders) != 1 {
					t.Errorf("%s full renders=%d, want one", key, len(h.renders))
				}
			}
		})
	}
}

func TestHistoryLaunch_DeletedEnvironmentKeepsHistoricalOwnership(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\ngitignore: {enabled: false}\n")
	mustWriteFile(t, ".agnostic-ai/environments/dev.yaml", "name: dev\ndev-commands:\n  - name: web\n    command: npm run dev\n")
	syncProject(t)
	git("add", "-A")
	git("commit", "-q", "-m", "generated launch")
	if err := os.Remove(".agnostic-ai/environments/dev.yaml"); err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "delete environment")
	h := &historyRenderer{}
	if h.provesAtHEAD(filepath.FromSlash(".claude/launch.json")) {
		t.Error("HEAD still owns deleted environment")
	}
	if !h.proves(filepath.FromSlash(".claude/launch.json")) {
		t.Error("old environment no longer proves launch")
	}
	if len(h.renders) != 1 {
		t.Errorf("full renders=%d, want one", len(h.renders))
	}
	if err := os.Remove(stateFilePath(".")); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	rep := unledgeredReport(cfg, map[string]bool{}, syncStateFile{}, func(p string) bool { return filepath.ToSlash(p) == ".claude/launch.json" })
	if !reflect.DeepEqual(rep.Orphaned, []string{filepath.FromSlash(".claude/launch.json")}) || len(rep.Leftover) != 0 {
		t.Errorf("deleted environment classification: %+v", rep)
	}
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	if h.proves(filepath.FromSlash(".claude/launch.json")) {
		t.Error("edited launch remains owned")
	}
}

func TestHistoryDefaultAbsence_DefaultProducersCannotEmitEitherPath(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeBenchProject(t, dir, 1)
	mustWriteFile(t, ".agnostic-ai/ignore/secrets.md", "---\nname: secrets\n---\n\n*.env\n")
	mustWriteFile(t, ".agnostic-ai/reviews/review.md", "---\nname: review\n---\n\nCheck changes.\n")
	mustWriteFile(t, ".agnostic-ai/settings/policy.yaml", "name: policy\npermissions:\n  allow: [read]\n")
	cfg, loaded, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	b := spec.NewBundle(append(loaded.All(), []spec.Entry{
		{Kind: spec.KindAgent, Name: "launch", Body: "agent"},
		{Kind: spec.KindSkill, Name: "launch", Body: "skill"},
		{Kind: spec.KindRule, Name: "launch", Body: "rule"},
		{Kind: spec.KindCommand, Name: "launch", Body: "command"},
	}...))
	for _, name := range adapters.Names() {
		a, _ := adapters.Get(name)
		files, err := captureAdapterFiles(adapters.NewSession(), a, b, cfg)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, f := range files {
			key := filepath.ToSlash(f.Path)
			if key == ".claude/launch.json" || key == ".gitignore" {
				t.Errorf("%s emitted %s under default configuration", name, key)
			}
		}
	}
	files, err := plannedOutputs()
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{".claude/launch.json", ".gitignore"} {
		if _, present := files[key]; present {
			t.Errorf("planned output includes %s", key)
		}
	}
	if _, present := files[".aiderignore"]; !present {
		t.Error("native ignore output missing")
	}
}

func TestHistoryLaunch_NestedProjectIgnoresSiblingEnvironment(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, ".agnostic-ai/environments/dev.yaml", "name: sibling\n")
	mustWriteFile(t, "nested/agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "nested/.claude/launch.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "nested manual")
	testutil.Chdir(t, filepath.Join(dir, "nested"))
	h := &historyRenderer{}
	h.provesAtHEAD(filepath.FromSlash(".claude/launch.json"))
	if !reflect.DeepEqual(h.renders, map[string]map[string]string{}) {
		t.Errorf("nested project rendered: %v", h.renders)
	}
}

func TestHistoryLaunch_NegativeProofStillConsumesHistoricalAdmission(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	var paths, commits []string
	for i := 0; i < maxHistoryRenders; i++ {
		path := fmt.Sprintf(".claude/other-%d.json", i)
		mustWriteFile(t, path, "{}\n")
		git("add", "-A")
		git("commit", "-q", "-m", "other")
		sha, err := gitOutput(".", nil, "rev-parse", "HEAD")
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
		commits = append(commits, strings.TrimSpace(sha))
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "manual")
	h := &historyRenderer{}
	if h.proves(filepath.FromSlash(".claude/launch.json")) {
		t.Error("manual file owned")
	}
	if len(h.renders) != 0 || len(h.admitted) != 1 {
		t.Errorf("renders=%d admissions=%d", len(h.renders), len(h.admitted))
	}

	for i, commit := range commits {
		h.renders[commit] = map[string]string{paths[i]: "{}\n"}
		if got := h.proves(paths[i]); got != (i < maxHistoryRenders-1) {
			t.Errorf("candidate %d proof=%v", i, got)
		}
	}
	mustWriteFile(t, ".claude/other.json", "{}\n")
	// A later HEAD must not enter the pinned snapshot or consume its budget.
	git("add", "-A")
	git("commit", "-q", "-m", "later")
	if h.proves(".claude/other.json") {
		t.Error("later file entered pinned history")
	}
	if len(h.admitted) != maxHistoryRenders {
		t.Errorf("admissions=%d", len(h.admitted))
	}
	if h.provesAtHEAD(filepath.FromSlash(".claude/launch.json")) || len(h.renders) != maxHistoryRenders {
		t.Error("pinned negative proof changed")
	}
}

func TestHistoryLaunch_LegacyAndSourceAliasKeepFullRender(t *testing.T) {
	for _, alias := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "symlink"}[alias], func(t *testing.T) {
			dir, git := launchGitRepo(t)
			testutil.Chdir(t, dir)
			configName := config.LegacyConfigFileName
			if alias {
				configName = config.ConfigFileName
			}
			mustWriteFile(t, configName, "version: 1\ntargets: [claude]\n")
			mustWriteFile(t, ".claude/launch.json", "{}\n")
			if alias {
				mustWriteFile(t, "outside/dev.yaml", "name: dev\n")
				if err := os.MkdirAll(".agnostic-ai", 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("../outside", ".agnostic-ai/environments"); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
			}
			git("add", "-A")
			git("commit", "-q", "-m", "fallback")
			h := &historyRenderer{}
			h.provesAtHEAD(filepath.FromSlash(".claude/launch.json"))
			if len(h.renders) != 1 {
				t.Errorf("full renders=%d, want one", len(h.renders))
			}
		})
	}
}

func TestHistoryLaunch_CustomGitFilterKeepsFullRender(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	git("config", "filter.unused.smudge", "cat")
	git("add", "-A")
	git("commit", "-q", "-m", "filter")
	h := &historyRenderer{}
	h.provesAtHEAD(filepath.FromSlash(".claude/launch.json"))
	if len(h.renders) != 1 {
		t.Errorf("full renders=%d, want one", len(h.renders))
	}
}

func TestHistoryLaunch_RestoredHelperRetainsOwnership(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/overlays/claude/launch.json", "{}\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "helper")
	h := &historyRenderer{}
	if !h.provesAtHEAD(filepath.FromSlash(".claude/launch.json")) {
		t.Error("restored helper ownership lost")
	}
}

func TestHistoryLaunch_UnrelatedHelperDoesNotDefeatAbsence(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/overlays/claude/readme.txt", "helper\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "manual")
	h := &historyRenderer{}
	h.provesAtHEAD(filepath.FromSlash(".claude/launch.json"))
	if len(h.renders) != 0 {
		t.Errorf("full renders=%d, want zero", len(h.renders))
	}
}

func TestHistoryLaunch_EscapingSkillNameCannotProduceLaunchAsset(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/skills/demo/SKILL.md", "---\nname: ../../.claude\n---\nskill\n")
	mustWriteFile(t, ".agnostic-ai/skills/demo/launch.json", "{}\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	if _, _, err := loadProject("."); err == nil || !strings.Contains(err.Error(), "invalid spec name") {
		t.Fatalf("escaping skill loader error=%v", err)
	}
	git("add", "-A")
	git("commit", "-q", "-m", "invalid skill")
	h := &historyRenderer{}
	if h.provesAtHEAD(filepath.FromSlash(".claude/launch.json")) || h.proves(filepath.FromSlash(".claude/launch.json")) {
		t.Error("invalid skill established ownership")
	}
}

func TestHistoryLaunch_ValidScopedSkillAssetDoesNotOwnRootLaunch(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/skills/.claude/demo/SKILL.md", "---\nname: demo\n---\nskill\n")
	mustWriteFile(t, ".agnostic-ai/skills/.claude/demo/launch.json", "{}\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "scoped skill")
	h := &historyRenderer{}
	if !h.prepare() {
		t.Fatal("history not prepared")
	}
	full := h.render(h.head)
	if full == nil {
		t.Fatal("valid scoped skill failed historical rendering")
	}
	if _, present := full[".claude/launch.json"]; present {
		t.Error("scoped skill asset owns root launch")
	}
	if h.provesAtHEAD(filepath.FromSlash(".claude/launch.json")) || len(h.renders) != 0 {
		t.Error("scoped skill asset defeated exact-key absence proof")
	}
}

func TestHistoryLaunch_CurrentCustomSourceKeepsFullRender(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "manual")
	h := &historyRenderer{sources: []string{"custom"}}
	h.provesAtHEAD(filepath.FromSlash(".claude/launch.json"))
	if len(h.renders) != 1 {
		t.Errorf("full renders=%d, want one", len(h.renders))
	}
}

func launchGitRepo(t *testing.T) (string, func(...string)) {
	t.Helper()
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "config"))
	t.Setenv("GIT_CONFIG_COUNT", "0")
	return gitRepo(t)
}

func TestHistoryDefaultAbsence_ConfiguredIgnoreOutputKeepsOwnership(t *testing.T) {
	dir, git := launchGitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [aider]\ngitignore: {enabled: false}\noutputs:\n  aider:\n    ignore-file: .gitignore\n    provenance-header: false\n")
	mustWriteFile(t, ".agnostic-ai/ignore/secrets.md", "---\nname: secrets\n---\n\n*.env\n")
	syncProject(t)
	git("add", "-A")
	git("commit", "-q", "-m", "configured ignore output")
	if err := os.Remove(stateFilePath(".")); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [aider]\ngitignore: {enabled: false}\n")
	cfg, err := config.Load(".")
	if err != nil {
		t.Fatal(err)
	}
	h := &historyRenderer{sources: configuredSources(cfg)}
	if !h.provesAtHEAD(filepath.FromSlash(".gitignore")) {
		t.Error("configured ignore not owned by HEAD")
	}
	if len(h.renders) != 1 {
		t.Errorf("full renders=%d, want one", len(h.renders))
	}
	rep := unledgeredReport(cfg, map[string]bool{}, syncStateFile{}, func(p string) bool { return filepath.ToSlash(p) == ".gitignore" })
	if !reflect.DeepEqual(rep.Leftover, []string{filepath.FromSlash(".gitignore")}) || len(rep.Orphaned) != 0 {
		t.Errorf("configured ignore classification: %+v", rep)
	}
	mustWriteFile(t, ".gitignore", "my own patterns\n")
	if h.provesAtHEAD(filepath.FromSlash(".gitignore")) {
		t.Error("edited ignore remains owned")
	}
	rep = unledgeredReport(cfg, map[string]bool{}, syncStateFile{}, func(p string) bool { return filepath.ToSlash(p) == ".gitignore" })
	if rep.hasDrift() {
		t.Errorf("edited ignore classified as generated: %+v", rep)
	}
}
