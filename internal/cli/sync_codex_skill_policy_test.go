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
	dir := setupFixture(t)
	skill := filepath.Join(dir, ".agnostic-ai", "skills", "deploy")
	mustWrite(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [codex, amp]\n")
	mustWrite(t, filepath.Join(skill, "SKILL.md"), "---\nname: deploy\ndescription: Deploy.\ndisable-model-invocation: true\n---\nDeploy.\n")
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
