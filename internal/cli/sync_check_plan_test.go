package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// --plan returned before --check ran, so `sync --check --plan` passed a
// CI step on drift (#1593).
func TestSync_CheckPlanFailsOnDrift(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review code.\n---\nReview.\n")
	runSyncOK(t)

	for _, args := range [][]string{{"sync", "--check", "--plan"}, {"sync", "--check", "--plan", "--json"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Errorf("%s on a synced project: %v\n%s", strings.Join(args, " "), err, out)
		}
	}

	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review code.\n---\nReview twice.\n")
	for _, args := range [][]string{{"sync", "--check", "--plan"}, {"sync", "--check", "--plan", "--json"}} {
		out, err := runCLI(t, args...)
		if err == nil || !strings.Contains(err.Error(), "drift detected") {
			t.Errorf("%s on drift: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	if _, err := runCLI(t, "sync", "--plan"); err != nil {
		t.Errorf("sync --plan alone failed on drift: %v", err)
	}
}
