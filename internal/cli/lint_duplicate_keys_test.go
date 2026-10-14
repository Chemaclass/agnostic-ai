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
	if f.Code != "LINT046" || f.Severity != lintError || f.Path != "skills/a/SKILL.md" {
		t.Errorf("finding = %+v", f)
	}
	for _, want := range []string{"`workspaces`", "`name`"} {
		if !strings.Contains(f.Message, want) {
			t.Errorf("message %q lacks %s", f.Message, want)
		}
	}
}

// The repeat an `import cursor` of 0.83.0 left in a skill is a lint
// error, where it used to read "ok, 1 spec(s) clean" (#2202).
func TestLint_FlagsARepeatedFrontmatterKey(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	silence(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, ".agnostic-ai/skills/web/SKILL.md", "---\nname: web\nworkspaces: [apps/web]\ndescription: Web skill.\nworkspaces: [apps/web]\n---\n\nBody.\n")
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "event: PostToolUse\ncommand: a\ncommand: b\n")
	out, err := runCLI(t, "lint")
	if err == nil {
		t.Fatalf("lint passed:\n%s", out)
	}
	for _, want := range []string{
		"LINT046 [error] " + ".agnostic-ai/skills/web/SKILL.md",
		"LINT046 [error] " + ".agnostic-ai/hooks/fmt.yaml",
		"`workspaces`",
		"`command`",
	} {
		if !strings.Contains(strings.ReplaceAll(out, "\\", "/"), want) {
			t.Errorf("lint output lacks %q:\n%s", want, out)
		}
	}

	mustWriteFile(t, ".agnostic-ai/skills/web/SKILL.md", "---\nname: web\nworkspaces: [apps/web]\ndescription: Web skill.\n---\n\nBody.\n")
	mustWriteFile(t, ".agnostic-ai/hooks/fmt.yaml", "event: PostToolUse\ncommand: a\n")
	if out, err := runCLI(t, "lint"); err != nil {
		t.Errorf("lint after removing the repeats: %v\n%s", err, out)
	}
}
