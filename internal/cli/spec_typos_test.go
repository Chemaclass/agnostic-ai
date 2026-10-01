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

// An agent may name a user or plugin skill sync cannot see, so a likely
// typo warns in sync and fails validate.
func TestSync_MistypedAgentSkillWarnsAndValidateFails(t *testing.T) {
	syncedTyposProject(t)
	mustWriteFile(t, ".agnostic-ai/agents/reviewer.md", "---\nname: reviewer\ndescription: Review code.\nskills: [pr-swep]\n---\nReview.\n")
	log := captureLog(t)

	runSyncOK(t)

	if want := `unknown skill "pr-swep" (did you mean pr-sweep?)`; !strings.Contains(log.String(), want) {
		t.Errorf("sync does not warn %q:\n%s", want, log.String())
	}
	if out, err := runCLI(t, "validate"); err == nil || !strings.Contains(out, "pr-swep") {
		t.Errorf("validate: %v\n%s", err, out)
	}
}

// A skill an agent names may live outside the project, such as a user
// or plugin skill, so only a likely typo of a project skill fails.
func TestSync_AgentSkillFromOutsideTheProjectPasses(t *testing.T) {
	syncedTyposProject(t)
	mustWriteFile(t, ".agnostic-ai/agents/reviewer.md", "---\nname: reviewer\ndescription: Review code.\nskills: [pr-sweep, deploy-to-prod, tools:pr-swep]\n---\nReview.\n")

	runSyncOK(t)
}

// Hooks carry each tool's own event names, so another tool's event is
// not a typo.
func TestSync_AnotherToolsHookEventPasses(t *testing.T) {
	syncedTyposProject(t)
	mustWriteFile(t, ".agnostic-ai/hooks/notify.yaml", "name: notify\nevent: beforeShellExecution\ncommand: \"echo hi\"\n")

	runSyncOK(t)
}

// OpenHands names events in snake_case, so session_start is known.
func TestSync_SnakeCaseHookEventPasses(t *testing.T) {
	syncedTyposProject(t)
	mustWriteFile(t, ".agnostic-ai/hooks/notify.yaml", "name: notify\nevent: session_start\ncommand: \"echo hi\"\n")

	runSyncOK(t)
}
