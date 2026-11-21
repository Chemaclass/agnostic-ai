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

// userNameClashProject writes a user source and a project in the
// working directory userAgentTestHome picks, and returns the user
// source root.
func userNameClashProject(t *testing.T, targets string, userLevel, project map[string]string) string {
	t.Helper()
	_, source := userAgentTestHome(t)
	for path, body := range userLevel {
		writeFile(t, filepath.Join(source, path), body)
	}
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	for path, body := range project {
		writeFile(t, filepath.Join(".agnostic-ai", path), body)
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

func TestSync_WarnsWhenAProjectSkillSharesAUserName(t *testing.T) {
	userNameClashProject(t, "amp, claude, codex, gemini",
		map[string]string{"skills/gh-issue/SKILL.md": otherSkill("gh-issue")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	want := "! .agnostic-ai/skills/gh-issue/SKILL.md: skill \"gh-issue\" also exists in " +
		"~/source/skills/gh-issue/SKILL.md with different content" +
		"; amp and claude load the user-level one, which exists only on this machine, gemini loads this one" +
		"; to load both, rename the user-level one (such as gh-issue-personal), or delete it to drop it, then run `agnostic-ai sync --scope user`; project sync cannot select this skill instead on amp, claude\n"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("sync should name the shared skill and who wins per target:\nwant %q\n%s", want, buf.String())
	}
}

func TestSync_WarnsWhenAProjectAgentSharesAUserName(t *testing.T) {
	userNameClashProject(t, "claude",
		map[string]string{"agents/reviewer.md": agentSpec("reviewer") + "Also check tests.\n"},
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")})
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "agent \"reviewer\" also exists in ") ||
		!strings.Contains(buf.String(), "; claude loads this one, so the user-level one is unused here; rename one to load both\n") {
		t.Errorf("sync should say the project agent wins in claude:\n%s", buf.String())
	}
}

func TestSync_NoNameClashWarningWhereNoTargetHidesEither(t *testing.T) {
	for _, tc := range []struct {
		name, targets, userLevelConfig, projectExtra string
	}{
		{name: "codex lists both skills", targets: "codex"},
		{name: "user source does not write claude", targets: "claude", userLevelConfig: "version: 1\ntargets: [codex]\n"},
		{name: "project skill skips claude", targets: "claude", projectExtra: "target: codex\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			userLevel := map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")}
			if tc.userLevelConfig != "" {
				userLevel["agnostic-ai.yaml"] = tc.userLevelConfig
			}
			userNameClashProject(t, tc.targets, userLevel,
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

func TestDoctor_ReportsAUserNameClashWithoutFailing(t *testing.T) {
	userNameClashProject(t, "claude",
		map[string]string{"skills/gh-issue/SKILL.md": otherSkill("gh-issue")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
	syncProject(t)

	out, err := runDoctor(t)

	if err != nil {
		t.Fatalf("a shared name must not fail doctor: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Names shared with the user level:\n  ! .agnostic-ai/skills/gh-issue/SKILL.md: skill \"gh-issue\" also exists in ") ||
		!strings.Contains(out, "; claude loads the user-level one, which exists only on this machine;") {
		t.Errorf("doctor should report the shared skill name:\n%s", out)
	}
}

// allowedClashProject shares gh-issue and gh-pr with the home, and the
// local config allows gh-issue under a name Claude folds to the same.
func allowedClashProject(t *testing.T) {
	t.Helper()
	userNameClashProject(t, "claude",
		map[string]string{
			"skills/gh-issue/SKILL.md": otherSkill("gh-issue"),
			"skills/gh-pr/SKILL.md":    otherSkill("gh-pr"),
		},
		map[string]string{
			"skills/gh-issue/SKILL.md": skillSpec("gh-issue", ""),
			"skills/gh-pr/SKILL.md":    skillSpec("gh-pr", ""),
		})
	writeFile(t, "agnostic-ai.local.yaml", "sync:\n  allow-global-names: [GH-Issue]\n")
}

func TestSync_SkipsTheWarningForAnAllowedUserName(t *testing.T) {
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

func TestDoctor_MarksAnAllowedUserNameClash(t *testing.T) {
	allowedClashProject(t)
	syncProject(t)

	out, err := runDoctor(t)

	if err != nil {
		t.Fatalf("a shared name must not fail doctor: %v\n%s", err, out)
	}
	want := "  ~ .agnostic-ai/skills/gh-issue/SKILL.md: skill \"gh-issue\" also exists in ~/source/skills/gh-issue/SKILL.md" +
		"; claude loads the user-level one, which exists only on this machine; allowed by sync.allow-global-names; project sync cannot select this skill instead on claude\n"
	if !strings.Contains(out, want) {
		t.Errorf("doctor should list the allowed clash, marked as allowed:\nwant %q\n%s", want, out)
	}
	if !strings.Contains(out, `  ! .agnostic-ai/skills/gh-pr/SKILL.md: skill "gh-pr" also exists in`) {
		t.Errorf("doctor should still warn about the clash nothing allows:\n%s", out)
	}
}

func TestSync_IgnoresEverySharedUserNameWhenTheModeSaysSo(t *testing.T) {
	allowedClashProject(t)
	writeFile(t, "agnostic-ai.local.yaml", "sync:\n  global-name-clash: ignore\n")
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(buf.String(), "also exists in") {
		t.Errorf("sync.global-name-clash: ignore should keep sync quiet about every shared name:\n%s", buf.String())
	}
}

func TestSync_WarnModeStillHonoursTheAllowList(t *testing.T) {
	allowedClashProject(t)
	writeFile(t, "agnostic-ai.local.yaml", "sync:\n  global-name-clash: warn\n  allow-global-names: [gh-issue]\n")
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	if strings.Contains(buf.String(), `skill "gh-issue" also exists in`) {
		t.Errorf("gh-issue is allowed, so sync should not warn about it:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), `skill "gh-pr" also exists in`) {
		t.Errorf("warn mode should still warn about gh-pr:\n%s", buf.String())
	}
}

func TestDoctor_MarksIgnoredUserNameClashes(t *testing.T) {
	allowedClashProject(t)
	writeFile(t, "agnostic-ai.local.yaml", "sync:\n  global-name-clash: ignore\n")
	syncProject(t)

	out, err := runDoctor(t)

	if err != nil {
		t.Fatalf("a shared name must not fail doctor: %v\n%s", err, out)
	}
	want := "  ~ .agnostic-ai/skills/gh-pr/SKILL.md: skill \"gh-pr\" also exists in ~/source/skills/gh-pr/SKILL.md" +
		"; claude loads the user-level one, which exists only on this machine; ignored by sync.global-name-clash; project sync cannot select this skill instead on claude\n"
	if !strings.Contains(out, want) {
		t.Errorf("doctor should list the ignored clash, marked as ignored:\nwant %q\n%s", want, out)
	}
	if strings.Contains(out, "  ! .agnostic-ai/skills/") {
		t.Errorf("ignore mode leaves no clash to warn about in doctor:\n%s", out)
	}
}

func clashesInWorkingDir(t *testing.T) []userNameClash {
	t.Helper()
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	return userNameClashes(b, cfg.Targets)
}

func TestUserNameClashes_SkipsAProjectThatIsTheHome(t *testing.T) {
	home, _ := userAgentTestHome(t)
	t.Setenv("AGNOSTIC_AI_HOME", "")
	testutil.Chdir(t, home)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "gh-issue", "SKILL.md"), skillSpec("gh-issue", ""))

	if got := clashesInWorkingDir(t); len(got) != 0 {
		t.Errorf("a skill cannot clash with itself: %v", got)
	}
}

func TestUserNameClashes_SkipsAHomeLinkedToTheProjectSpecs(t *testing.T) {
	home, _ := userAgentTestHome(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	writeFile(t, filepath.Join(".agnostic-ai", "skills", "gh-issue", "SKILL.md"), skillSpec("gh-issue", ""))
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

func TestUserNameClashes_ClaudeFoldsSkillNames(t *testing.T) {
	userNameClashProject(t, "claude, gemini",
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("GH-Issue", "")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("\"ｇｈ-issue\\u200b\"", "")})

	got := clashesInWorkingDir(t)

	if len(got) != 1 || !slices.Equal(got[0].userWins, []string{"claude"}) || len(got[0].projectWins) != 0 {
		t.Errorf("Claude should match the folded name, Gemini only the exact one: %+v", got)
	}
}

func TestUserNameClash_AbbreviatesHome(t *testing.T) {
	home, _ := userAgentTestHome(t)
	c := userNameClash{
		project:   spec.Entry{Kind: spec.KindSkill, Name: "gh-issue", Path: ".agnostic-ai/skills/gh-issue/SKILL.md"},
		userLevel: spec.Entry{Path: filepath.Join(home, ".agnostic-ai", "skills", "gh-issue", "SKILL.md")},
		userWins:  []string{"claude"},
	}

	if got := c.String(); !strings.Contains(got, "also exists in ~/.agnostic-ai/skills/gh-issue/SKILL.md;") {
		t.Errorf("the user-level path should start with ~: %s", got)
	}
}

func TestUserNameClashes_AmpOnlyWhenBothWriteAmp(t *testing.T) {
	userNameClashProject(t, "amp, claude",
		map[string]string{
			"agnostic-ai.yaml":         "version: 1\ntargets: [claude]\n",
			"skills/gh-issue/SKILL.md": skillSpec("gh-issue", ""),
		},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})

	got := clashesInWorkingDir(t)

	if len(got) != 1 || !slices.Equal(got[0].userWins, []string{"claude"}) {
		t.Errorf("Amp loads the project copy of a claude-only user-level skill, so only claude wins globally: %+v", got)
	}
}

func TestUserNameClashes_ClineLoadsTheUserSkill(t *testing.T) {
	userNameClashProject(t, "cline",
		map[string]string{"skills/gh-issue/SKILL.md": otherSkill("gh-issue")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})

	got := clashesInWorkingDir(t)

	if len(got) != 1 || !slices.Equal(got[0].userWins, []string{"cline"}) {
		t.Errorf("Cline loads the user-level skill over the project one: %+v", got)
	}
}

func TestUserNameClashes_ClineLoadsAHomeSkillFromTheSharedAgentsFolder(t *testing.T) {
	userNameClashProject(t, "cline",
		map[string]string{
			"agnostic-ai.yaml":         "version: 1\ntargets: [amp]\n",
			"skills/gh-issue/SKILL.md": skillSpec("gh-issue", ""),
		},
		map[string]string{"skills/gh-issue/SKILL.md": otherSkill("gh-issue")})

	got := clashesInWorkingDir(t)

	if len(got) != 1 || !slices.Equal(got[0].userWins, []string{"cline"}) {
		t.Errorf("home syncs amp into ~/.agents/skills, which Cline reads: %+v", got)
	}
}

func TestUserNameClashes_SkipsTargetsWithNoUserLevelSurface(t *testing.T) {
	prev := userNameWinners[spec.KindAgent]
	userNameWinners[spec.KindAgent] = map[string]bool{"claude": false, "amp": true}
	t.Cleanup(func() { userNameWinners[spec.KindAgent] = prev })
	userNameClashProject(t, "amp, claude",
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")},
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")})

	got := clashesInWorkingDir(t)

	if len(got) != 1 || len(got[0].userWins) != 0 || !slices.Equal(got[0].projectWins, []string{"claude"}) {
		t.Errorf("amp writes no user-level agents, so it cannot hide one: %+v", got)
	}
}

func TestSync_SaysAnIdenticalUserCopyChangesNothing(t *testing.T) {
	userNameClashProject(t, "claude",
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	want := "! .agnostic-ai/skills/gh-issue/SKILL.md: skill \"gh-issue\" is identical in ~/source/skills/gh-issue/SKILL.md" +
		", so every target loads the same content; delete one copy to keep them from drifting apart, and run `agnostic-ai sync --scope user` after deleting the user-level one; project sync cannot select this skill instead on claude\n"
	if !strings.Contains(buf.String(), want) {
		t.Errorf("sync should say an identical copy changes nothing:\nwant %q\n%s", want, buf.String())
	}
}

func TestUserNameClashes_ComparesContent(t *testing.T) {
	const skill = "skills/gh-issue/SKILL.md"
	const asset = "skills/gh-issue/scripts/run.sh"
	for _, tc := range []struct {
		name               string
		userLevel, project map[string]string
		want               contentMatch
	}{
		{
			name:      "same skill under a folded name",
			userLevel: map[string]string{skill: skillSpec("GH-Issue", "")},
			project:   map[string]string{skill: skillSpec("gh-issue", "")},
			want:      contentIdentical,
		},
		{
			name:      "same flat-file skill",
			userLevel: map[string]string{"skills/gh-issue.md": skillSpec("gh-issue", "")},
			project:   map[string]string{"skills/gh-issue.md": skillSpec("gh-issue", "")},
			want:      contentIdentical,
		},
		{
			name:      "frontmatter differs",
			userLevel: map[string]string{skill: otherSkill("gh-issue")},
			project:   map[string]string{skill: skillSpec("gh-issue", "")},
			want:      contentDiffers,
		},
		{
			name:      "body differs",
			userLevel: map[string]string{skill: skillSpec("gh-issue", "") + "More steps.\n"},
			project:   map[string]string{skill: skillSpec("gh-issue", "")},
			want:      contentDiffers,
		},
		{
			name:      "nested asset differs",
			userLevel: map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo global\n"},
			project:   map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo project\n"},
			want:      contentDiffers,
		},
		{
			name:      "asset only in the user-level copy",
			userLevel: map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo global\n"},
			project:   map[string]string{skill: skillSpec("gh-issue", "")},
			want:      contentDiffers,
		},
		{
			name:      "same nested assets",
			userLevel: map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo same\n"},
			project:   map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo same\n"},
			want:      contentIdentical,
		},
		{
			name: "user-local override keeps the shared folder's assets",
			userLevel: map[string]string{
				skill:            skillSpec("gh-issue", ""),
				asset:            "echo same\n",
				"local/" + skill: skillSpec("gh-issue", ""),
			},
			project: map[string]string{skill: skillSpec("gh-issue", ""), asset: "echo same\n"},
			want:    contentIdentical,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			userNameClashProject(t, "claude", tc.userLevel, tc.project)

			got := clashesInWorkingDir(t)

			if len(got) != 1 || got[0].content != tc.want {
				t.Errorf("want one clash with content %v: %+v", tc.want, got)
			}
		})
	}
}

func TestUserNameClashes_SameAgentIsIdentical(t *testing.T) {
	userNameClashProject(t, "claude",
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")},
		map[string]string{"agents/reviewer.md": agentSpec("reviewer")})

	got := clashesInWorkingDir(t)

	if len(got) != 1 || got[0].content != contentIdentical {
		t.Errorf("want one identical agent clash: %+v", got)
	}
}

func TestUserNameClashes_ModelTierMakesNoContentClaim(t *testing.T) {
	userNameClashProject(t, "claude",
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "model: strong\n")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "model: strong\n")})
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nmodels:\n  strong: {claude: opus}\n")

	got := clashesInWorkingDir(t)

	if len(got) != 1 || got[0].content != contentUnknown {
		t.Fatalf("each side resolves the tier through its own config, so the copies cannot be called identical: %+v", got)
	}
	if text := got[0].String(); strings.Contains(text, "different content") || strings.Contains(text, "identical") {
		t.Errorf("the warning should make no content claim: %s", text)
	}
}

func TestUserNameClashes_SkipsSymlinkedAssetsLikeTheEmitter(t *testing.T) {
	source := userNameClashProject(t, "claude",
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

func TestUserNameClashes_UnreadableAssetMakesNoContentClaim(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs POSIX permissions and a non-root user")
	}
	source := userNameClashProject(t, "claude",
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

func TestUserNameClash_MessagePerWinner(t *testing.T) {
	project := spec.Entry{Kind: spec.KindSkill, Name: "gh-issue", Path: ".agnostic-ai/skills/gh-issue/SKILL.md"}
	userLevel := spec.Entry{Name: "gh-issue", Path: "/elsewhere/skills/gh-issue/SKILL.md"}
	const rename = "; to load both, rename the user-level one (such as gh-issue-personal), or delete it to drop it, then run `agnostic-ai sync --scope user`"
	for _, tc := range []struct {
		name string
		c    userNameClash
		want string
	}{
		{
			name: "user-level wins",
			c:    userNameClash{project: project, userLevel: userLevel, userWins: []string{"claude"}, content: contentDiffers},
			want: " with different content; claude loads the user-level one, which exists only on this machine" + rename + "; project sync cannot select this skill instead on claude",
		},
		{
			name: "project wins",
			c:    userNameClash{project: project, userLevel: userLevel, projectWins: []string{"gemini"}, content: contentDiffers},
			want: " with different content; gemini loads this one, so the user-level one is unused here; rename one to load both",
		},
		{
			name: "each wins somewhere",
			c:    userNameClash{project: project, userLevel: userLevel, userWins: []string{"claude"}, projectWins: []string{"gemini"}, content: contentDiffers},
			want: " with different content; claude loads the user-level one, which exists only on this machine, gemini loads this one" + rename + "; project sync cannot select this skill instead on claude",
		},
		{
			name: "identical",
			c:    userNameClash{project: project, userLevel: userLevel, userWins: []string{"claude"}, content: contentIdentical},
			want: " is identical in /elsewhere/skills/gh-issue/SKILL.md, so every target loads the same content; delete one copy to keep them from drifting apart, and run `agnostic-ai sync --scope user` after deleting the user-level one; project sync cannot select this skill instead on claude",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.String(); !strings.HasSuffix(got, tc.want) {
				t.Errorf("want suffix %q\ngot %q", tc.want, got)
			}
		})
	}
}

func TestUserNameClash_ReportsUnsupportedProjectSkillSelection(t *testing.T) {
	clash := userNameClash{
		project:   spec.Entry{Kind: spec.KindSkill, Name: "gh-issue", Path: ".agnostic-ai/skills/gh-issue/SKILL.md"},
		userLevel: spec.Entry{Kind: spec.KindSkill, Name: "gh-issue", Path: "/home/source/skills/gh-issue/SKILL.md"},
		userWins:  []string{"claude"}, projectWins: []string{"gemini"}, content: contentDiffers,
	}
	for _, line := range []string{clash.String(), clash.silenced("allowed by sync.allow-global-names")} {
		for _, text := range []string{"claude loads the user-level one", "gemini loads this one", "project sync cannot select this skill instead on claude"} {
			if !strings.Contains(line, text) {
				t.Errorf("skill selection limit missing %q: %s", text, line)
			}
		}
		if strings.Contains(line, "instead on gemini") {
			t.Errorf("project winner reported unsupported: %s", line)
		}
	}
}

func TestSyncAndDoctor_ExplainUnsupportedHomeWinningSkill(t *testing.T) {
	userNameClashProject(t, "claude,gemini",
		map[string]string{"skills/gh-issue/SKILL.md": otherSkill("gh-issue")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
	log := captureLog(t)
	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}
	out, err := runDoctor(t)
	if err != nil {
		t.Fatalf("doctor: %v: %s", err, out)
	}
	for _, text := range []string{"claude loads the user-level one", "gemini loads this one", "project sync cannot select this skill instead on claude"} {
		if !strings.Contains(log.String(), text) {
			t.Errorf("sync lacks %q: %s", text, log.String())
		}
		if !strings.Contains(out, text) {
			t.Errorf("doctor lacks %q: %s", text, out)
		}
	}
}

func TestDoctor_ExplainsUnsupportedSkillEvenWhenSyncIsSilenced(t *testing.T) {
	for _, setting := range []string{"  allow-global-names: [gh-issue]\n", "  global-name-clash: ignore\n"} {
		t.Run(strings.TrimSpace(setting), func(t *testing.T) {
			userNameClashProject(t, "claude,gemini",
				map[string]string{"skills/gh-issue/SKILL.md": otherSkill("gh-issue")},
				map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
			writeFile(t, "agnostic-ai.local.yaml", "sync:\n"+setting)
			log := captureLog(t)
			if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(log.String(), "also exists in") {
				t.Errorf("silenced sync warned: %s", log.String())
			}
			out, err := runDoctor(t)
			if err != nil {
				t.Fatalf("doctor: %v: %s", err, out)
			}
			if !strings.Contains(out, "project sync cannot select this skill instead on claude") || !strings.Contains(out, "gemini loads this one") {
				t.Errorf("doctor hides selection limit: %s", out)
			}
		})
	}
}

func TestUserNameClash_ClaudeAgentStillSelectsProjectCopy(t *testing.T) {
	clash := userNameClash{project: spec.Entry{Kind: spec.KindAgent, Name: "reviewer"}, userLevel: spec.Entry{Kind: spec.KindAgent, Name: "reviewer"}, projectWins: []string{"claude"}, content: contentDiffers}
	line := clash.String()
	if !strings.Contains(line, "claude loads this one, so the user-level one is unused here") {
		t.Errorf("agent winner changed: %s", line)
	}
	if strings.Contains(line, "project sync cannot") {
		t.Errorf("skill limit leaked to agents: %s", line)
	}
}
