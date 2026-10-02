package integration

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_LinkedSourceRootsKeepAssetsAndLexicalProvenance(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	cases := []struct {
		name      string
		absolute  bool
		rootSkill bool
	}{
		{name: "relative root"},
		{name: "absolute root", absolute: true},
		{name: "root-level skill", rootSkill: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			testutil.Chdir(t, project)
			source := t.TempDir()
			alias := filepath.Join(project, "skills-alias")
			configured := "skills-alias"
			if tc.absolute {
				alias = filepath.Join(t.TempDir(), "skills-alias")
				configured = filepath.ToSlash(alias)
			}
			skillDir := "demo"
			if tc.rootSkill {
				skillDir = ""
			}
			const skill = "---\nname: demo\ndescription: Read shared guidance.\n---\nShared guidance.\n"
			const asset = "Asset from the shared source.\n"
			assetRel := filepath.Join(skillDir, "references", "guide.md")
			must(t, os.MkdirAll(filepath.Dir(filepath.Join(source, assetRel)), 0o755))
			must(t, os.WriteFile(filepath.Join(source, skillDir, "SKILL.md"), []byte(skill), 0o644))
			must(t, os.WriteFile(filepath.Join(source, assetRel), []byte(asset), 0o644))
			outside := t.TempDir()
			must(t, os.WriteFile(filepath.Join(outside, "SKILL.md"), []byte(strings.ReplaceAll(skill, "demo", "foreign")), 0o644))
			testutil.DirectoryAlias(t, outside, filepath.Join(source, "nested-link"))
			testutil.DirectoryAlias(t, source, alias)
			config := "version: 1\ntargets: [claude]\nsources:\n  skills: " + configured + "\n"
			must(t, os.WriteFile("agnostic-ai.yaml", []byte(config), 0o644))
			run := func(args ...string) {
				t.Helper()
				cmd := exec.Command(binary, args...)
				cmd.Dir = project
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("%v: %v\n%s", args, err, out)
				}
			}
			run("sync", "--gitignore=off")
			assertContains(t, filepath.Join(project, ".claude", "skills", "demo", "SKILL.md"), "Shared guidance.")
			assertContains(t, filepath.Join(project, ".claude", "skills", "demo", "references", "guide.md"), asset)
			if _, err := os.Stat(filepath.Join(project, ".claude", "skills", "foreign", "SKILL.md")); !os.IsNotExist(err) {
				t.Errorf("sync traversed nested directory link: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(project, ".agnostic-ai", ".sync-state"))
			must(t, err)
			var state struct {
				Files map[string]json.RawMessage `json:"spec_file_sums"`
			}
			must(t, json.Unmarshal(data, &state))
			for _, rel := range []string{filepath.Join(skillDir, "SKILL.md"), assetRel} {
				key := filepath.ToSlash(filepath.Join(configured, rel))
				if _, ok := state.Files[key]; !ok {
					t.Errorf("provenance lacks configured path %q: %v", key, state.Files)
				}
			}
			for key := range state.Files {
				if strings.HasPrefix(key, filepath.ToSlash(source)+"/") {
					t.Errorf("provenance used physical target %q", key)
				}
			}
			run("sync", "--check", "--gitignore=off")
			run("lint")
		})
	}
}
