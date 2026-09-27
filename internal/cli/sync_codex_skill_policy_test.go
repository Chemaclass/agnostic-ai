package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The shape `import claude` then `import codex` leaves: Claude's
// manual-only flag in the spec and Codex's own openai.yaml bundled.
func setupBundledOpenAIYAMLSkill(t *testing.T, bundled string) string {
	t.Helper()
	return setupBundledOpenAIYAMLProject(t, "codex, amp", "disable-model-invocation: true\n", bundled)
}

func setupBundledOpenAIYAMLProject(t *testing.T, targets, frontmatter, bundled string) string {
	t.Helper()
	dir := setupFixture(t)
	skill := filepath.Join(dir, ".agnostic-ai", "skills", "deploy")
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+targets+"]\n")
	mustWrite(t, filepath.Join(skill, "SKILL.md"), "---\nname: deploy\ndescription: Deploy.\n"+frontmatter+"---\nDeploy.\n")
	mustWrite(t, filepath.Join(skill, "agents", "openai.yaml"), bundled)
	return dir
}

func TestSync_ManualOnlySkillMergesPolicyIntoBundledOpenAIYAML(t *testing.T) {
	cases := []struct {
		name    string
		bundled string
		want    []string
	}{
		{
			name:    "bundled interface keeps its fields",
			bundled: "interface:\n  display_name: Deploy UI\n",
			want:    []string{"display_name: Deploy UI", "allow_implicit_invocation: false"},
		},
		{
			name:    "bundled explicit policy wins",
			bundled: "policy:\n  allow_implicit_invocation: true\n",
			want:    []string{"allow_implicit_invocation: true"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupBundledOpenAIYAMLSkill(t, tc.bundled)
			testutil.Chdir(t, dir)
			silence(t)

			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filepath.Join(dir, ".agents", "skills", "deploy", "agents", "openai.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(string(data), want) {
					t.Errorf("openai.yaml missing %q:\n%s", want, data)
				}
			}
			if strings.Count(string(data), "allow_implicit_invocation") != 1 {
				t.Errorf("openai.yaml policy set more than once:\n%s", data)
			}
			if err := runSync(t, "--check"); err != nil {
				t.Errorf("sync --check after sync: %v", err)
			}
		})
	}
}

// Only the tree Codex scans gets the merged file. Elsewhere the bundled
// file is an opaque asset: copied verbatim, never parsed.
func TestSync_BundledOpenAIYAMLStaysVerbatimOutsideCodexSkillsTree(t *testing.T) {
	const notAMap = "- a\n- b\n"
	const withComment = "# kept\ninterface:\n  display_name: Deploy UI\n"
	cases := []struct {
		name        string
		targets     string
		frontmatter string
		bundled     string
		folder      string
	}{
		{"claude with a list file", "claude", "", notAMap, ""},
		{"claude, manual-only, list file", "claude", "disable-model-invocation: true\n", notAMap, ""},
		{"cursor with a list file", "cursor", "", notAMap, ".cursor/skills/deploy"},
		{"cursor, manual-only", "cursor", "disable-model-invocation: true\n", withComment, ".cursor/skills/deploy"},
		{"codex with a list file", "codex", "", notAMap, ".agents/skills/deploy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := setupBundledOpenAIYAMLProject(t, tc.targets, tc.frontmatter, tc.bundled)
			testutil.Chdir(t, dir)
			silence(t)

			if err := runSync(t); err != nil {
				t.Fatal(err)
			}
			if tc.folder == "" {
				return
			}
			data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(tc.folder), "agents", "openai.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tc.bundled {
				t.Errorf("openai.yaml not verbatim:\ngot:  %q\nwant: %q", data, tc.bundled)
			}
		})
	}
}

func TestSync_ManualOnlySkillNamesAnUnparsableBundledOpenAIYAML(t *testing.T) {
	dir := setupBundledOpenAIYAMLProject(t, "codex", "disable-model-invocation: true\n", "- a\n")
	testutil.Chdir(t, dir)
	silence(t)

	err := runSync(t)
	if err == nil || !strings.Contains(err.Error(), filepath.Join("deploy", "agents", "openai.yaml")) {
		t.Fatalf("want an error naming the bundled file, got %v", err)
	}
}

func TestSync_TargetSharingCodexSkillsDirWritesMergedOpenAIYAML(t *testing.T) {
	dir := setupBundledOpenAIYAMLProject(t, "codex, kiro", "disable-model-invocation: true\n", "interface:\n  display_name: Deploy UI\n")
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, kiro]\noutputs:\n  codex:\n    skills-dir: shared/skills\n  kiro:\n    skills-dir: shared/skills\n")
	testutil.Chdir(t, dir)
	silence(t)

	if err := runSync(t); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "shared", "skills", "deploy", "agents", "openai.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"display_name: Deploy UI", "allow_implicit_invocation: false"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("openai.yaml missing %q:\n%s", want, data)
		}
	}
	if err := runSync(t, "--check"); err != nil {
		t.Errorf("sync --check after sync: %v", err)
	}
}
