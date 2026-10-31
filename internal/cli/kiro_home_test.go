package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestKiroHome_UsesNativeFallbackAcrossSurfaces(t *testing.T) {
	home, _ := globalAgentTestHome(t)
	g := globalTargets["kiro"]
	paths := []string{g.instructions, g.agents, g.skills, g.hooksDir, g.hookScriptsDir, globalPathHome + ".kiro/settings/cli.json"}
	cases := []struct {
		name, root string
		fallback   bool
	}{
		{"unset", "", true},
		{"relative", "profiles/work", true},
		{"filesystem root", filepath.VolumeName(home) + string(filepath.Separator), true},
		{"home", home, true},
		{"home ancestor", filepath.Dir(home), true},
		{"cleaned home", home + string(filepath.Separator) + "profiles" + string(filepath.Separator) + "..", true},
		{"profile", filepath.Join(home, "profiles", "work"), false},
		{"sibling prefix", home + "-profile", false},
	}
	if runtime.GOOS == "windows" {
		cases = append(cases, struct {
			name, root string
			fallback   bool
		}{"UNC root", `\\server\share\`, true})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("KIRO_HOME", tc.root)
			if err := g.rootError("kiro"); err != nil {
				t.Errorf("native fallback rejected: %v", err)
			}
			for _, path := range paths {
				want := globalPath(home, path)
				if !tc.fallback {
					want = filepath.Join(tc.root, filepath.FromSlash(path[len(g.root):]))
				}
				if got := g.path(home, path); got != want {
					t.Errorf("%s resolves to %q, want %q", path, got, want)
				}
			}
			if tc.root != "" && filepath.Clean(tc.root) != filepath.Clean(home) && tc.fallback && slices.Contains(globalRoots(home), filepath.Clean(tc.root)) {
				t.Errorf("invalid profile still expands ownership roots: %q", tc.root)
			}
		})
	}
}

func TestKiroHome_OtherTargetsStillRejectRelativeRoots(t *testing.T) {
	globalAgentTestHome(t)
	for name, g := range globalTargets {
		if g.rootEnv == "" || name == "kiro" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			t.Setenv(g.rootEnv, "profiles/work")
			if err := g.rootError(name); err == nil {
				t.Errorf("%s accepted relative %s", name, g.rootEnv)
			}
		})
	}
}

func TestKiroHome_RefusesForeignPriorStateWithoutChangingFiles(t *testing.T) {
	home, source := globalAgentTestHome(t)
	t.Setenv("KIRO_HOME", filepath.Dir(home))
	oldPath := filepath.Join(t.TempDir(), "steering", "AGENTS.md")
	original := "User-owned file outside the fallback root.\n"
	mustWriteGlobalTest(t, oldPath, original)
	statePath := filepath.Join(source, "state", "global.json")
	state, err := json.Marshal(globalState{Version: globalStateVersion, Files: []string{oldPath}})
	if err != nil {
		t.Fatal(err)
	}
	mustWriteGlobalTest(t, statePath, string(state))
	mustWriteGlobalTest(t, filepath.Join(source, "AGNOSTIC_AI.md"), "New instructions.\n")
	if _, _, err := runGlobalAgentTest("--only", "kiro"); err == nil || !strings.Contains(err.Error(), "recorded under another home") || !strings.Contains(err.Error(), oldPath) {
		t.Fatalf("unsafe prior state accepted: %v", err)
	}
	if got := readGlobalTest(t, oldPath); got != original {
		t.Errorf("prior file changed: %q", got)
	}
	if got := readGlobalTest(t, statePath); got != string(state) {
		t.Error("blocked sync changed recorded ownership")
	}
	if _, err := os.Stat(filepath.Join(home, ".kiro", "steering", "AGENTS.md")); !os.IsNotExist(err) {
		t.Errorf("blocked sync created output: %v", err)
	}
}
