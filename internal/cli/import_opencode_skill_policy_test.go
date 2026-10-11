package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestImportOpencodeSkillInvocationPolicyRoundTrip(t *testing.T) {
	for _, front := range []string{
		"disable-model-invocation: true\n",
		"disable-model-invocation: false\n",
		"metadata:\n  opencode/autoinvoke: \"false\"\n",
		"disable-model-invocation: true\nmetadata:\n  opencode/autoinvoke: \"true\"\n",
	} {
		t.Run(strings.ReplaceAll(front, "\n", " "), func(t *testing.T) {
			dir := testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
			native := "---\nname: release\ndescription: Release on request.\n" + front + "---\nRelease body.\n"
			writeFile(t, ".opencode/skills/release/SKILL.md", native)
			writeFile(t, ".opencode/skills/release/assets/check.txt", "asset\n")
			execCLI(t, "import", "opencode")
			execCLI(t, "lint")
			execCLI(t, "sync")
			execCLI(t, "sync", "--check")
			data, err := os.ReadFile(filepath.Join(dir, ".opencode/skills/release/SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			expected, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(native))
			if err != nil {
				t.Fatal(err)
			}
			actual, err := spec.ParseMarkdownBytes(spec.KindSkill, data)
			if err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"disable-model-invocation", "metadata"} {
				if !reflect.DeepEqual(actual.Meta[field], expected.Meta[field]) {
					t.Errorf("%s changed: got %#v, want %#v", field, actual.Meta[field], expected.Meta[field])
				}
			}
			if !strings.HasSuffix(strings.TrimSpace(actual.Body), strings.TrimSpace(expected.Body)) {
				t.Errorf("body changed: %q", actual.Body)
			}
			if got := readFile(t, ".opencode/skills/release/assets/check.txt"); got != "asset\n" {
				t.Errorf("asset changed: %q", got)
			}
		})
	}
}

func TestImportOpencodeSkillDeletedPolicyStaysDeleted(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
	writeFile(t, ".agnostic-ai/skills/release/SKILL.md", "---\nname: release\ndescription: Release.\ndisable-model-invocation: true\n---\nBody.\n")
	execCLI(t, "sync")
	writeFile(t, ".opencode/skills/release/SKILL.md", "---\nname: release\ndescription: Release.\n---\nBody.\n")
	execCLI(t, "import", "opencode")
	if got := readFile(t, ".agnostic-ai/skills/release/SKILL.md"); strings.Contains(got, "disable-model-invocation:") {
		t.Errorf("deleted native policy restored: %s", got)
	}
}

func TestSyncOpenCodeSkillPolicySurvivesSharedFolders(t *testing.T) {
	testutil.TempCwd(t)
	silence(t)
	writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude,opencode,amp]\nsync:\n  shared-skills: true\n")
	writeFile(t, ".agnostic-ai/skills/release/SKILL.md", "---\nname: release\ndescription: Release.\ndisable-model-invocation: true\n---\nBody.\n")
	execCLI(t, "sync")
	execCLI(t, "sync", "--check")
	for _, path := range []string{".claude/skills/release/SKILL.md", ".opencode/skills/release/SKILL.md", ".agents/skills/release/SKILL.md"} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		native, err := spec.ParseMarkdownBytes(spec.KindSkill, data)
		if err != nil {
			t.Fatal(err)
		}
		policy, present := native.Meta["disable-model-invocation"]
		if strings.HasPrefix(path, ".agents/") {
			if present {
				t.Errorf("Amp inherited unsupported policy: %#v", native.Meta)
			}
		} else if !present || policy != true {
			t.Errorf("shared %s lost policy: %#v", path, native.Meta)
		}
	}
}

func TestSyncGlobalOpenCodeSkillKeepsNativeInvocationForms(t *testing.T) {
	home, source := globalAgentTestHome(t)
	mustWriteGlobalTest(t, filepath.Join(source, "skills/release/SKILL.md"), "---\nname: release\ndescription: Release.\ndisable-model-invocation: true\nmetadata:\n  opencode/autoinvoke: \"false\"\n---\nBody.\n")
	mustWriteGlobalTest(t, filepath.Join(source, "skills/release/assets/check.txt"), "asset\n")
	for _, args := range [][]string{{"--only", "opencode"}, {"--only", "opencode", "--check"}} {
		if _, _, err := runGlobalAgentTest(args...); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(filepath.Join(home, ".config/opencode/skills/release/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	native, err := spec.ParseMarkdownBytes(spec.KindSkill, data)
	if err != nil {
		t.Fatal(err)
	}
	if native.Meta["disable-model-invocation"] != true {
		t.Errorf("global policy lost: %#v", native.Meta)
	}
	metadata, _ := native.Meta["metadata"].(map[string]any)
	if metadata["opencode/autoinvoke"] != "false" {
		t.Errorf("global metadata lost: %#v", metadata)
	}
	asset, err := os.ReadFile(filepath.Join(home, ".config/opencode/skills/release/assets/check.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(asset) != "asset\n" {
		t.Errorf("global asset changed: %q", asset)
	}
}

func TestImportOpencodeSkillNativePolicyReplacesTargetOverride(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing string
		imported string
	}{
		{"flag false", "  disable-model-invocation: true\n", "disable-model-invocation: false\n"},
		{"flag removed", "  disable-model-invocation: true\n", ""},
		{"alias changed", "  metadata:\n    opencode/autoinvoke: \"false\"\n    owner: team\n", "metadata:\n  opencode/autoinvoke: \"true\"\n  owner: team\n"},
		{"alias removed", "  metadata:\n    opencode/autoinvoke: \"false\"\n    owner: team\n", "metadata:\n  owner: team\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testutil.TempCwd(t)
			silence(t)
			writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
			writeFile(t, ".agnostic-ai/skills/release/SKILL.md", "---\nname: release\ndescription: Release.\nx-opencode:\n"+tc.existing+"  custom: keep-this\nx-claude:\n  disable-model-invocation: true\n  custom: sibling-only\n---\nBody.\n")
			execCLI(t, "sync")
			native := "---\nname: release\ndescription: Release.\n" + tc.imported + "---\nEdited body.\n"
			writeFile(t, ".opencode/skills/release/SKILL.md", native)
			execCLI(t, "import", "opencode", "--overwrite")
			imported, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(readFile(t, ".agnostic-ai/skills/release/SKILL.md")))
			if err != nil {
				t.Fatal(err)
			}
			target, _ := imported.Meta["x-opencode"].(map[string]any)
			if target["custom"] != "keep-this" {
				t.Errorf("unrelated target metadata lost: %#v", target)
			}
			sibling, _ := imported.Meta["x-claude"].(map[string]any)
			if !reflect.DeepEqual(sibling, map[string]any{"disable-model-invocation": true, "custom": "sibling-only"}) {
				t.Errorf("sibling target changed: %#v", sibling)
			}
			execCLI(t, "lint")
			execCLI(t, "sync")
			execCLI(t, "sync", "--check")
			actual, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(readFile(t, ".opencode/skills/release/SKILL.md")))
			if err != nil {
				t.Fatal(err)
			}
			expected, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(native))
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{"disable-model-invocation", "metadata"} {
				got, gotSet := actual.Meta[key]
				want, wantSet := expected.Meta[key]
				if gotSet != wantSet || !reflect.DeepEqual(got, want) {
					t.Errorf("native %s edit replaced by stale override: got %#v (present %t), want %#v (present %t)", key, got, gotSet, want, wantSet)
				}
			}
			if !strings.HasSuffix(strings.TrimSpace(actual.Body), "Edited body.") {
				t.Errorf("edited body lost: %q", actual.Body)
			}
		})
	}
}

func TestImportOpencodeSkillNativePolicyReconcilesYAMLOverrides(t *testing.T) {
	for _, tc := range []struct {
		name     string
		existing string
	}{
		{"merged override", "x-cursor: &opts\n  disable-model-invocation: true\nx-opencode:\n  <<: *opts\n  custom: keep-this\n"},
		{"shared anchor", "x-opencode: &opts\n  disable-model-invocation: true\n  custom: keep-this\nx-claude: *opts\n"},
	} {
		for _, policy := range []string{"disable-model-invocation: false\n", ""} {
			name := "removed"
			if policy != "" {
				name = "false"
			}
			t.Run(tc.name+"/"+name, func(t *testing.T) {
				testutil.TempCwd(t)
				silence(t)
				writeFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [opencode]\n")
				writeFile(t, ".agnostic-ai/skills/release/SKILL.md", "---\nname: release\ndescription: Release.\n"+tc.existing+"---\nBody.\n")
				execCLI(t, "sync")
				native := "---\nname: release\ndescription: Release.\n" + policy + "---\nEdited body.\n"
				writeFile(t, ".opencode/skills/release/SKILL.md", native)
				execCLI(t, "import", "opencode", "--overwrite")
				imported, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(readFile(t, ".agnostic-ai/skills/release/SKILL.md")))
				if err != nil {
					t.Fatalf("import left invalid source YAML: %v", err)
				}
				siblingTarget := "x-cursor"
				sibling := map[string]any{"disable-model-invocation": true}
				if tc.name == "shared anchor" {
					siblingTarget = "x-claude"
					sibling["custom"] = "keep-this"
				}
				if !reflect.DeepEqual(imported.Meta[siblingTarget], sibling) {
					t.Errorf("sibling semantics changed: got %#v, want %#v", imported.Meta[siblingTarget], sibling)
				}
				target, _ := imported.Meta["x-opencode"].(map[string]any)
				if target["custom"] != "keep-this" {
					t.Errorf("unrelated target metadata lost: %#v", target)
				}
				execCLI(t, "lint")
				execCLI(t, "sync")
				execCLI(t, "sync", "--check")
				actual, err := spec.ParseMarkdownBytes(spec.KindSkill, []byte(readFile(t, ".opencode/skills/release/SKILL.md")))
				if err != nil {
					t.Fatal(err)
				}
				got, present := actual.Meta["disable-model-invocation"]
				if policy == "" {
					if present {
						t.Errorf("removed native policy restored by YAML override: %#v", got)
					}
				} else if !present || got != false {
					t.Errorf("false native policy restored by YAML override: %#v", got)
				}
			})
		}
	}
}
