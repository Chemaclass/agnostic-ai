package emit

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestReportGlobalSkillSyntax_KiloTrustDependsOnDestinationAndSyntax(t *testing.T) {
	home := t.TempDir()
	for _, tc := range []struct {
		name, dir, body string
		unsupported     bool
	}{
		{"native", filepath.Join(home, ".kilo/skills"), "Live: !`printf context`", false},
		{"shared", filepath.Join(home, ".agents/skills"), "Live: !`printf context`", false},
		{"compatibility", filepath.Join(home, ".claude/skills"), "Live: !`printf context`", false},
		{"override", filepath.Join(home, "custom/skills"), "Live: !`printf context`", true},
		{"project", ".kilo/skills", "Live: !`printf context`", true},
		{"nested", filepath.Join(home, ".kilo/skills/nested"), "Live: !`printf context`", true},
		{"shell block", filepath.Join(home, ".kilo/skills"), "```!\nprintf context\n```", true},
		{"tilde shell block", filepath.Join(home, ".kilo/skills"), "~~~!\nprintf context\n~~~", true},
		{"arguments", filepath.Join(home, ".kilo/skills"), "$ARGUMENTS", true},
		{"positional", filepath.Join(home, ".kilo/skills"), "$1", true},
		{"code example", filepath.Join(home, ".kilo/skills"), "```sh\n!`printf context`\n```", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entries := []spec.Entry{{Kind: spec.KindSkill, Name: "probe", Body: tc.body}}
			for _, mode := range []string{OnUnsupportedWarn, OnUnsupportedError} {
				buf := swapWarnerForNotes(t)
				err := ReportGlobalSkillSyntax("kilo", entries, mode, home, tc.dir)
				if (err != nil) != (tc.unsupported && mode == OnUnsupportedError) {
					t.Errorf("%s error = %v", mode, err)
				}
				FlushCoverageNotes()
				if mode == OnUnsupportedWarn && strings.Contains(buf.String(), "has no effect") != tc.unsupported {
					t.Errorf("warning = %s, unsupported = %v", buf, tc.unsupported)
				}
			}
		})
	}
}
