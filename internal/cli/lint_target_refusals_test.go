package cli

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestLint_TargetRefusalsFailLint(t *testing.T) {
	for _, tc := range []struct {
		name, config, file, body, want string
	}{
		{
			name:   "codebuddy agent name",
			config: "targets: [claude, codebuddy]\n",
			file:   ".agnostic-ai/agents/reviewer2.md",
			body:   "---\nname: reviewer2\ndescription: Reviews code.\n---\n\nReview the diff.\n",
			want:   "codebuddy",
		},
		{
			name:   "zed skill name",
			config: "targets: [claude, zed]\n",
			file:   ".agnostic-ai/skills/Lint-It/SKILL.md",
			body:   "---\nname: Lint-It\ndescription: Lints code.\n---\n\nRun the linter.\n",
			want:   "zed",
		},
		{
			name:   "zed command written as a skill",
			config: "targets: [claude, zed]\n",
			file:   ".agnostic-ai/commands/Deploy.md",
			body:   "---\nname: Deploy\ndescription: Deploys.\n---\n\nDeploy the app.\n",
			want:   "zed",
		},
		{
			name:   "opencode skill name",
			config: "targets: [claude, opencode]\n",
			file:   ".agnostic-ai/skills/Lint-It/SKILL.md",
			body:   "---\nname: Lint-It\ndescription: Lints code.\n---\n\nRun the linter.\n",
			want:   "opencode",
		},
		{
			name:   "junie agent name",
			config: "targets: [claude, junie]\n",
			file:   ".agnostic-ai/agents/Reviewer.md",
			body:   "---\nname: Reviewer\ndescription: Reviews code.\n---\n\nReview the diff.\n",
			want:   "junie",
		},
		{
			name:   "grok ignored rules",
			config: "targets: [claude, grok]\ngitignore:\n  enabled: true\n",
			file:   ".agnostic-ai/rules/demo.md",
			body:   "---\nname: demo\ndescription: Demo rule.\nalwaysApply: true\n---\n\nDemo body.\n",
			want:   "grok",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := budgetProject(t, tc.config)
			mustWriteFile(t, filepath.Join(dir, filepath.FromSlash(tc.file)), tc.body)

			report, _, err := runLintJSON(t)
			if err == nil {
				t.Fatal("lint must fail on a spec a selected target refuses")
			}
			var found bool
			for _, f := range report.Findings {
				if f.Code == "LINT053" && f.Severity == "error" && strings.Contains(f.Message, tc.want) {
					found = true
					if tc.want != "grok" && f.Path != tc.file {
						t.Errorf("finding path = %q, want %q", f.Path, tc.file)
					}
					if strings.Contains(f.Message, tc.want+": "+tc.want+":") {
						t.Errorf("target named twice: %q", f.Message)
					}
				}
			}
			if !found {
				t.Errorf("want one LINT053 error naming %s, got %+v", tc.want, report.Findings)
			}
		})
	}
}

func TestLint_TargetRefusalsSkipUnselectedTargets(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, codex]\ngitignore:\n  enabled: true\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/agents/reviewer2.md"), "---\nname: reviewer2\ndescription: Reviews code.\n---\n\nReview the diff.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/skills/lint-it/SKILL.md"), "---\nname: lint-it\ndescription: Lints code.\n---\n\nRun the linter.\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/rules/demo.md"), "---\nname: demo\ndescription: Demo rule.\nalwaysApply: true\n---\n\nDemo body.\n")

	if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT053")) != 0 {
		t.Errorf("targets outside the project must not raise LINT053:\n%s", out)
	}
}

func TestLint_GrokWithCommittedInstructionsIsClean(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, grok]\ngitignore:\n  enabled: true\n  commit: [grok:instructions]\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/rules/demo.md"), "---\nname: demo\ndescription: Demo rule.\nalwaysApply: true\n---\n\nDemo body.\n")

	if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT053")) != 0 {
		t.Errorf("committed grok instructions must clear LINT053:\n%s", out)
	}
}

func TestLint_GrokRuleSyncDropsIsClean(t *testing.T) {
	dir := budgetProject(t, "targets: [claude, grok]\ngitignore:\n  enabled: true\n")
	mustWriteFile(t, filepath.Join(dir, ".agnostic-ai/rules/go.md"), "---\nname: go\ndescription: Go rule.\nglobs: [\"**/*.go\"]\n---\n\nGo body.\n")

	if out, _ := runCLI(t, "lint"); len(findingLines(out, "LINT053")) != 0 {
		t.Errorf("a rule sync drops before grok must not raise LINT053:\n%s", out)
	}
}
