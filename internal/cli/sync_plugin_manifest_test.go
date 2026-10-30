package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSyncPluginManifest_UnownedStaysUnclaimed(t *testing.T) {
	for _, target := range []string{"antigravity", "goose"} {
		t.Run(target, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			output := "    skills-dir: .agents/plugins/team/skills\n"
			if target == "antigravity" {
				output = "    mcp-file: .agents/plugins/team/mcp.json\n"
			}
			mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\noutputs:\n  "+target+":\n"+output)
			mustWriteFile(t, ".agnostic-ai/skills/probe/SKILL.md", "---\nname: probe\ndescription: Probe.\n---\nProbe.\n")
			mustWriteFile(t, ".agnostic-ai/mcps/probe.yaml", "name: probe\ncommand: probe\n")
			path := filepath.FromSlash(".agents/plugins/team/plugin.json")
			want := "{\n  \"name\": \"custom\", \"version\": \"2.0.0\"\n}\n"
			mustWriteFile(t, path, want)
			for range 2 {
				runSyncOK(t)
				if got := readFile(t, path); got != want {
					t.Errorf("unowned manifest changed: %s", got)
				}
				state := readStateFile(".")
				if slices.Contains(state.Outputs, path) || state.OutputSums[path] != "" {
					t.Errorf("sync claimed unowned manifest: %v", state.Outputs)
				}
			}
			runSyncOK(t, "--check")
		})
	}
}

func TestSyncAntigravity_LegacyPluginManifestSurvivesNextSync(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [antigravity]\noutputs:\n  antigravity:\n    mcp-file: .agents/plugins/team/mcp_config.json\n")
	mustWriteFile(t, ".agnostic-ai/mcps/probe.yaml", "name: probe\ncommand: probe\n")
	path := filepath.FromSlash(".agents/plugins/team/plugin.json")
	for range 2 {
		runSyncOK(t)
		if got := readFile(t, path); !strings.Contains(got, `"name": "team"`) {
			t.Errorf("unexpected manifest: %s", got)
		}
		if !slices.Contains(readStateFile(".").Outputs, path) {
			t.Error("legacy plugin manifest lost sync ownership")
		}
		if _, err := os.Stat(path + ".bak"); !os.IsNotExist(err) {
			t.Errorf("unexpected manifest backup: %v", err)
		}
	}
	runSyncOK(t, "--check")
}
