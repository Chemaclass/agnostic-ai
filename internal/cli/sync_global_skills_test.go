package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_RendersSkillMetadataAndPreservesAssets(t *testing.T) {
	for _, tc := range []struct{ name, meta string }{
		{"override", "x-claude:\n  model: opus\n  effort: xhigh\n"},
		{"target maps", "model: {claude: opus}\neffort: {claude: xhigh}\n"},
		{"override precedence", "model: {claude: sonnet}\neffort: {claude: low}\nx-claude:\n  model: opus\n  effort: xhigh\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			folder := filepath.Join(source, "skills", "review")
			mustWriteGlobalTest(t, filepath.Join(folder, "SKILL.md"), "---\nname: review\ndescription: Review code\n"+tc.meta+"---\nReview carefully.\n")
			asset := "#!/bin/sh\nprintf 'asset\\n'\n"
			mustWriteGlobalTest(t, filepath.Join(folder, "scripts", "check.sh"), asset)
			if err := os.Chmod(filepath.Join(folder, "scripts", "check.sh"), 0o755); err != nil {
				t.Fatal(err)
			}
			for _, flag := range []string{"", "", "--check"} {
				args := []string{"--only", "claude,codex,cursor"}
				if flag != "" {
					args = append(args, flag)
				}
				if _, _, err := runGlobalAgentTest(args...); err != nil {
					t.Fatal(err)
				}
			}
			for _, dir := range []string{".claude", ".agents", ".cursor"} {
				data, err := os.ReadFile(filepath.Join(home, dir, "skills", "review", "SKILL.md"))
				if err != nil {
					t.Fatal(err)
				}
				got := string(data)
				if strings.Contains(got, "x-claude:") || !strings.Contains(got, "Review carefully.") {
					t.Errorf("%s invalid skill: %s", dir, got)
				}
				if dir == ".claude" {
					if !strings.Contains(got, "model: opus") || !strings.Contains(got, "effort: xhigh") {
						t.Errorf("Claude metadata: %s", got)
					}
				} else if strings.Contains(got, "model:") || strings.Contains(got, "effort:") {
					t.Errorf("%s leaked metadata: %s", dir, got)
				}
				path := filepath.Join(home, dir, "skills", "review", "scripts", "check.sh")
				copied, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(copied) != asset {
					t.Errorf("%s changed asset: %q", dir, copied)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				sourceInfo, err := os.Stat(filepath.Join(folder, "scripts", "check.sh"))
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != sourceInfo.Mode().Perm() {
					t.Errorf("%s changed asset mode", dir)
				}
			}
		})
	}
}

func TestSyncGlobal_SharedSkillsStayNeutralAcrossTargetSelections(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Neutral description\nx-codex:\n  description: Codex description\n  custom: codex-only\nx-amp:\n  description: Amp description\n  custom: amp-only\n---\nReview carefully.\n")
	var first []byte
	for _, targets := range []string{"codex", "amp", "codex,amp", "amp,codex"} {
		if _, _, err := runGlobalAgentTest("--only", targets); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "review", "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "description: Neutral description") || strings.Contains(string(data), "custom:") || strings.Contains(string(data), "x-codex:") || strings.Contains(string(data), "x-amp:") {
			t.Errorf("%s leaked target metadata: %s", targets, data)
		}
		if first == nil {
			first = data
		} else if !bytes.Equal(data, first) {
			t.Errorf("%s changed shared skill", targets)
		}
		if _, _, err := runGlobalAgentTest("--only", targets, "--check"); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSyncGlobal_ReportsDroppedSkillModelAndEffort(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\nmodel: {claude: opus, codex: model-code, cursor: model-cursor}\neffort: high\n---\nReview.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "deploy", "SKILL.md"), "---\nname: deploy\nmodel: shared-model\neffort: low\n---\nDeploy.\n")
	_, warnings, err := runGlobalAgentTest("--only", "claude,codex,cursor")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"model", "effort"} {
		if !strings.Contains(warnings, "`"+field+"` on 2 skills has no effect on codex, cursor") {
			t.Errorf("missing %s coverage: %s", field, warnings)
		}
	}
}
