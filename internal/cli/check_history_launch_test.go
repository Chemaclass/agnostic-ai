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

func TestHistoryLaunch_HandwrittenFileSkipsRenderWithoutChangingClassification(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	writeBenchProject(t, dir, 3)
	mustWriteFile(t, ".claude/launch.json", "{}\n")
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
	if h.provesAtHEAD(".claude/launch.json") || h.proves(".claude/launch.json") {
		t.Error("manual launch proved owned")
	}
	if len(h.renders) != 0 {
		t.Errorf("full renders=%d, want zero", len(h.renders))
	}
	if len(h.admitted) != 1 {
		t.Errorf("history admissions=%d, want one", len(h.admitted))
	}
	if h.provesAtHEAD(".claude/other.json") {
		t.Error("missing other JSON proved owned")
	}
	if len(h.renders) != 1 {
		t.Errorf("other JSON full renders=%d, want one", len(h.renders))
	}
}

func TestHistoryLaunch_UnknownInputsKeepFullRender(t *testing.T) {
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
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, git := gitRepo(t)
			testutil.Chdir(t, dir)
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\n"+tc.config)
			mustWriteFile(t, ".claude/launch.json", "{}\n")
			if tc.path != "" {
				mustWriteFile(t, tc.path, tc.body)
			}
			git("add", "-A")
			git("commit", "-q", "-m", "guard")
			h := &historyRenderer{}
			h.provesAtHEAD(".claude/launch.json")
			if len(h.renders) != 1 {
				t.Errorf("full renders=%d, want one", len(h.renders))
			}
		})
	}
}

func TestHistoryLaunch_DeletedEnvironmentKeepsHistoricalOwnership(t *testing.T) {
	dir, git := gitRepo(t)
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
	if h.provesAtHEAD(".claude/launch.json") {
		t.Error("HEAD still owns deleted environment")
	}
	if !h.proves(".claude/launch.json") {
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
	rep := unledgeredReport(cfg, map[string]bool{}, syncStateFile{}, func(p string) bool { return p == ".claude/launch.json" })
	if !reflect.DeepEqual(rep.Orphaned, []string{".claude/launch.json"}) || len(rep.Leftover) != 0 {
		t.Errorf("deleted environment classification: %+v", rep)
	}
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	if h.proves(".claude/launch.json") {
		t.Error("edited launch remains owned")
	}
}

func TestHistoryLaunch_DefaultAdaptersHaveNoOtherLaunchProducer(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	writeBenchProject(t, dir, 1)
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
			if filepath.ToSlash(f.Path) == ".claude/launch.json" {
				t.Errorf("%s emitted launch without environments", name)
			}
		}
	}
}

func TestHistoryLaunch_NestedProjectIgnoresSiblingEnvironment(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, ".agnostic-ai/environments/dev.yaml", "name: sibling\n")
	mustWriteFile(t, "nested/agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, "nested/.claude/launch.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "nested manual")
	testutil.Chdir(t, filepath.Join(dir, "nested"))
	h := &historyRenderer{}
	h.provesAtHEAD(".claude/launch.json")
	if !reflect.DeepEqual(h.renders, map[string]map[string]string{}) {
		t.Errorf("nested project rendered: %v", h.renders)
	}
}

func TestHistoryLaunch_NegativeProofStillConsumesHistoricalAdmission(t *testing.T) {
	dir, git := gitRepo(t)
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
	if h.proves(".claude/launch.json") {
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
	if h.provesAtHEAD(".claude/launch.json") || len(h.renders) != maxHistoryRenders {
		t.Error("pinned negative proof changed")
	}
}

func TestHistoryLaunch_LegacyAndSourceAliasKeepFullRender(t *testing.T) {
	for _, alias := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy", true: "symlink"}[alias], func(t *testing.T) {
			dir, git := gitRepo(t)
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
			h.provesAtHEAD(".claude/launch.json")
			if len(h.renders) != 1 {
				t.Errorf("full renders=%d, want one", len(h.renders))
			}
		})
	}
}

func TestHistoryLaunch_CustomGitFilterKeepsFullRender(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	git("config", "filter.unused.smudge", "cat")
	git("add", "-A")
	git("commit", "-q", "-m", "filter")
	h := &historyRenderer{}
	h.provesAtHEAD(".claude/launch.json")
	if len(h.renders) != 1 {
		t.Errorf("full renders=%d, want one", len(h.renders))
	}
}

func TestHistoryLaunch_RestoredHelperRetainsOwnership(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/overlays/claude/launch.json", "{}\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "helper")
	h := &historyRenderer{}
	if !h.provesAtHEAD(".claude/launch.json") {
		t.Error("restored helper ownership lost")
	}
}

func TestHistoryLaunch_UnrelatedHelperDoesNotDefeatAbsence(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/overlays/claude/readme.txt", "helper\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "manual")
	h := &historyRenderer{}
	h.provesAtHEAD(".claude/launch.json")
	if len(h.renders) != 0 {
		t.Errorf("full renders=%d, want zero", len(h.renders))
	}
}

func TestHistoryLaunch_EscapingSkillNameCannotProduceLaunchAsset(t *testing.T) {
	dir, git := gitRepo(t)
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
	if h.provesAtHEAD(".claude/launch.json") || h.proves(".claude/launch.json") {
		t.Error("invalid skill established ownership")
	}
}

func TestHistoryLaunch_ValidScopedSkillAssetDoesNotOwnRootLaunch(t *testing.T) {
	dir, git := gitRepo(t)
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
	if h.provesAtHEAD(".claude/launch.json") || len(h.renders) != 0 {
		t.Error("scoped skill asset defeated exact-key absence proof")
	}
}

func TestHistoryLaunch_CurrentCustomSourceKeepsFullRender(t *testing.T) {
	dir, git := gitRepo(t)
	testutil.Chdir(t, dir)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".claude/launch.json", "{}\n")
	git("add", "-A")
	git("commit", "-q", "-m", "manual")
	h := &historyRenderer{sources: []string{"custom"}}
	h.provesAtHEAD(".claude/launch.json")
	if len(h.renders) != 1 {
		t.Errorf("full renders=%d, want one", len(h.renders))
	}
}
