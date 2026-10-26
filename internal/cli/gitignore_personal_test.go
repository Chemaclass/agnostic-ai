package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/gitquery"
)

func crlf(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

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
		{"keeps lines after its begin marker when the end is missing", "# >>> agnostic-ai (personal) /p >>>\n/keep\n", "", nil, "# >>> agnostic-ai (personal) /p >>>\n/keep\n"},
		{"keeps lines after a foreign marker", "# >>> agnostic-ai (personal) notes >>>\n/keep\n# <<< agnostic-ai (personal) notes <<<\n", "", nil, "# >>> agnostic-ai (personal) notes >>>\n/keep\n# <<< agnostic-ai (personal) notes <<<\n"},
		{"keeps user lines between a stray begin and its block", "# >>> agnostic-ai (personal) /p >>>\n/keep\n" + block("/p", "/old"), "", nil, "# >>> agnostic-ai (personal) /p >>>\n/keep\n"},
		{"removes a CRLF block", "a\r\n" + crlf(block("/p", "/old")) + "b\r\n", "", nil, "a\r\nb\r\n"},
		{"replaces a CRLF block in the file's style", "a\r\n" + crlf(block("/p", "/old")), "", []string{"/new"}, "a\r\n" + crlf(block("/p", "/new"))},
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

func excludeRepo(t *testing.T) (root, exclude string) {
	t.Helper()
	silence(t)
	root = t.TempDir()
	chdir(t, root)
	git(t, root, "init", "--quiet")
	gitquery.Reset()
	return root, filepath.Join(root, ".git", "info", "exclude")
}

func TestWritePersonalExcludes_WaitsForAnotherWriter(t *testing.T) {
	root, exclude := excludeRepo(t)
	held, err := os.OpenFile(filepath.Join(filepath.Dir(exclude), "agnostic-ai-exclude.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := tryLockProjectFile(held); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(exclude)
	done := make(chan error, 1)
	go func() { done <- writePersonalExcludes(root, []string{"/.claude/x.md"}) }()
	time.Sleep(200 * time.Millisecond)
	if during, _ := os.ReadFile(exclude); string(during) != string(before) {
		t.Errorf("info/exclude changed while another writer held the lock:\n%s", during)
	}
	_ = held.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if after, _ := os.ReadFile(exclude); !strings.Contains(string(after), "/.claude/x.md") {
		t.Errorf("info/exclude after the lock was released = %q", after)
	}
}

func TestPriorIgnoreFiles_RestoresOnlyThisProjectsBlock(t *testing.T) {
	root, exclude := excludeRepo(t)
	if err := writePersonalExcludes(root, []string{"/old.md"}); err != nil {
		t.Fatal(err)
	}
	prior := priorIgnoreFiles(root, &config.Config{})
	if err := writePersonalExcludes(root, []string{"/new.md"}); err != nil {
		t.Fatal(err)
	}
	other := t.TempDir()
	data, _ := os.ReadFile(exclude)
	if err := os.WriteFile(exclude, []byte(withPersonalExcludes(string(data), personalExcludeKey(other), "", []string{"/other.md"})+"/mine.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := restoreFiles(prior); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exclude)
	for _, want := range []string{"/old.md", "/other.md", "/mine.txt"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("restored info/exclude lacks %s:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), "/new.md") {
		t.Errorf("restored info/exclude keeps the failed sync's line:\n%s", got)
	}
}

func holdExcludeLock(t *testing.T, exclude string) *os.File {
	t.Helper()
	held, err := os.OpenFile(filepath.Join(filepath.Dir(exclude), "agnostic-ai-exclude.lock"), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := tryLockProjectFile(held); err != nil {
		t.Fatal(err)
	}
	return held
}

func TestPriorIgnoreFiles_ReadsTheBlockUnderTheLock(t *testing.T) {
	root, exclude := excludeRepo(t)
	if err := writePersonalExcludes(root, []string{"/old.md"}); err != nil {
		t.Fatal(err)
	}
	held := holdExcludeLock(t, exclude)
	done := make(chan []priorFile, 1)
	go func() { done <- priorIgnoreFiles(root, &config.Config{}) }()
	select {
	case <-done:
		t.Fatal("priorIgnoreFiles read info/exclude while another writer held the lock")
	case <-time.After(200 * time.Millisecond):
	}
	_ = held.Close()
	<-done
}

func TestBuiltinChanges_ApplyHoldsTheGitExcludeLock(t *testing.T) {
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
	held := holdExcludeLock(t, filepath.Join(root, ".git", "info", "exclude"))
	done := make(chan error, 1)
	go func() {
		_, err := h.apply(proposal.ID)
		done <- err
	}()
	select {
	case err := <-done:
		t.Fatalf("apply finished while another writer held the info/exclude lock: %v", err)
	case <-time.After(300 * time.Millisecond):
	}
	_ = held.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
