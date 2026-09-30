package cli

import (
	"path/filepath"
	"strings"
	"testing"
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
	source := globalNameClashProject(t, "amp, claude, codex, gemini",
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")},
		map[string]string{"skills/gh-issue/SKILL.md": skillSpec("gh-issue", "")})
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	want := "! .agnostic-ai/skills/gh-issue/SKILL.md: skill \"gh-issue\" also exists in " +
		filepath.ToSlash(filepath.Join(source, "skills", "gh-issue", "SKILL.md")) +
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
