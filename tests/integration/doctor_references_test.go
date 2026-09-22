package integration

import (
	"os"
	"sort"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// Every adapter renders cleanly under the reference check: removing the
// skills from the bundle to find skill-owned documents must not fail any
// target, rules stay out of scope, and code or external links never count
// as local references.
func TestDoctorCheckReferences_CleanForEveryTarget(t *testing.T) {
	targets := adapters.Names()
	sort.Strings(targets)
	for _, target := range targets {
		t.Run(target, func(t *testing.T) {
			testutil.Chdir(t, t.TempDir())
			mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: ["+target+"]\n")
			mustWrite(t, ".agnostic-ai/AGNOSTIC_AI.md", "# A\n")
			mustWrite(t, ".agnostic-ai/rules/style.md", "---\nname: style\n---\nSee [x](missing-from-rule.md).\n")
			mustWrite(t, ".agnostic-ai/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: deploy\n---\n"+
				"See [docs](https://example.com/setup.md) and `[x](inline.md)`.\n\n```\n[y](fenced.md)\n```\n")
			runCmd(t, "sync")
			runCmd(t, "doctor", "--check-references")
		})
	}
}

// The acceptance scenario of #1037: a reference present in the source
// bundle but absent from the emitted one fails the check, and restoring
// the emitted file clears it.
func TestDoctorCheckReferences_BrokenEmittedReferenceFailsUntilRestored(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWrite(t, ".agnostic-ai/AGNOSTIC_AI.md", "# A\n")
	mustWrite(t, ".agnostic-ai/skills/deploy/SKILL.md", "---\nname: deploy\ndescription: deploy\n---\nRead [setup](references/setup.md).\n")
	mustWrite(t, ".agnostic-ai/skills/deploy/references/setup.md", "setup\n")
	runCmd(t, "sync")
	runCmd(t, "doctor", "--check-references")

	const emitted = ".claude/skills/deploy/references/setup.md"
	body := readString(t, emitted)
	must(t, os.Remove(emitted))
	runCmdExpectErr(t, "doctor", "--check-references")

	mustWrite(t, emitted, body)
	runCmd(t, "doctor", "--check-references")
}
