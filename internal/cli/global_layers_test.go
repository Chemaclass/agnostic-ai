package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSyncGlobal_LocalLayerReplacesSharedSpecsAndAppendsAgreements(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Shared agreements.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "reviewer", "SKILL.md"), "---\nname: reviewer\nmodel: opus\n---\nShared skill.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "reviewer", "shared.txt"), "shared asset\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\nmodel: shared-model\n---\nShared agent.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "rules", "reviewer.md"), "---\nname: reviewer\n---\nShared rule.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "reviewer.yaml"), "name: reviewer\nevent: SessionStart\ncommand: shared-command\n")
	if _, _, err := runGlobalAgentTest("--only", "claude,codex"); err != nil {
		t.Fatal(err)
	}

	local := filepath.Join(source, "local")
	mustWriteGlobalTest(t, filepath.Join(local, "AGNOSTIC_AI.md"), "Personal agreements.\n")
	mustWriteGlobalTest(t, filepath.Join(local, "skills", "reviewer", "SKILL.md"), "---\nname: reviewer\n---\nLocal skill.\n")
	mustWriteGlobalTest(t, filepath.Join(local, "skills", "reviewer", "local.txt"), "local asset\n")
	mustWriteGlobalTest(t, filepath.Join(local, "agents", "reviewer.md"), "---\nname: reviewer\n---\nLocal agent.\n")
	mustWriteGlobalTest(t, filepath.Join(local, "rules", "reviewer.md"), "---\nname: reviewer\n---\nLocal rule.\n")
	mustWriteGlobalTest(t, filepath.Join(local, "rules", "personal.md"), "---\nname: personal\n---\nAdditional local rule.\n")
	mustWriteGlobalTest(t, filepath.Join(local, "hooks", "reviewer.yaml"), "name: reviewer\nevent: SessionStart\ncommand: local-command\n")
	for _, flag := range []string{"", "", "--check"} {
		args := []string{"--only", "claude,codex"}
		if flag != "" {
			args = append(args, flag)
		}
		if _, _, err := runGlobalAgentTest(args...); err != nil {
			t.Fatal(err)
		}
	}
	for path, expected := range map[string]string{
		".claude/skills/reviewer/SKILL.md": "Local skill.",
		".agents/skills/reviewer/SKILL.md": "Local skill.",
		".claude/agents/reviewer.md":       "Local agent.",
		".codex/agents/reviewer.toml":      "Local agent.",
		".claude/settings.json":            "local-command",
		".codex/hooks.json":                "local-command",
	} {
		data, err := os.ReadFile(filepath.Join(home, path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), expected) || strings.Contains(string(data), "Shared") || strings.Contains(string(data), "shared-") || strings.Contains(string(data), "model") {
			t.Errorf("%s did not replace shared spec: %s", path, data)
		}
	}
	for _, dir := range []string{".claude", ".agents"} {
		if _, err := os.Stat(filepath.Join(home, dir, "skills", "reviewer", "shared.txt")); !os.IsNotExist(err) {
			t.Errorf("stale shared asset in %s: %v", dir, err)
		}
		data, err := os.ReadFile(filepath.Join(home, dir, "skills", "reviewer", "local.txt"))
		if err != nil || string(data) != "local asset\n" {
			t.Errorf("local asset in %s: %q, %v", dir, data, err)
		}
	}
	for _, path := range []string{".claude/CLAUDE.md", ".codex/AGENTS.md"} {
		data, err := os.ReadFile(filepath.Join(home, path))
		if err != nil {
			t.Fatal(err)
		}
		got := string(data)
		if !strings.Contains(got, "Shared agreements.") || !strings.Contains(got, "Local rule.") || !strings.Contains(got, "Additional local rule.") || strings.Contains(got, "Shared rule.") {
			t.Errorf("wrong effective rules in %s: %s", path, got)
		}
		if !strings.Contains(got, "Personal agreements.\n"+globalEnd) {
			t.Errorf("local agreements must end managed block in %s: %s", path, got)
		}
	}
}

func TestListGlobal_ReportsEffectiveLayersWithoutProjectConfig(t *testing.T) {
	for _, defaultRoot := range []bool{false, true} {
		t.Run(map[bool]string{false: "override", true: "default"}[defaultRoot], func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			if defaultRoot {
				t.Setenv("AGNOSTIC_AI_HOME", "")
				source = filepath.Join(home, ".agnostic-ai")
			}
			mustWriteGlobalTest(t, filepath.Join(source, "skills", "reviewer", "SKILL.md"), "---\nname: reviewer\n---\nShared skill.\n")
			mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\n---\nShared agent.\n")
			mustWriteGlobalTest(t, filepath.Join(source, "local", "skills", "reviewer", "SKILL.md"), "---\nname: reviewer\n---\nLocal skill.\n")
			mustWriteGlobalTest(t, filepath.Join(source, "local", "rules", "personal.md"), "---\nname: personal\n---\nPersonal rule.\n")
			root := NewRootCmd("test")
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetArgs([]string{"list", "--global"})
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"agent\treviewer\tglobal\n", "skill\treviewer\tglobal-local\n", "rule\tpersonal\tglobal-local\n"} {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q in %s", want, out.String())
				}
			}
			if strings.Count(out.String(), "skill\treviewer\t") != 1 {
				t.Errorf("duplicate overridden skill: %s", out.String())
			}
		})
	}
}

func TestSyncGlobal_SharedHomeWithoutLocalLayer(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Shared agreements.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "reviewer", "SKILL.md"), "---\nname: reviewer\n---\nShared skill.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude,codex"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude,codex", "--check"); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{".claude/skills/reviewer/SKILL.md", ".agents/skills/reviewer/SKILL.md"} {
		data, err := os.ReadFile(filepath.Join(home, path))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "Shared skill.") {
			t.Errorf("shared-only home: %s", data)
		}
	}
	if _, err := os.Stat(filepath.Join(source, "local")); !os.IsNotExist(err) {
		t.Errorf("sync created a local layer: %v", err)
	}
}
