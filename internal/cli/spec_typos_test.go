package cli

import (
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func syncedTyposProject(t *testing.T) []string {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWriteFile(t, ".agnostic-ai/skills/pr-sweep/SKILL.md", "---\nname: pr-sweep\ndescription: Sweep PRs.\n---\nSweep.\n")
	mustWriteFile(t, ".agnostic-ai/agents/reviewer.md", "---\nname: reviewer\ndescription: Review code.\n---\nReview.\n")
	mustWriteFile(t, ".agnostic-ai/hooks/notify.yaml", "name: notify\nevent: PreToolUse\ncommand: \"echo hi\"\n")
	runSyncOK(t)
	return readStateFile(".").Outputs
}

func assertTypoRejected(t *testing.T, outputs []string, want string) {
	t.Helper()
	before := snapshotFiles(t, outputs...)
	for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"sync", "--json"}, {"validate"}} {
		out, err := runCLI(t, args...)
		if err == nil {
			t.Errorf("%s passed:\n%s", strings.Join(args, " "), out)
			continue
		}
		if !strings.Contains(err.Error()+out, want) {
			t.Errorf("%s does not say %q: %v\n%s", strings.Join(args, " "), want, err, out)
		}
	}
	if after := snapshotFiles(t, outputs...); !reflect.DeepEqual(after, before) {
		t.Error("a spec typo changed the outputs")
	}
}

// A mistyped hook event shipped to every tool with a green sync --check;
// only validate caught it (#1591).
func TestSync_MistypedHookEventFailsBeforeAnyWrite(t *testing.T) {
	outputs := syncedTyposProject(t)
	mustWriteFile(t, ".agnostic-ai/hooks/notify.yaml", "name: notify\nevent: PreToolUze\ncommand: \"echo hi\"\n")

	assertTypoRejected(t, outputs, `hook event "PreToolUze" (did you mean PreToolUse?)`)
}

func TestSync_MistypedAgentSkillFailsBeforeAnyWrite(t *testing.T) {
	outputs := syncedTyposProject(t)
	mustWriteFile(t, ".agnostic-ai/agents/reviewer.md", "---\nname: reviewer\ndescription: Review code.\nskills: [pr-swep]\n---\nReview.\n")

	assertTypoRejected(t, outputs, `skill "pr-swep" (did you mean pr-sweep?)`)
}

// A skill an agent names may live outside the project, such as a user
// or plugin skill, so only a likely typo of a project skill fails.
func TestSync_AgentSkillFromOutsideTheProjectPasses(t *testing.T) {
	syncedTyposProject(t)
	mustWriteFile(t, ".agnostic-ai/agents/reviewer.md", "---\nname: reviewer\ndescription: Review code.\nskills: [pr-sweep, deploy-to-prod]\n---\nReview.\n")

	runSyncOK(t)
}

// Hooks carry each tool's own event names, so another tool's event is
// not a typo.
func TestSync_AnotherToolsHookEventPasses(t *testing.T) {
	syncedTyposProject(t)
	mustWriteFile(t, ".agnostic-ai/hooks/notify.yaml", "name: notify\nevent: beforeShellExecution\ncommand: \"echo hi\"\n")

	runSyncOK(t)
}
