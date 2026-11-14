package cli

import (
	"fmt"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// effortMapTargets are the importers that read a native key back into an
// agent's `effort`, with the native file and key each one writes.
var effortMapTargets = []struct{ target, file, key string }{
	{"claude", ".claude/agents/rev.md", "effort"},
	{"qoder", ".qoder/agents/rev.md", "effort"},
	{"grok", ".grok/agents/rev.md", "effort"},
	{"junie", ".junie/agents/rev.md", "effort"},
	{"codebuddy", ".codebuddy/agents/rev.md", "effort"},
	{"factory", ".factory/droids/rev.md", "reasoningEffort"},
	{"augment", ".augment/agents/rev.md", "thinking"},
}

func effortMapProject(t *testing.T, target, effort string) {
	t.Helper()
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", fmt.Sprintf("version: 1\ntargets: [%s]\n", target))
	mustWriteFile(t, ".agnostic-ai/agents/rev.md", "---\nname: rev\ndescription: r\n"+effort+"---\n\nbody\n")
	runSyncOK(t)
}

func editNativeEffort(t *testing.T, file, key, value string) {
	t.Helper()
	lines := strings.Split(readFile(t, file), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, key+":") {
			if value == "" {
				lines = append(lines[:i], lines[i+1:]...)
			} else {
				lines[i] = key + ": " + value
			}
			break
		}
	}
	mustWriteFile(t, file, strings.Join(lines, "\n"))
}

func reimportEffortMap(t *testing.T, target string) string {
	t.Helper()
	if _, err := runCLI(t, "import", target, "--overwrite"); err != nil {
		t.Fatalf("import: %v", err)
	}
	return readFile(t, ".agnostic-ai/agents/rev.md")
}

func TestImportAgents_ReimportUpdatesOnlyTheTargetsEffortEntry(t *testing.T) {
	for _, tc := range effortMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, fmt.Sprintf("effort:\n  %s: high\n  zed: low\n  default: max\n", tc.target))
			editNativeEffort(t, tc.file, tc.key, "medium")

			got := reimportEffortMap(t, tc.target)
			want := fmt.Sprintf("effort:\n  %s: medium\n  zed: low\n  default: max\n", tc.target)
			if !strings.Contains(got, want) {
				t.Errorf("only %s's entry should change:\n%s", tc.target, got)
			}
			runSyncOK(t)
			if _, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v", err)
			}
		})
	}
}

func TestImportAgents_ReimportKeepsAnUnchangedEffortMapAsItIs(t *testing.T) {
	for _, tc := range effortMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			spec := "effort:\n  zed: low\n  default: high\n"
			effortMapProject(t, tc.target, spec)
			before := readFile(t, ".agnostic-ai/agents/rev.md")

			if got := reimportEffortMap(t, tc.target); got != before {
				t.Errorf("a value the map already gives %s must not rewrite it:\n%s", tc.target, got)
			}
		})
	}
}

func TestImportAgents_ReimportAddsATargetEntryWhenTheValueDiffersFromDefault(t *testing.T) {
	for _, tc := range effortMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, "effort:\n  zed: low\n  default: high\n")
			editNativeEffort(t, tc.file, tc.key, "medium")

			got := reimportEffortMap(t, tc.target)
			want := fmt.Sprintf("effort:\n  zed: low\n  default: high\n  %s: medium\n", tc.target)
			if !strings.Contains(got, want) {
				t.Errorf("a new entry for %s expected:\n%s", tc.target, got)
			}
			runSyncOK(t)
			if _, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v", err)
			}
		})
	}
}

func TestImportAgents_ReimportRemovesOnlyTheTargetsEntryWhenTheNativeKeyIsGone(t *testing.T) {
	for _, tc := range effortMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, fmt.Sprintf("effort:\n  %s: high\n  zed: low\n", tc.target))
			editNativeEffort(t, tc.file, tc.key, "")

			got := reimportEffortMap(t, tc.target)
			if !strings.Contains(got, "effort:\n  zed: low\n") || strings.Contains(got, tc.target+": high") {
				t.Errorf("only %s's entry should go:\n%s", tc.target, got)
			}
			runSyncOK(t)
			if _, err := runCLI(t, "sync", "--check"); err != nil {
				t.Errorf("sync --check: %v", err)
			}
		})
	}
}

func TestImportAgents_ReimportDropsTheEffortMapWhenItsLastEntryGoes(t *testing.T) {
	for _, tc := range effortMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, fmt.Sprintf("effort:\n  %s: high\n", tc.target))
			editNativeEffort(t, tc.file, tc.key, "")

			if got := reimportEffortMap(t, tc.target); strings.Contains(got, "effort") {
				t.Errorf("an empty map should go:\n%s", got)
			}
		})
	}
}

func TestImportAgents_ReimportKeepsAScalarEffortScalar(t *testing.T) {
	for _, tc := range effortMapTargets {
		t.Run(tc.target, func(t *testing.T) {
			effortMapProject(t, tc.target, "effort: high\n")
			editNativeEffort(t, tc.file, tc.key, "medium")

			if got := reimportEffortMap(t, tc.target); !strings.Contains(got, "effort: medium\n") {
				t.Errorf("a scalar effort should stay one:\n%s", got)
			}
		})
	}
}

func TestImportAugment_ReimportKeepsAMapEntryAugmentCannotWrite(t *testing.T) {
	effortMapProject(t, "augment", "effort:\n  augment: 8000\n  zed: low\n")
	mustWriteFile(t, ".augment/agents/rev.md", "---\nname: rev\ndescription: r\n---\n\nbody\n")

	got := reimportEffortMap(t, "augment")
	if !strings.Contains(got, "effort:\n  augment: 8000\n  zed: low\n") {
		t.Errorf("a budget Augment cannot show must survive in the map:\n%s", got)
	}
}
