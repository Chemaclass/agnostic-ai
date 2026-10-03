package cli

import (
	"path/filepath"
	"testing"
)

func TestJSONOutput_RecordPathsUseSlashesAndListsAreNeverNull(t *testing.T) {
	out := jsonOutput{
		Writes:  []fileRecord{{Path: filepath.FromSlash(".agents/skills/style/SKILL.md"), Backup: filepath.FromSlash(".claude/x.md.bak")}},
		Skipped: []fileRecord{{Path: filepath.FromSlash(".cursor/rules/a.mdc")}},
	}.forOutput()

	if out.Writes[0].Path != ".agents/skills/style/SKILL.md" || out.Writes[0].Backup != ".claude/x.md.bak" || out.Skipped[0].Path != ".cursor/rules/a.mdc" {
		t.Errorf("paths not slashed: %+v %+v", out.Writes, out.Skipped)
	}
	empty := jsonOutput{}.forOutput()
	if empty.Writes == nil || empty.Skipped == nil || empty.Errors == nil {
		t.Errorf("lists must be empty, not nil: %+v", empty)
	}
}
