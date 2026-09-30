package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// globalNameClashProject writes a global home and a project in the
// working directory globalAgentTestHome picks, and returns the global
// source root.
func globalNameClashProject(t *testing.T, targets string, global, project map[string]string) string {
	t.Helper()
	_, source := globalAgentTestHome(t)
	for path, body := range global {
		mustWriteGlobalTest(t, filepath.Join(source, path), body)
	}
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	for path, body := range project {
		mustWriteFile(t, filepath.Join(".agnostic-ai", path), body)
	}
	silence(t)
	return source
}

func skillSpec(name, extra string) string {
	return "---\nname: " + name + "\ndescription: Work one issue.\n" + extra + "---\nSteps.\n"
}

func agentSpec(name string) string {
	return "---\nname: " + name + "\ndescription: Reviews diffs.\n---\nReview.\n"
}

func TestSync_WarnsWhenAProjectSkillSharesAGlobalName(t *testing.T) {
	globalNameClashProject(t, "amp, claude, codex, gemini",
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	want := "! .agnostic-ai/skills/gh-issue/SKILL.md: skill \"gh-issue\" also exists in " +
		"~/source/skills/gh-issue/SKILL.md" +
		"; amp and claude load the global one, gemini loads this one; rename one to load both\n"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("sync should name the shared skill and who wins per target:\nwant %q\n%s", want, buf.String())
	}
}

func TestSync_WarnsWhenAProjectAgentSharesAGlobalName(t *testing.T) {
	globalNameClashProject(t, "claude",
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")},
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")})
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "agent \"reviewer\" also exists in ") || !strings.Contains(buf.String(), "; claude loads this one;") {
		t.Errorf("sync should say the project agent wins in claude:\n%s", buf.String())
	}
}

func TestSync_NoNameClashWarningWhereNoTargetHidesEither(t *testing.T) {
	for _, tc := range []struct {
		name, targets, globalConfig, projectExtra string
	}{
		{name: "codex lists both skills", targets: "codex"},
		{name: "global home does not write claude", targets: "claude", globalConfig: "version: 1\ntargets: [codex]\n"},
		{name: "project skill skips claude", targets: "claude", projectExtra: "target: codex\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			global := map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")}
			if tc.globalConfig != "" {
				global["agnostic-ai.yaml"] = tc.globalConfig
			}
			globalNameClashProject(t, tc.targets, global,
				map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", tc.projectExtra)})
			buf := captureLog(t)

			if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
				t.Fatal(err)
			}

			if strings.Contains(buf.String(), "also exists in") {
				t.Errorf("no target hides either skill, so sync should stay quiet:\n%s", buf.String())
			}
		})
	}
}

func TestDoctor_ReportsAGlobalNameClashWithoutFailing(t *testing.T) {
	globalNameClashProject(t, "claude",
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
	syncProject(t)

	out, err := runDoctor(t)

	if err != nil {
		t.Fatalf("a shared name must not fail doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Global names:\n  ! .agnostic-ai/skills/gh-issue/SKILL.md: skill \"gh-issue\" also exists in ") ||
		!strings.Contains(out, "; claude loads the global one;") {
		t.Errorf("doctor should report the shared skill name:\n%s", out)
	}
}

func clashesInWorkingDir(t *testing.T) []globalNameClash {
	t.Helper()
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	return globalNameClashes(b, cfg.Targets)
}

func TestGlobalNameClashes_SkipsAProjectThatIsTheHome(t *testing.T) {
	home, _ := globalAgentTestHome(t)
	t.Setenv("AGNOSTIC_AI_HOME", "")
	testutil.Chdir(t, home)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(".agnostic-ai", "skills", "gh-issue", "SKILL.md"), skillSpec("gh-issue", ""))

	if got := clashesInWorkingDir(t); len(got) != 0 {
		t.Errorf("a skill cannot clash with itself: %v", got)
	}
}

func TestGlobalNameClashes_SkipsAHomeLinkedToTheProjectSpecs(t *testing.T) {
	home, _ := globalAgentTestHome(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(".agnostic-ai", "skills", "gh-issue", "SKILL.md"), skillSpec("gh-issue", ""))
	specs, err := filepath.Abs(".agnostic-ai")
	if err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "linked-home")
	if err := os.Symlink(specs, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv("AGNOSTIC_AI_HOME", link)

	if got := clashesInWorkingDir(t); len(got) != 0 {
		t.Errorf("a skill cannot clash with itself through a link: %v", got)
	}
}

func TestGlobalNameClashes_ClaudeFoldsSkillNames(t *testing.T) {
	globalNameClashProject(t, "claude, gemini",
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("GH-Issue", "")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("\"ｇｈ-issue\\u200b\"", "")})

	got := clashesInWorkingDir(t)

	if len(got) != 1 || !slices.Equal(got[0].globalWins, []string{"claude"}) || len(got[0].projectWins) != 0 {
		t.Errorf("Claude should match the folded name, Gemini only the exact one: %+v", got)
	}
}

func TestGlobalNameClash_AbbreviatesHome(t *testing.T) {
	home, _ := globalAgentTestHome(t)
	c := globalNameClash{
		project:    spec.Entry{Kind: spec.KindSkill, Name: "gh-issue", Path: ".agnostic-ai/skills/gh-issue/SKILL.md"},
		global:     spec.Entry{Path: filepath.Join(home, ".agnostic-ai", "skills", "gh-issue", "SKILL.md")},
		globalWins: []string{"claude"},
	}

	if got := c.String(); !strings.Contains(got, "also exists in ~/.agnostic-ai/skills/gh-issue/SKILL.md;") {
		t.Errorf("the global path should start with ~: %s", got)
	}
}

func TestGlobalNameClashes_AmpOnlyWhenBothWriteAmp(t *testing.T) {
	globalNameClashProject(t, "amp, claude",
		map[string]string{
			"agnostic-ai.yaml":         "version: 1\ntargets: [claude]\n",
			"skills/gh-issue/SKILL.md": skillSpec("gh-issue", ""),
		},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})

	got := clashesInWorkingDir(t)

	if len(got) != 1 || !slices.Equal(got[0].globalWins, []string{"claude"}) {
		t.Errorf("Amp loads the project copy of a claude-only global skill, so only claude wins globally: %+v", got)
	}
}

func TestGlobalNameClashes_SkipsTargetsWithNoUserLevelSurface(t *testing.T) {
	prev := globalNameWinners[spec.KindAgent]
	globalNameWinners[spec.KindAgent] = map[string]bool{"claude": false, "amp": true}
	t.Cleanup(func() { globalNameWinners[spec.KindAgent] = prev })
	globalNameClashProject(t, "amp, claude",
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")},
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")})

	got := clashesInWorkingDir(t)

	if len(got) != 1 || len(got[0].globalWins) != 0 || !slices.Equal(got[0].projectWins, []string{"claude"}) {
		t.Errorf("amp writes no global agents, so it cannot hide one: %+v", got)
	}
}
