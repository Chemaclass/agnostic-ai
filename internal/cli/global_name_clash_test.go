package cli

import (
	"os"
	"path/filepath"
	"runtime"
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

// otherSkill is a skill under the same name whose content differs.
func otherSkill(name string) string {
	return skillSpec(name, "argument-hint: \"[number]\"\n")
}

func TestSync_WarnsWhenAProjectSkillSharesAGlobalName(t *testing.T) {
	globalNameClashProject(t, "amp, claude, codex, gemini",
		map[string]string{"skills/gh-issue/SKILL.md": otherSkill("gh-issue")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	want := "! .agnostic-ai/skills/gh-issue/SKILL.md: skill \"gh-issue\" also exists in " +
		"~/source/skills/gh-issue/SKILL.md with different content" +
		"; amp and claude load the global one, which exists only on this machine, gemini loads this one" +
		"; to load both, rename the global one (such as gh-issue-personal), or delete it to drop it, then run `agnostic-ai sync --global`\n"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("sync should name the shared skill and who wins per target:\nwant %q\n%s", want, buf.String())
	}
}

func TestSync_WarnsWhenAProjectAgentSharesAGlobalName(t *testing.T) {
	globalNameClashProject(t, "claude",
		map[string]string{"agents/reviewer.md": agentSpec("reviewer") + "Also check tests.\n"},
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")})
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "agent \"reviewer\" also exists in ") ||
		!strings.Contains(buf.String(), "; claude loads this one, so the global one is unused here; rename one to load both\n") {
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
		map[string]string{"skills/gh-issue/SKILL.md": otherSkill("gh-issue")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
	syncProject(t)

	out, err := runDoctor(t)

	if err != nil {
		t.Fatalf("a shared name must not fail doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Global names:\n  ! .agnostic-ai/skills/gh-issue/SKILL.md: skill \"gh-issue\" also exists in ") ||
		!strings.Contains(out, "; claude loads the global one, which exists only on this machine;") {
		t.Errorf("doctor should report the shared skill name:\n%s", out)
	}
}

// allowedClashProject shares gh-issue and gh-pr with the home, and the
// local config allows gh-issue under a name Claude folds to the same.
func allowedClashProject(t *testing.T) {
	t.Helper()
	globalNameClashProject(t, "claude",
		map[string]string{
			"skills/gh-issue/SKILL.md": otherSkill("gh-issue"),
			"skills/gh-pr/SKILL.md":    otherSkill("gh-pr"),
		},
		map[string]string{
			"skills/gh-issue/SKILL.md": skillSpec("gh-issue", ""),
			"skills/gh-pr/SKILL.md":    skillSpec("gh-pr", ""),
		})
	mustWriteFile(t, "agnostic-ai.local.yaml", "sync:\n  allow-global-names: [GH-Issue]\n")
}

func TestSync_SkipsTheWarningForAnAllowedGlobalName(t *testing.T) {
	allowedClashProject(t)
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(buf.String(), `skill "gh-issue" also exists in`) {
		t.Errorf("sync.allow-global-names lists gh-issue, so sync should not warn about it:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `! .agnostic-ai/skills/gh-pr/SKILL.md: skill "gh-pr" also exists in`) {
		t.Errorf("gh-pr is not allowed, so sync should still warn about it:\n%s", buf.String())
	}
}

func TestDoctor_MarksAnAllowedGlobalNameClash(t *testing.T) {
	allowedClashProject(t)
	syncProject(t)

	out, err := runDoctor(t)

	if err != nil {
		t.Fatalf("a shared name must not fail doctor: %v\n%s", err, out)
	}
	want := "  ~ .agnostic-ai/skills/gh-issue/SKILL.md: skill \"gh-issue\" also exists in ~/source/skills/gh-issue/SKILL.md" +
		"; claude loads the global one, which exists only on this machine; allowed by sync.allow-global-names\n"
	if !strings.Contains(out, want) {
		t.Errorf("doctor should list the allowed clash, marked as allowed:\nwant %q\n%s", want, out)
	}
	if !strings.Contains(out, `  ! .agnostic-ai/skills/gh-pr/SKILL.md: skill "gh-pr" also exists in`) {
		t.Errorf("doctor should still warn about the clash nothing allows:\n%s", out)
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

func TestSync_SaysAnIdenticalGlobalCopyChangesNothing(t *testing.T) {
	globalNameClashProject(t, "claude",
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	want := "! .agnostic-ai/skills/gh-issue/SKILL.md: skill \"gh-issue\" is identical in ~/source/skills/gh-issue/SKILL.md" +
		", so every target loads the same content; delete one copy to keep them from drifting apart, and run `agnostic-ai sync --global` after deleting the global one\n"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("sync should say an identical copy changes nothing:\nwant %q\n%s", want, buf.String())
	}
}

func TestGlobalNameClashes_ComparesContent(t *testing.T) {
	const skill = "skills/gh-issue/SKILL.md"
	const asset = "skills/gh-issue/scripts/run.sh"
	for _, tc := range []struct {
		name            string
		global, project map[string]string
		want            contentMatch
	}{
		{
			name:    "same skill under a folded name",
			global:  map[string]string{skill: skillSpec("GH-Issue", "")},
			project: map[string]string{skill: skillSpec("gh-issue", "")},
			want:    contentIdentical,
		},
		{
			name:    "same flat-file skill",
			global:  map[string]string{"skills/gh-issue.md": skillSpec("gh-issue", "")},
			project: map[string]string{"skills/gh-issue.md": skillSpec("gh-issue", "")},
			want:    contentIdentical,
		},
		{
			name:    "frontmatter differs",
			global:  map[string]string{skill: otherSkill("gh-issue")},
			project: map[string]string{skill: skillSpec("gh-issue", "")},
			want:    contentDiffers,
		},
		{
			name:    "body differs",
			global:  map[string]string{skill: skillSpec("gh-issue", "") + "More steps.\n"},
			project: map[string]string{skill: skillSpec("gh-issue", "")},
			want:    contentDiffers,
		},
		{
			name:    "nested asset differs",
			global:  map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo global\n"},
			project: map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo project\n"},
			want:    contentDiffers,
		},
		{
			name:    "asset only in the global copy",
			global:  map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo global\n"},
			project: map[string]string{skill: skillSpec("gh-issue", "")},
			want:    contentDiffers,
		},
		{
			name:    "same nested assets",
			global:  map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo same\n"},
			project: map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo same\n"},
			want:    contentIdentical,
		},
		{
			name: "global-local override keeps the shared folder's assets",
			global: map[string]string{
				skill:            skillSpec("gh-issue", ""),
				asset:            "echo same\n",
				"local/" + skill: skillSpec("gh-issue", ""),
			},
			project: map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo same\n"},
			want:    contentIdentical,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			globalNameClashProject(t, "claude", tc.global, tc.project)

			got := clashesInWorkingDir(t)

			if len(got) != 1 || got[0].content != tc.want {
				t.Errorf("want one clash with content %v: %+v", tc.want, got)
			}
		})
	}
}

func TestGlobalNameClashes_SameAgentIsIdentical(t *testing.T) {
	globalNameClashProject(t, "claude",
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")},
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")})

	got := clashesInWorkingDir(t)

	if len(got) != 1 || got[0].content != contentIdentical {
		t.Errorf("want one identical agent clash: %+v", got)
	}
}

func TestGlobalNameClashes_ModelTierMakesNoContentClaim(t *testing.T) {
	globalNameClashProject(t, "claude",
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "model: strong\n")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "model: strong\n")})
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nmodels:\n  strong: {claude: opus}\n")

	got := clashesInWorkingDir(t)

	if len(got) != 1 || got[0].content != contentUnknown {
		t.Fatalf("each side resolves the tier through its own config, so the copies cannot be called identical: %+v", got)
	}
	if text := got[0].String(); strings.Contains(text, "different content") || strings.Contains(text, "identical") {
		t.Errorf("the warning should make no content claim: %s", text)
	}
}

func TestGlobalNameClashes_SkipsSymlinkedAssetsLikeTheEmitter(t *testing.T) {
	source := globalNameClashProject(t, "claude",
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
	if err := os.Symlink(os.TempDir(), filepath.Join(source, "skills", "gh-issue", "tmp")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	got := clashesInWorkingDir(t)

	if len(got) != 1 || got[0].content != contentIdentical {
		t.Errorf("a symlink never ships, so the copies match: %+v", got)
	}
}

func TestGlobalNameClashes_UnreadableAssetMakesNoContentClaim(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	source := globalNameClashProject(t, "claude",
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", ""), "skills/gh-issue/run.sh": "echo same\n"},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", ""), "skills/gh-issue/run.sh": "echo same\n"})
	unreadable := filepath.Join(source, "skills", "gh-issue", "run.sh")
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(unreadable, 0o644) })

	got := clashesInWorkingDir(t)

	if len(got) != 1 || got[0].content != contentUnknown {
		t.Errorf("an unreadable asset proves nothing: %+v", got)
	}
}

func TestGlobalNameClash_MessagePerWinner(t *testing.T) {
	project := spec.Entry{Kind: spec.KindSkill, Name: "gh-issue", Path: ".agnostic-ai/skills/gh-issue/SKILL.md"}
	global := spec.Entry{Name: "gh-issue", Path: "/elsewhere/skills/gh-issue/SKILL.md"}
	const rename = "; to load both, rename the global one (such as gh-issue-personal), or delete it to drop it, then run `agnostic-ai sync --global`"
	for _, tc := range []struct {
		name string
		c    globalNameClash
		want string
	}{
		{
			name: "global wins",
			c:    globalNameClash{project: project, global: global, globalWins: []string{"claude"}, content: contentDiffers},
			want: " with different content; claude loads the global one, which exists only on this machine" + rename,
		},
		{
			name: "project wins",
			c:    globalNameClash{project: project, global: global, projectWins: []string{"gemini"}, content: contentDiffers},
			want: " with different content; gemini loads this one, so the global one is unused here; rename one to load both",
		},
		{
			name: "each wins somewhere",
			c:    globalNameClash{project: project, global: global, globalWins: []string{"claude"}, projectWins: []string{"gemini"}, content: contentDiffers},
			want: " with different content; claude loads the global one, which exists only on this machine, gemini loads this one" + rename,
		},
		{
			name: "identical",
			c:    globalNameClash{project: project, global: global, globalWins: []string{"claude"}, content: contentIdentical},
			want: " is identical in /elsewhere/skills/gh-issue/SKILL.md, so every target loads the same content; delete one copy to keep them from drifting apart, and run `agnostic-ai sync --global` after deleting the global one",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.String(); !strings.HasSuffix(got, tc.want) {
				t.Errorf("want suffix %q\ngot %q", tc.want, got)
			}
		})
	}
}
