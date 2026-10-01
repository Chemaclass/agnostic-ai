package cli

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const nanMCPSpec = ".agnostic-ai/mcps/gh.yaml"

func nanMCPProject(t *testing.T) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [amp]\n")
	mustWriteFile(t, nanMCPSpec, "name: gh\ncommand: npx\n")
	runSyncOK(t)
	mustWriteFile(t, nanMCPSpec, "name: gh\ncommand: npx\nx-amp:\n  timeout: .nan\n")
}

// An MCP server value JSON cannot hold, such as a YAML .nan, fails the
// sync and names the spec and the server, instead of being dropped while
// the old server stays (#1561).
func TestSync_FailsOnAnMCPValueJSONCannotHold(t *testing.T) {
	nanMCPProject(t)
	before := snapshotFiles(t, ".amp/settings.json", ".agnostic-ai/.sync-state")

	out, err := runCLI(t, "sync")

	if err == nil {
		t.Fatalf("sync passed:\n%s", out)
	}
	for _, want := range []string{nanMCPSpec, `"gh"`, "timeout"} {
		if !strings.Contains(err.Error()+out, want) {
			t.Errorf("error does not name %s: %v\n%s", want, err, out)
		}
	}
	if after := snapshotFiles(t, ".amp/settings.json", ".agnostic-ai/.sync-state"); !reflect.DeepEqual(after, before) {
		t.Errorf("a failed sync changed the output or the ledger")
	}
}

// sync --json reports a failed target and goes on with the others. The
// failed target wrote nothing, so its earlier outputs are not orphans:
// they and their ledger records stay.
func TestSyncJSON_AFailedTargetKeepsItsEarlierOutputs(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [amp]\nsync:\n  collision-policy: prefer-spec\n")
	mustWriteFile(t, nanMCPSpec, "name: gh\ncommand: npx\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review code.\n---\nReview.\n")
	runSyncOK(t)
	outputs := readStateFile(".").Outputs
	before := snapshotFiles(t, outputs...)
	mustWriteFile(t, nanMCPSpec, "name: gh\ncommand: npx\nx-amp:\n  timeout: .nan\n")

	out, _ := runCLI(t, "sync", "--json")

	if !strings.Contains(out, `"errors"`) || !strings.Contains(out, "timeout") {
		t.Errorf("sync --json did not report the failed target:\n%s", out)
	}
	for path, body := range before {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != body {
			t.Errorf("%s changed or went after a failed target: %v", path, err)
		}
	}
	if got := readStateFile(".").Outputs; !reflect.DeepEqual(got, outputs) {
		t.Errorf("ledger outputs = %v, want %v", got, outputs)
	}
}

// An x-amp value fails amp only: claude never reads it.
func TestSync_AnotherTargetsBadMCPValueDoesNotFailThisTarget(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [amp, claude]\n")
	mustWriteFile(t, nanMCPSpec, "name: gh\ncommand: npx\nx-amp:\n  timeout: .nan\n")

	runSyncOK(t, "-t", "claude")

	if _, err := runCLI(t, "sync", "-t", "amp"); err == nil {
		t.Error("sync -t amp passed with its own .nan")
	}
}

func TestValidate_ReportsAnMCPValueJSONCannotHold(t *testing.T) {
	nanMCPProject(t)
	out, err := runCLI(t, "validate")
	if err == nil || !strings.Contains(out, nanMCPSpec) || !strings.Contains(out, "timeout") {
		t.Errorf("validate: %v\n%s", err, out)
	}
}

func TestLint_ReportsAnMCPValueJSONCannotHold(t *testing.T) {
	nanMCPProject(t)
	out, err := runCLI(t, "lint")
	lines := strings.Join(findingLines(out, "LINT027"), "\n")
	if err == nil || !strings.Contains(lines, nanMCPSpec) || !strings.Contains(lines, "x-amp.timeout") {
		t.Errorf("lint: %v\n%s", err, out)
	}
}

func snapshotFiles(t *testing.T, paths ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		out[p] = string(data)
	}
	return out
}
