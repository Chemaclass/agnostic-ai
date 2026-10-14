package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestLintDuplicateKeys_ReportsEachRepeatedKeyOnce(t *testing.T) {
	got := lintDuplicateKeys([]spec.Entry{
		{Path: "skills/a/SKILL.md", DuplicateKeys: []string{"workspaces", "name"}},
		{Path: "skills/b/SKILL.md"},
	})
	if len(got) != 1 {
		t.Fatalf("findings = %v, want one", got)
	}
	f := got[0]
	if f.Code != "LINT046" || f.Severity != lintWarn || f.Path != "skills/a/SKILL.md" {
		t.Errorf("finding = %+v", f)
	}
	for _, want := range []string{"keys `workspaces`, `name` are written more than once", "Remove the extra lines"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("message %q lacks %s", f.Message, want)
		}
	}
}

// The repeat an `import cursor` of 0.83.0 left in a skill is a lint
// warning, where it used to read "ok, 1 spec(s) clean". It fails only
// under --strict, so a project that carries one still passes CI (#2202).
func TestLint_FlagsARepeatedFrontmatterKey(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/skills/web/SKILL.md", "---\nname: web\nworkspaces: [apps/web]\ndescription: Web skill.\nworkspaces: [apps/web]\n---\n\nBody.\n")
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "event: PostToolUse\ncommand: a\ncommand: b\n")
	out, err := runCLI(t, "lint")
	if err != nil {
		t.Fatalf("lint failed on a warning:\n%s", out)
	}
	for _, want := range []string{
		"LINT046 [warn] " + ".agnostic-ai/skills/web/SKILL.md",
		"LINT046 [warn] " + ".agnostic-ai/hooks/fmt.yaml",
		"key `workspaces` is written more than once",
		"key `command` is written more than once",
		"Remove the extra line to fix it",
	} {
		if !strings.Contains(strings.ReplaceAll(out, "\\", "/"), want) {
			t.Errorf("lint output lacks %q:\n%s", want, out)
		}
	}

	if out, err := runCLI(t, "lint", "--strict"); err == nil {
		t.Errorf("lint --strict passed with a repeated key:\n%s", out)
	}

	mustWriteFile(t, ".agnostic-ai/skills/web/SKILL.md", "---\nname: web\nworkspaces: [apps/web]\ndescription: Web skill.\n---\n\nBody.\n")
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "event: PostToolUse\ncommand: a\n")
	if out, err := runCLI(t, "lint"); err != nil || strings.Contains(out, "LINT046") {
		t.Errorf("lint after removing the repeats: %v\n%s", err, out)
	}
}
