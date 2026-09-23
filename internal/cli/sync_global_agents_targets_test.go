package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Paths and formats are asserted independently of the global target table.
func TestSyncGlobal_AgentsUseNativeFormatsForEverySupportedTarget(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review changes\n---\nReview carefully.\n")
	_, warnings, err := runGlobalAgentTest()
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]string{
		"claude":      ".claude/agents/reviewer.md",
		"codex":       ".codex/agents/reviewer.toml",
		"cursor":      ".cursor/agents/reviewer.md",
		"gemini":      ".gemini/agents/reviewer.md",
		"copilot":     ".copilot/agents/reviewer.agent.md",
		"cline":       ".cline/agents/reviewer.yml",
		"opencode":    ".config/opencode/agents/reviewer.md",
		"antigravity": ".gemini/config/agents/reviewer/agent.md",
		"qoder":       ".qoder/agents/reviewer.md",
		"factory":     ".factory/droids/reviewer.md",
		"kiro":        ".kiro/agents/reviewer.md",
		"junie":       ".junie/agents/reviewer.md",
		"augment":     ".augment/agents/reviewer.md",
		"kilo":        ".config/kilo/agents/reviewer.md",
		"windsurf":    ".config/devin/agents/reviewer.md",
		"goose":       ".agents/agents/reviewer.md",
		"openhands":   ".agents/agents/reviewer.md",
		"trae":        ".trae-cn/agents/reviewer.md",
	}
	if runtime.GOOS == "windows" {
		paths["windsurf"] = "AppData/Roaming/devin/agents/reviewer.md"
	}
	for target, rel := range paths {
		data, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(rel)))
		if err != nil {
			t.Errorf("%s: %v", target, err)
			continue
		}
		if !strings.Contains(string(data), "Review carefully.") {
			t.Errorf("%s missing prompt: %s", target, data)
		}
		if target == "codex" {
			for _, field := range []string{`name = "reviewer"`, `description = "Review changes"`, "developer_instructions ="} {
				if !strings.Contains(string(data), field) {
					t.Errorf("Codex missing TOML field %q: %s", field, data)
				}
			}
		} else if !strings.HasPrefix(string(data), "---\n") {
			t.Errorf("%s lacks native frontmatter: %s", target, data)
		}
		if strings.Contains(warnings, target+": global agents are unsupported") {
			t.Errorf("supported %s warned: %s", target, warnings)
		}
	}
	for _, target := range []string{"amp", "zed", "warp", "crush"} {
		if !strings.Contains(warnings, target+": global agents are unsupported") {
			t.Errorf("%s missing unsupported diagnostic: %s", target, warnings)
		}
	}
	if _, _, err := runGlobalAgentTest("--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_SharedAgentsKeepUnselectedOwnership(t *testing.T) {
	home, source := globalAgentTestHome(t)
	input := filepath.Join(source, "agents", "reviewer.md")
	mustWriteGlobalTest(t, input, "---\nname: reviewer\n---\nReview.\n")
	if _, _, err := runGlobalAgentTest("--only", "goose,openhands"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".agents", "agents", "reviewer.md")
	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "goose"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("unselected OpenHands agent removed: %v", err)
	}
	if _, _, err := runGlobalAgentTest("--only", "openhands"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("agent retained after last owner removed it: %v", err)
	}
}

func TestSyncGlobal_SharedAgentConflictsStopBeforeWrites(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\nx-goose:\n  model: first\nx-openhands:\n  model: second\n---\nReview.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude,goose,openhands"); err == nil || !strings.Contains(err.Error(), "different content") {
		t.Fatalf("want shared conflict: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Error("wrote before detecting conflict")
	}
}

func TestSyncGlobal_SharedAgentUpdateRequiresAllOwners(t *testing.T) {
	home, source := globalAgentTestHome(t)
	input := filepath.Join(source, "agents", "reviewer.md")
	mustWriteGlobalTest(t, input, "---\nname: reviewer\n---\nOriginal.\n")
	if _, _, err := runGlobalAgentTest("--only", "goose,openhands"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".agents", "agents", "reviewer.md")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, input, "---\nname: reviewer\n---\nChanged.\n")
	if _, _, err := runGlobalAgentTest("--only", "goose"); err == nil || !strings.Contains(err.Error(), "unselected target openhands") {
		t.Fatalf("want shared owner conflict: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Errorf("partial sync changed shared agent: %s, %v", after, err)
	}
	if _, _, err := runGlobalAgentTest("--only", "goose,openhands"); err != nil {
		t.Fatal(err)
	}
	after, err = os.ReadFile(path)
	if err != nil || !strings.Contains(string(after), "Changed.") {
		t.Errorf("full owner sync did not update shared agent: %s, %v", after, err)
	}
}

func TestSyncGlobal_AgentsHonorNativeRootOverrides(t *testing.T) {
	for _, tc := range []struct{ target, env, rel string }{
		{"claude", "CLAUDE_CONFIG_DIR", "agents/reviewer.md"},
		{"codex", "CODEX_HOME", "agents/reviewer.toml"},
		{"gemini", "GEMINI_CLI_HOME", ".gemini/agents/reviewer.md"},
		{"copilot", "COPILOT_HOME", "agents/reviewer.agent.md"},
		{"cline", "CLINE_DIR", "agents/reviewer.yml"},
		{"qoder", "QODER_CONFIG_DIR", "agents/reviewer.md"},
		{"kiro", "KIRO_HOME", "agents/reviewer.md"},
		{"junie", "JUNIE_HOME", "agents/reviewer.md"},
		{"opencode", "XDG_CONFIG_HOME", "opencode/agents/reviewer.md"},
		{"kilo", "XDG_CONFIG_HOME", "kilo/agents/reviewer.md"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			defaultPath := globalTargets[tc.target].agentsPath(home)
			nativeRoot := t.TempDir()
			t.Setenv(tc.env, nativeRoot)
			mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review\n---\nReview.\n")
			if _, _, err := runGlobalAgentTest("--only", tc.target); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(nativeRoot, filepath.FromSlash(tc.rel))
			data, err := os.ReadFile(path)
			if err != nil || !strings.Contains(string(data), "Review.") {
				t.Fatalf("native override output: %s, %v", data, err)
			}
			if _, err := os.Stat(defaultPath); !os.IsNotExist(err) {
				t.Errorf("wrote default agent directory: %v", err)
			}
			if _, _, err := runGlobalAgentTest("--only", tc.target, "--check"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSyncGlobal_AgentRenderingDoesNotLoadProjectConfiguration(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, "agnostic-ai.yaml", "not: valid: yaml")
	mustWriteGlobalTest(t, ".agnostic-ai/agents/project.md", "---\nname: project\n---\nProject only.\n")
	mustWriteGlobalTest(t, ".agnostic-ai/overlays/claude.settings.json", "invalid JSON")
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review\n---\nReview.\n")
	if _, _, err := runGlobalAgentTest(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(".claude"); !os.IsNotExist(err) {
		t.Errorf("global render wrote project files: %v", err)
	}
}

func TestSyncGlobal_AgentsRejectRelativeNativeRoots(t *testing.T) {
	_, source := globalAgentTestHome(t)
	t.Setenv("QODER_CONFIG_DIR", ".qoder")
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\n---\nReview.\n")
	if _, _, err := runGlobalAgentTest("--only", "qoder"); err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("want relative root rejection: %v", err)
	}
	if _, err := os.Stat(".qoder"); !os.IsNotExist(err) {
		t.Error("relative root wrote project output")
	}
}

func TestSyncGlobal_DefaultRunSkipsTargetWithRelativeRoot(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("QODER_CONFIG_DIR", ".qoder")
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Be terse.\n")
	_, warnings, err := runGlobalAgentTest()
	if err != nil {
		t.Fatalf("relative qoder root failed the default run: %v", err)
	}
	if !strings.Contains(warnings, "skipping qoder") {
		t.Errorf("missing skip warning:\n%s", warnings)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "CLAUDE.md")); err != nil {
		t.Errorf("other targets not synced: %v", err)
	}
	if _, err := os.Stat(".qoder"); !os.IsNotExist(err) {
		t.Error("relative root wrote project output")
	}
}

func TestSyncGlobal_RootOverrideMovesEverySurface(t *testing.T) {
	home, source := globalAgentTestHome(t)
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Be terse.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "tidy", "SKILL.md"), "---\nname: tidy\ndescription: Tidy\n---\nTidy up.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "hooks", "start.yaml"), "name: start\nevent: SessionStart\ncommand: echo hi\n")
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), "---\nname: reviewer\ndescription: Review\n---\nReview.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"CLAUDE.md", "skills/tidy/SKILL.md", "settings.json", "agents/reviewer.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s not under the override root: %v", rel, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("wrote the default root: %v", err)
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_DefaultRunSkipsAgentsANativeFormatRejects(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "code_reviewer.md"), "---\nname: code_reviewer\ndescription: Review\n---\nReview.\n")
	_, warnings, err := runGlobalAgentTest()
	if err != nil {
		t.Fatalf("one strict target failed the default run: %v", err)
	}
	if !strings.Contains(warnings, "skipping trae agents") {
		t.Errorf("missing trae skip warning:\n%s", warnings)
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "agents", "code_reviewer.md")); err != nil {
		t.Errorf("claude agent not written: %v", err)
	}
	if _, _, err := runGlobalAgentTest("--only", "trae"); err == nil {
		t.Error("explicit trae run accepted a name its format rejects")
	}
}

func TestSyncGlobal_CheckAcceptsOlderStateVersion(t *testing.T) {
	_, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "Be terse.\n")
	if _, _, err := runGlobalAgentTest(); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(source, "state", "global.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	older := strings.Replace(string(data), `"version": 5`, `"version": 4`, 1)
	if older == string(data) {
		t.Fatalf("state has no version 5 marker:\n%s", data)
	}
	mustWriteGlobalTest(t, statePath, older)
	if _, _, err := runGlobalAgentTest("--check"); err != nil {
		t.Errorf("version 4 state with the same ownership reported drift: %v", err)
	}
}

func TestSyncGlobal_RemovedFolderAgentLeavesNoEmptyDirectory(t *testing.T) {
	home, source := globalAgentTestHome(t)
	agent := filepath.Join(source, "agents", "reviewer.md")
	mustWriteGlobalTest(t, agent, "---\nname: reviewer\ndescription: Review\n---\nReview.\n")
	if _, _, err := runGlobalAgentTest("--only", "antigravity"); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".gemini", "config", "agents", "reviewer")
	if _, err := os.Stat(filepath.Join(dir, "agent.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(agent); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "antigravity"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("left an empty agent folder behind: %v", err)
	}
}
