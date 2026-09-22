package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func globalAgentTestHome(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	for _, key := range []string{"CLAUDE_CONFIG_DIR", "CODEX_HOME", "GEMINI_CLI_HOME", "COPILOT_HOME", "CLINE_DIR", "QODER_CONFIG_DIR", "KIRO_HOME", "JUNIE_HOME"} {
		t.Setenv(key, "")
	}
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	source := filepath.Join(home, "source")
	t.Setenv("AGNOSTIC_AI_HOME", source)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	testutil.Chdir(t, t.TempDir())
	return home, source
}

func runGlobalAgentTest(args ...string) (string, string, error) {
	var out, warnings bytes.Buffer
	cmd := NewRootCmd("test")
	cmd.SetOut(&out)
	cmd.SetErr(&warnings)
	cmd.SetArgs(append([]string{"sync", "--global"}, args...))
	err := cmd.Execute()
	return out.String(), warnings.String(), err
}

const globalReviewer = `---
name: reviewer
description: Review code
model:
  claude: sonnet
  default: other-model
tools: [Read, Grep]
x-claude:
  model: opus
  permissionMode: plan
x-codex:
  model: codex-only
---
Review carefully.
::target claude
Claude detail.
::end
::target codex
Codex detail.
::end
`

func TestSyncGlobal_AgentWorksOutsideProject(t *testing.T) {
	for _, defaultRoot := range []bool{true, false} {
		name := "override"
		if defaultRoot {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			if defaultRoot {
				t.Setenv("AGNOSTIC_AI_HOME", "")
				source = filepath.Join(home, ".agnostic-ai")
			}
			mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), globalReviewer)
			path := filepath.Join(home, ".claude", "agents", "reviewer.md")
			state := filepath.Join(source, "state", "global.json")
			out, _, err := runGlobalAgentTest("--only", "claude", "--dry-run")
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, path) {
				t.Errorf("preview missing agent: %s", out)
			}
			for _, p := range []string{path, state} {
				if _, err := os.Stat(p); !os.IsNotExist(err) {
					t.Errorf("dry-run wrote %s", p)
				}
			}
			if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err == nil {
				t.Error("check accepted missing agent")
			}
			if _, err := os.Stat(state); !os.IsNotExist(err) {
				t.Error("check wrote state")
			}
			if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"name: reviewer", "description: Review code", "model: opus", "permissionMode: plan", "Read", "Grep", "Review carefully.", "Claude detail."} {
				if !strings.Contains(string(data), want) {
					t.Errorf("agent missing %q:\n%s", want, data)
				}
			}
			for _, unwanted := range []string{"x-claude", "x-codex", "other-model", "codex-only", "Codex detail.", "::target"} {
				if strings.Contains(string(data), unwanted) {
					t.Errorf("agent contains %q:\n%s", unwanted, data)
				}
			}
			beforeState, err := os.ReadFile(state)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			afterState, err := os.ReadFile(state)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, after) || !bytes.Equal(beforeState, afterState) {
				t.Error("second sync changed output or state")
			}
			if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
				t.Fatal(err)
			}
			mustWriteGlobalTest(t, path, "changed\n")
			if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err == nil || !strings.Contains(err.Error(), path) {
				t.Errorf("want agent drift: %v", err)
			}
			unchanged, err := os.ReadFile(path)
			if err != nil || string(unchanged) != "changed\n" {
				t.Errorf("check rewrote agent: %s, %v", unchanged, err)
			}
			if _, _, err := runGlobalAgentTest("--only", "claude", "--backup"); err != nil {
				t.Fatal(err)
			}
			backup, err := os.ReadFile(path + ".bak")
			if err != nil || string(backup) != "changed\n" {
				t.Errorf("backup = %q, %v", backup, err)
			}
		})
	}
}

func TestSyncGlobal_AgentCollisionStopsAllWrites(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), globalReviewer)
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "New instructions.\n")
	path := filepath.Join(home, ".claude", "agents", "reviewer.md")
	mustWriteGlobalTest(t, path, "Personal agent.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude"); err == nil || !strings.Contains(err.Error(), "unmanaged") {
		t.Fatalf("want collision: %v", err)
	}
	for _, p := range []string{filepath.Join(home, ".claude", "CLAUDE.md"), filepath.Join(source, "state", "global.json")} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("collision wrote %s", p)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "Personal agent.\n" {
		t.Errorf("personal agent changed: %s, %v", data, err)
	}
}

func TestSyncGlobal_AgentCleanupHonorsSelectionAndOwnership(t *testing.T) {
	home, source := globalAgentTestHome(t)
	agentSource := filepath.Join(source, "agents", "reviewer.md")
	mustWriteGlobalTest(t, agentSource, globalReviewer)
	personal := filepath.Join(home, ".claude", "agents", "personal.md")
	mustWriteGlobalTest(t, personal, "Personal agent.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills", "review", "SKILL.md"), "---\nname: review\n---\nReview.\n")
	if _, _, err := runGlobalAgentTest("--only", "claude,cursor"); err != nil {
		t.Fatal(err)
	}
	agent := filepath.Join(home, ".claude", "agents", "reviewer.md")
	if err := os.Remove(agentSource); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runGlobalAgentTest("--only", "cursor"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(agent); err != nil {
		t.Fatalf("unselected agent removed: %v", err)
	}
	for _, flag := range []string{"--dry-run", "--check"} {
		_, _, err := runGlobalAgentTest("--only", "claude", flag)
		if flag == "--check" && err == nil {
			t.Error("check missed removed source")
		}
		if flag == "--dry-run" && err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(agent); err != nil {
			t.Errorf("%s removed agent: %v", flag, err)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(agent); !os.IsNotExist(err) {
		t.Errorf("removed source still emitted: %v", err)
	}
	for _, p := range []string{personal, filepath.Join(home, ".cursor", "skills", "review", "SKILL.md")} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("unrelated file removed: %s: %v", p, err)
		}
	}
	if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
		t.Fatal(err)
	}
}

func TestSyncGlobal_AgentFiltersAndUnsupportedWarnings(t *testing.T) {
	home, source := globalAgentTestHome(t)
	for name, routing := range map[string]string{
		"claude-only": "targets: [claude]", "excluded": "targets: [claude, amp]\ntarget-exclude: claude", "shared": "", "elsewhere": "target: gemini",
	} {
		mustWriteGlobalTest(t, filepath.Join(source, "agents", name+".md"), "---\nname: "+name+"\n"+routing+"\n---\nReview.\n")
	}
	_, warnings, err := runGlobalAgentTest("--only", "claude,amp")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"claude-only", "shared"} {
		if _, err := os.Stat(filepath.Join(home, ".claude", "agents", name+".md")); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}
	for _, name := range []string{"excluded", "elsewhere"} {
		if _, err := os.Stat(filepath.Join(home, ".claude", "agents", name+".md")); !os.IsNotExist(err) {
			t.Errorf("emitted filtered %s", name)
		}
	}
	for _, want := range []string{"amp", "global agents", "excluded", "shared"} {
		if !strings.Contains(warnings, want) {
			t.Errorf("missing warning %q: %s", want, warnings)
		}
	}
	for _, unwanted := range []string{"claude-only", "elsewhere"} {
		if strings.Contains(warnings, unwanted) {
			t.Errorf("warned for filtered %s: %s", unwanted, warnings)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".amp", "agents")); !os.IsNotExist(err) {
		t.Error("invented Cursor global path")
	}
	_, warnings, err = runGlobalAgentTest("--target", "claude,amp", "--except", "amp")
	if err != nil || warnings != "" {
		t.Errorf("excluded target warned: %s, %v", warnings, err)
	}
}

func TestSyncGlobal_AgentsPreserveExistingStateAndNativeSettings(t *testing.T) {
	for _, version := range []int{1, 2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			home, source := globalAgentTestHome(t)
			unselected := filepath.Join(home, ".cursor", "skills", "review", "SKILL.md")
			mustWriteGlobalTest(t, unselected, "Existing skill.\n")
			statePath := filepath.Join(source, "state", "global.json")
			old, err := json.Marshal(globalState{Version: version, Files: []string{unselected}})
			if err != nil {
				t.Fatal(err)
			}
			mustWriteGlobalTest(t, statePath, string(old))
			settings := filepath.Join(home, ".claude", "settings.json")
			mustWriteGlobalTest(t, settings, `{"model":"sonnet","hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"user-command"}]}]}}`)
			mustWriteGlobalTest(t, filepath.Join(source, "agents", "reviewer.md"), globalReviewer)
			if _, _, err := runGlobalAgentTest("--only", "claude"); err != nil {
				t.Fatal(err)
			}
			next, err := loadGlobalState(statePath)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{unselected, filepath.Join(home, ".claude", "agents", "reviewer.md")} {
				if !slices.Contains(next.Files, want) {
					t.Errorf("ownership missing %s: %v", want, next.Files)
				}
			}
			data, err := os.ReadFile(settings)
			if err != nil || !strings.Contains(string(data), "user-command") || !strings.Contains(string(data), "sonnet") {
				t.Errorf("native settings lost: %s, %v", data, err)
			}
			if _, _, err := runGlobalAgentTest("--only", "claude", "--check"); err != nil {
				t.Fatal(err)
			}
		})
	}
}
