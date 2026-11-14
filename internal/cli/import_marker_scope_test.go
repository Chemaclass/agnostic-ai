package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func markerProject(t *testing.T, targets, frontmatter string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+targets+"]\n")
	mustWriteFile(t, ".agnostic-ai/agents/rev.md", "---\nname: rev\ndescription: r\n"+frontmatter+"---\n\nbody\n")
	runSyncOK(t)
}

func TestImport_AnotherToolsDeleteMarkerSurvivesAnImportOfThisOne(t *testing.T) {
	for _, tc := range []struct{ target, file string }{
		{"gemini", ".gemini/agents/rev.md"},
		{"qwen", ".qwen/agents/rev.md"},
		{"junie", ".junie/agents/rev.md"},
		{"grok", ".grok/agents/rev.md"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			markerProject(t, "claude, "+tc.target, "effort: high\nx-claude:\n  effort: null\n")
			native := readFile(t, tc.file)
			if !strings.Contains(native, "description: r\n") {
				t.Fatalf("no description line to edit in %s:\n%s", tc.file, native)
			}
			mustWriteFile(t, tc.file, strings.Replace(native, "description: r\n", "description: r2\n", 1))

			reimportEffortMap(t, tc.target)
			runSyncOK(t)

			if got := readFile(t, ".agnostic-ai/agents/rev.md"); !strings.Contains(got, "effort: null") {
				t.Errorf("claude's marker must stay:\n%s", got)
			}
			if claude := readFile(t, ".claude/agents/rev.md"); strings.Contains(claude, "effort:") {
				t.Errorf("claude must still get no effort:\n%s", claude)
			}
		})
	}
}

func TestImportClaude_ReimportKeepsItsDeleteMarkerWhileTheNativeFileLacksTheKey(t *testing.T) {
	markerProject(t, "claude", "effort:\n  zed: low\n  default: high\nx-claude:\n  effort: null\n")

	got := reimportEffortMap(t, "claude")
	if !strings.Contains(got, "effort: null") {
		t.Errorf("the marker must stay while the native file has no effort:\n%s", got)
	}
	runSyncOK(t)
	if native := readFile(t, ".claude/agents/rev.md"); strings.Contains(native, "effort:") {
		t.Errorf("sync must not write effort back:\n%s", native)
	}
}

func TestImportClaude_ReimportKeepsItsModelDeleteMarkerWhileTheNativeFileLacksIt(t *testing.T) {
	markerProject(t, "claude", "model:\n  zed: low\n  default: m-two\nx-claude:\n  model: null\n")

	if got := reimportEffortMap(t, "claude"); !strings.Contains(got, "model: null") {
		t.Errorf("the marker must stay while the native file has no model:\n%s", got)
	}
	runSyncOK(t)
	if native := readFile(t, ".claude/agents/rev.md"); strings.Contains(native, "model:") {
		t.Errorf("sync must not write model back:\n%s", native)
	}
}

func TestImportClaude_ReimportDropsItsDeleteMarkerWhenTheNativeFileShowsTheKey(t *testing.T) {
	markerProject(t, "claude", "effort:\n  zed: low\n  default: high\nx-claude:\n  effort: null\n")
	addNativeLine(t, ".claude/agents/rev.md", "effort", "medium")

	got := reimportEffortMap(t, "claude")
	if strings.Contains(got, "effort: null") {
		t.Errorf("the marker goes once the native file shows effort:\n%s", got)
	}
	runSyncOK(t)
	if native := readFile(t, ".claude/agents/rev.md"); !strings.Contains(native, "effort: medium") {
		t.Errorf("sync must keep the restored effort:\n%s", native)
	}
}

// Without an unrelated edit, import leaves the spec as written, so the
// marker only reaches the merge once the native file changes.
func TestImportClaude_KeepsItsDeleteMarkerAcrossAnUnrelatedEdit(t *testing.T) {
	for _, targets := range []string{"claude", "claude, qoder"} {
		t.Run(targets, func(t *testing.T) {
			markerProject(t, targets, "effort: high\nx-claude:\n  effort: null\n")
			native := readFile(t, ".claude/agents/rev.md")
			mustWriteFile(t, ".claude/agents/rev.md", strings.Replace(native, "description: r\n", "description: r2\n", 1))

			if got := reimportEffortMap(t, "claude"); !strings.Contains(got, "effort: null") || !strings.Contains(got, "description: r2") {
				t.Errorf("the edit must land and the marker must stay:\n%s", got)
			}
			runSyncOK(t)
			if claude := readFile(t, ".claude/agents/rev.md"); strings.Contains(claude, "effort:") {
				t.Errorf("claude must still get no effort:\n%s", claude)
			}
		})
	}
}
