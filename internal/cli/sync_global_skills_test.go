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

func TestSyncGlobal_ReportsDroppedSkillFields(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\nmodel: {claude: opus, codex: model-code, cursor: model-cursor}\neffort: high\nargument-hint: '[file]'\nallowed-tools: [Read]\n---\nReview.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "deploy", "SKILL.md"), "---\nname: deploy\nmodel: shared-model\neffort: low\nargument-hint: '[version]'\nallowed-tools: [Read]\n---\nDeploy.\n")
	_, warnings, err := runGlobalAgentTest("--only", "claude,codex,cursor")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"model", "effort", "argument-hint", "allowed-tools"} {
		if !strings.Contains(warnings, "`"+field+"` on 2 skills has no effect on codex, cursor") {
			t.Errorf("missing %s coverage: %s", field, warnings)
		}
	}
}

const globalManualOnlySkill = "---\nname: gh-issues\ndescription: Work through ready issues, one PR each.\ndisable-model-invocation: true\nx-codex:\n  policy:\n    allow_implicit_invocation: false\n---\nProcess the queue.\n"

func TestSyncGlobal_WritesCodexSkillPolicySidecar(t *testing.T) {
	home, source := globalAgentTestHome(t)
	spec := filepath.Join(source, "skills", "gh-issues", "SKILL.md")
	mustWriteGlobalTest(t, spec, globalManualOnlySkill)
	sidecar := filepath.Join(home, ".agents", "skills", "gh-issues", "agents", "openai.yaml")
	for _, flag := range []string{"", "", "--check"} {
		args := []string{"--only", "claude,codex"}
		if flag != "" {
			args = append(args, flag)
		}
		if _, _, err := runGlobalAgentTest(args...); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "allow_implicit_invocation: false") || !strings.Contains(string(data), "Generated by agnostic-ai") {
		t.Errorf("codex policy sidecar: %s", data)
	}

	mustWriteGlobalTest(t, spec, "---\nname: gh-issues\ndescription: Work through ready issues, one PR each.\n---\nProcess the queue.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude,codex"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Errorf("sidecar kept after the policy left the spec: %v", err)
	}
}

func TestSyncGlobal_ReportsManualOnlySkillsThatStayModelInvocable(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "gh-issues", "SKILL.md"), globalManualOnlySkill)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "deploy", "SKILL.md"), "---\nname: deploy\ndescription: Deploy.\ndisable-model-invocation: true\n---\nDeploy.\n")
	_, warnings, err := runGlobalAgentTest("--only", "claude,codex,amp")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings, "`disable-model-invocation` on 2 skills has no effect on amp") {
		t.Errorf("missing amp coverage: %s", warnings)
	}
	for _, target := range []string{"claude", "codex"} {
		if strings.Contains(warnings, "has no effect on "+target) {
			t.Errorf("%s stays manual-only, yet noted: %s", target, warnings)
		}
	}
}

func TestSyncGlobal_ManualOnlySkillWritesCodexPolicyWithoutXCodex(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "deploy", "SKILL.md"), "---\nname: deploy\ndescription: Deploy.\ndisable-model-invocation: true\n---\nDeploy.\n")
	sidecar := filepath.Join(home, ".agents", "skills", "deploy", "agents", "openai.yaml")
	for _, only := range []string{"codex,amp", "amp"} {
		if _, _, err := runGlobalAgentTest("--only", only); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(sidecar)
		if err != nil {
			t.Fatalf("--only %s: %v", only, err)
		}
		if !strings.Contains(string(data), "allow_implicit_invocation: false") {
			t.Errorf("--only %s: codex policy sidecar: %s", only, data)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "codex,amp", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_PartialSyncOfSharedSkillsKeepsCodexPolicy(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "gh-issues", "SKILL.md"), globalManualOnlySkill)
	sidecar := filepath.Join(home, ".agents", "skills", "gh-issues", "agents", "openai.yaml")
	for _, only := range []string{"codex,amp", "amp", "amp"} {
		if _, _, err := runGlobalAgentTest("--only", only); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(sidecar); err != nil {
			t.Fatalf("--only %s removed the codex policy: %v", only, err)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "amp", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_BundledOpenAIYAMLYieldsToCodexPolicy(t *testing.T) {
	home, source := globalAgentTestHome(t)
	folder := filepath.Join(source, "skills", "gh-issues")
	mustWriteGlobalTest(t, filepath.Join(folder, "SKILL.md"), globalManualOnlySkill)
	mustWriteGlobalTest(t, filepath.Join(folder, "agents", "openai.yaml"), "policy:\n  allow_implicit_invocation: true\n")
	for _, only := range []string{"amp,codex", "codex,amp"} {
		if _, _, err := runGlobalAgentTest("--only", only); err != nil {
			t.Fatalf("--only %s: %v", only, err)
		}
		data, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "gh-issues", "agents", "openai.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "allow_implicit_invocation: false") {
			t.Errorf("--only %s: bundled asset won over the spec policy: %s", only, data)
		}
	}
}

func TestSyncGlobal_ExplicitCodexInvocationPolicySilencesManualOnlyNote(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "lint", "SKILL.md"), "---\nname: lint\ndescription: Lint.\ndisable-model-invocation: true\nx-codex:\n  policy:\n    allow_implicit_invocation: true\n---\nLint.\n")
	_, warnings, err := runGlobalAgentTest("--only", "codex")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(warnings, "disable-model-invocation") {
		t.Errorf("explicit codex policy still noted: %s", warnings)
	}
}

func TestSyncGlobal_ManualOnlySkillMergesPolicyIntoBundledOpenAIYAML(t *testing.T) {
	cases := []struct {
		name    string
		bundled string
		want    []string
	}{
		{"bundled interface keeps its fields", "interface:\n  display_name: Deploy UI\n", []string{"display_name: Deploy UI", "allow_implicit_invocation: false"}},
		{"bundled explicit policy wins", "policy:\n  allow_implicit_invocation: true\n", []string{"allow_implicit_invocation: true"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			folder := filepath.Join(source, "skills", "deploy")
			mustWriteGlobalTest(t, filepath.Join(folder, "SKILL.md"), "---\nname: deploy\ndescription: Deploy.\ndisable-model-invocation: true\n---\nDeploy.\n")
			mustWriteGlobalTest(t, filepath.Join(folder, "agents", "openai.yaml"), tc.bundled)
			if _, _, err := runGlobalAgentTest("--only", "codex,amp"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "deploy", "agents", "openai.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(string(data), want) {
					t.Errorf("openai.yaml missing %q:\n%s", want, data)
				}
			}
			if _, _, err := runGlobalAgentTest("--only", "codex,amp", "--check"); err != nil {
				t.Errorf("sync --check after sync: %v", err)
			}
		})
	}
}

func TestSyncGlobal_AmpAloneWritesMergedBundledOpenAIYAML(t *testing.T) {
	home, source := globalAgentTestHome(t)
	folder := filepath.Join(source, "skills", "deploy")
	mustWriteGlobalTest(t, filepath.Join(folder, "SKILL.md"), "---\nname: deploy\ndescription: Deploy.\ndisable-model-invocation: true\n---\nDeploy.\n")
	mustWriteGlobalTest(t, filepath.Join(folder, "agents", "openai.yaml"), "interface:\n  display_name: Deploy UI\n")
	if _, _, err := runGlobalAgentTest("--only", "amp"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "deploy", "agents", "openai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"display_name: Deploy UI", "allow_implicit_invocation: false"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("openai.yaml missing %q:\n%s", want, data)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "amp", "--check"); err != nil {
		t.Errorf("sync --check after sync: %v", err)
	}
}

func TestSyncGlobal_CodexCopiesAnUnparsableBundledOpenAIYAML(t *testing.T) {
	home, source := globalAgentTestHome(t)
	folder := filepath.Join(source, "skills", "deploy")
	mustWriteGlobalTest(t, filepath.Join(folder, "SKILL.md"), "---\nname: deploy\ndescription: Deploy.\n---\nDeploy.\n")
	mustWriteGlobalTest(t, filepath.Join(folder, "agents", "openai.yaml"), "- a\n")
	if _, _, err := runGlobalAgentTest("--only", "codex"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(home, ".agents", "skills", "deploy", "agents", "openai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "- a\n" {
		t.Errorf("openai.yaml not verbatim: %q", data)
	}
}

func TestSyncGlobal_ReportsOmittedFalseSkillInvocationFlag(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review.\ndisable-model-invocation: false\n---\nReview.\n")
	_, warnings, err := runGlobalAgentTest("--only", "claude,codex,cursor,amp")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(warnings, "`disable-model-invocation` on 1 skill has no effect on codex, amp") {
		t.Errorf("missing invocation field coverage: %s", warnings)
	}
	for _, target := range []string{"claude", "cursor"} {
		if strings.Contains(warnings, "has no effect on "+target) {
			t.Errorf("retained %s invocation flag noted: %s", target, warnings)
		}
	}
}

func TestSyncGlobal_NormalizesSkillCapabilitiesAndKeepsNativeOverride(t *testing.T) {
	for _, tc := range []struct{ meta, want string }{
		{"allowed-tools: [read, 'shell(git log --format=%h,%s)']\n", "allowed-tools:\n  - Read\n  - Bash(git log --format=%h,%s)"},
		{"allowed-tools: [read]\nx-claude:\n  allowed-tools: [Bash(pwd)]\n", "allowed-tools:\n  - Bash(pwd)"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\ndescription: Review code.\n"+tc.meta+"---\nReview code.\n")
			if _, warnings, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatalf("sync = %v, %s", err, warnings)
			}
			data, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "review", "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tc.want) {
				t.Errorf("skill = %s; want %s", data, tc.want)
			}
		})
	}
}

func TestSyncGlobal_RejectsMalformedSkillCapability(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\nallowed-tools: [raed]\n---\nReview code.\n")
	if _, warnings, err := runGlobalAgentTest("--only", "claude"); err == nil || !strings.Contains(err.Error(), "unknown capability") {
		t.Errorf("sync = %v, %s", err, warnings)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "review", "SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("invalid skill was emitted: %v", err)
	}
}

func TestSyncGlobal_UnsupportedSkillDeleteHonorsMode(t *testing.T) {
	for _, mode := range []string{"warn", "silent", "error"} {
		t.Run(mode, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			mustWriteGlobalTest(t, filepath.Join(source, "agnostic-ai.yaml"), "targets: [claude]\non-unsupported: "+mode+"\n")
			mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\nallowed-tools: [read, delete]\n---\nReview code.\n")
			_, warnings, err := runGlobalAgentTest("--only", "claude")
			if mode == "error" {
				if err == nil || !strings.Contains(err.Error(), "delete has no native") {
					t.Errorf("error = %v, %s", err, warnings)
				}
				return
			}
			if err != nil {
				t.Fatalf("sync = %v, %s", err, warnings)
			}
			if mode == "warn" && !strings.Contains(warnings, "delete has no native") {
				t.Errorf("warning = %s", warnings)
			}
			if mode == "silent" && strings.Contains(warnings, "delete has no native") {
				t.Errorf("silent = %s", warnings)
			}
			data, err := os.ReadFile(filepath.Join(home, ".claude", "skills", "review", "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "Delete") || strings.Contains(string(data), "- delete") || !strings.Contains(string(data), "- Read") {
				t.Errorf("skill = %s", data)
			}
		})
	}
}
