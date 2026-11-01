package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestSync_SkipsFoldersAGlobDirectoryEntryOwns(t *testing.T) {
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	dir := testutil.TempCwd(t)
	mustWriteFile(t, filepath.Join(dir, "agnostic-ai.yaml"),
		"version: 1\ntargets: [claude, cursor]\ngitignore:\n  enabled: true\nsync:\n  unmanaged:\n    - .claude/skills/legacy-*/\n")
	for _, name := range []string{"legacy-a", "pr"} {
		mustWriteFile(t, filepath.Join(dir, ".agnostic-ai", "skills", name, "SKILL.md"),
			"---\nname: "+name+"\ndescription: Skill "+name+".\n---\n\nBody.\n")
	}
	silence(t)
	buf := captureLog(t)

	if err := runSyncOnce(".", nil, false, false, "on", 1); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "skip (unmanaged) .claude/skills/legacy-a/SKILL.md") {
		t.Errorf("no skip line for the owned folder in:\n%s", buf.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "skills", "legacy-a")); !os.IsNotExist(err) {
		t.Errorf(".claude/skills/legacy-a written despite the glob entry (err=%v)", err)
	}
	for _, p := range []string{".claude/skills/pr/SKILL.md", ".cursor/skills/legacy-a/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("%s missing: %v", p, err)
		}
	}
	if strings.Contains(readFile(t, ".gitignore"), ".claude/skills/legacy-a") {
		t.Errorf("gitignore lists the owned folder:\n%s", readFile(t, ".gitignore"))
	}
	cmd := NewRootCmd("test")
	cmd.SetArgs([]string{"sync", "--check"})
	if err := cmd.Execute(); err != nil {
		t.Errorf("sync --check after sync: %v", err)
	}
}
