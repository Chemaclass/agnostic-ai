package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/gitquery"
)

func TestWithPersonalExcludes(t *testing.T) {
	gone := filepath.ToSlash(filepath.Join(t.TempDir(), "gone"))
	kept := filepath.ToSlash(t.TempDir())
	block := func(key string, lines ...string) string {
		return personalExcludeStart + key + " >>>\n" + personalExcludeNote + "\n" + strings.Join(lines, "\n") + "\n" + personalExcludeEnd + key + " <<<\n"
	}
	cases := []struct {
		name, content, prefix string
		patterns              []string
		want                  string
	}{
		{"adds a block after the user's lines", "# mine\n/notes.txt", "", []string{"/.claude/skills/handoff/SKILL.md"}, "# mine\n/notes.txt\n" + block("/p", "/.claude/skills/handoff/SKILL.md")},
		{"anchors patterns at the project inside the work tree", "", "apps/web/", []string{"/.claude/x.md"}, block("/p", "/apps/web/.claude/x.md")},
		{"replaces its block in place", "a\n" + block("/p", "/old") + "b\n", "", []string{"/new"}, "a\n" + block("/p", "/new") + "b\n"},
		{"removes its block and keeps user lines", "a\n" + block("/p", "/old") + "b\n", "", nil, "a\nb\n"},
		{"keeps another project's block", block(kept, "/k"), "", nil, block(kept, "/k")},
		{"drops the block of a project that is gone", "a\n" + block(gone, "/g"), "", nil, "a\n"},
		{"leaves a file without blocks alone", "a\n", "", nil, "a\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := withPersonalExcludes(c.content, "/p", c.prefix, c.patterns); got != c.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

func personalProject(t *testing.T, team string) {
	t.Helper()
	silence(t)
	root := t.TempDir()
	chdir(t, root)
	t.Setenv("HOME", t.TempDir())
	t.Setenv("AGNOSTIC_AI_HOME", t.TempDir())
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\nbuiltins: "+team+"\ngitignore:\n  enabled: true\n")
	mustWriteFile(t, ".agnostic-ai/AGNOSTIC_AI.md", "Project instructions.\n")
}

func splitWithLocal(t *testing.T, team, local string, recorded []string) (block, personal []string) {
	t.Helper()
	personalProject(t, team)
	mustWriteFile(t, "agnostic-ai.local.yaml", "builtins: "+local+"\n")
	cfg, b, err := loadProject(".")
	if err != nil {
		t.Fatal(err)
	}
	block, personal, err = personalSplitBlock(".", cfg, b, cfg.Targets, recorded)
	if err != nil {
		t.Fatal(err)
	}
	return block, personal
}

func TestSplitManagedBlock_KeepsOutputsThePersonalConfigTurnsOff(t *testing.T) {
	block, personal := splitWithLocal(t, "[handoff]", "[]", nil)
	if !slices.Contains(block, "/.claude/skills/") {
		t.Errorf("block without the team's skills: %v", block)
	}
	if len(personal) != 0 {
		t.Errorf("personal = %v, want none", personal)
	}
}

func TestSplitManagedBlock_ListsOutputsOnlyThePersonalConfigAdds(t *testing.T) {
	block, personal := splitWithLocal(t, "[]", "[handoff]", []string{".claude/skills/handoff/SKILL.md"})
	if blockCovers(block, "/.claude/skills/handoff/SKILL.md") {
		t.Errorf("block lists a personal output: %v", block)
	}
	if !slices.Contains(personal, "/.claude/skills/handoff/SKILL.md") {
		t.Errorf("personal = %v, want the handoff skill", personal)
	}
}

func TestBuiltinChanges_PersonalToggleKeepsCommittedIgnores(t *testing.T) {
	personalProject(t, "[]")
	root, _ := os.Getwd()
	git(t, root, "init", "--quiet")
	gitquery.Reset()
	mustWriteFile(t, "agnostic-ai.local.yaml", "builtins: []\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	h := newBuiltinChangeHandler(".", false)
	proposal := builtinNamedProposal(t, h, "caveman", true)
	if proposal.Config != "agnostic-ai.local.yaml" {
		t.Fatalf("config = %s", proposal.Config)
	}
	var excluded bool
	for _, change := range proposal.Changes {
		switch change.Path {
		case ".gitignore", worktreeIncludeFile:
			t.Errorf("preview changes %s:\n%s", change.Path, change.Diff)
		case ".git/info/exclude":
			excluded = strings.Contains(change.Diff, "+/.claude/skills/caveman/SKILL.md")
		}
	}
	if !excluded {
		t.Fatalf("preview does not exclude the personal skill: %+v", proposal.Changes)
	}
	builtinAction(t, h, builtinApplyPath, proposal.ID)
	exclude, _ := os.ReadFile(filepath.Join(".git", "info", "exclude"))
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if again, _ := os.ReadFile(filepath.Join(".git", "info", "exclude")); string(again) != string(exclude) {
		t.Errorf("sync changed the planned info/exclude:\n%s\nwant:\n%s", again, exclude)
	}
}
