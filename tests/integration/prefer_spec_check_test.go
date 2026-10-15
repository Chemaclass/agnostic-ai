package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/cli"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Targets that render a shared skill differently: opencode keeps the
// metadata key the others drop, so prefer-spec picks one version.
func seedPreferSpecSharedSkills(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	must(t, os.WriteFile(filepath.Join(dir, "agnostic-ai.yaml"), []byte(`version: 1
targets: [codex, amp, gemini, opencode]
gitignore:
  enabled: false
outputs:
  gemini:
    skills-dir: .agents/skills
  opencode:
    skills-dir: .agents/skills
sync:
  collision-policy: prefer-spec
`), 0o644))
	for _, name := range []string{"alpha", "beta", "gamma"} {
		skill := filepath.Join(dir, ".agnostic-ai", "skills", name, "SKILL.md")
		must(t, os.MkdirAll(filepath.Dir(skill), 0o755))
		must(t, os.WriteFile(skill, []byte("---\nname: "+name+"\ndescription: Skill "+name+" does things.\nlicense: MIT\nmetadata:\n  owner: team\n---\n\nBody "+name+".\n"), 0o644))
	}
	return dir
}

func TestPreferSpec_SharedSkillDivergentRenderings_CheckPassesAfterSync(t *testing.T) {
	dir := seedPreferSpecSharedSkills(t)

	// Repeat to catch a winner that depends on emit order.
	for range 3 {
		runCmd(t, "sync")
		runCmd(t, "sync", "--check")
	}
	// The last target in `targets:` wins the shared path.
	assertContains(t, filepath.Join(dir, ".agents", "skills", "alpha", "SKILL.md"), "owner: team")
}

func TestPreferSpec_SharedSkillHandEdit_StillDrifts(t *testing.T) {
	dir := seedPreferSpecSharedSkills(t)
	runCmd(t, "sync")

	shared := filepath.Join(dir, ".agents", "skills", "beta", "SKILL.md")
	data, err := os.ReadFile(shared)
	must(t, err)
	must(t, os.WriteFile(shared, append(data, []byte("\nHand edit.\n")...), 0o644))

	runCmdExpectErr(t, "sync", "--check")
}

func TestPreferSpec_SharedSkillDivergentRenderings_StatusShowsNoDriftAfterSync(t *testing.T) {
	seedPreferSpecSharedSkills(t)
	runCmd(t, "sync")

	root := cli.NewRootCmd("test")
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"status"})
	if err := root.Execute(); err != nil {
		t.Fatalf("status: %v\n%s", err, out.String())
	}
	if strings.Contains(out.String(), "out of date") {
		t.Errorf("status reports drift after a clean prefer-spec sync:\n%s", out.String())
	}
}
