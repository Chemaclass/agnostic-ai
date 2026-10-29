package emit

import (
	"path/filepath"
	"testing"
)

// A caller may name the folder with slashes or a trailing separator while
// an adapter asks with filepath.Join, which uses backslashes on Windows.
func TestSkillsDirWrittenByOthers_MatchesAnySpellingOfTheFolder(t *testing.T) {
	sess := NewSession()
	sess.SetSkillsDirWriters(map[string][]string{".agents/skills/": {"codex", "crush"}})

	if !sess.SkillsDirWrittenByOthers(filepath.Join(".agents", "skills"), "crush") {
		t.Errorf("codex also writes .agents/skills, want true")
	}
	if sess.SkillsDirWrittenByOthers(".claude/skills", "crush") {
		t.Errorf("no target writes .claude/skills, want false")
	}
}
