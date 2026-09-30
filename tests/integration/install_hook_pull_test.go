package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestInstallHook_PostMergeRestoresUntrackedOutputsAfterPull(t *testing.T) {
	repository, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(bin, name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = repository
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v %s", err, out)
	}
	config := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(config, []byte("[user]\n name = test\n email = test@example.com\n[commit]\n gpgsign = false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", config)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	source := filepath.Join(t.TempDir(), "source with space")
	if err := os.Mkdir(source, 0o755); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, source)
	run := func(dir, command string, args ...string) string {
		t.Helper()
		cmd := exec.Command(command, args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v in %s: %v %s", command, args, dir, err, out)
		}
		return string(out)
	}
	git := func(dir string, args ...string) string { return run(dir, "git", args...) }
	git(source, "init", "-q")
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\ngitignore:\n  enabled: false\n")
	mustWrite(t, ".agnostic-ai/AGNOSTIC_AI.md", "# Project\n")
	mustWrite(t, ".agnostic-ai/hooks/guard.yaml", "name: guard\nevent: PreToolUse\ncommand: echo guard\n")
	paths := []string{"CLAUDE.md", ".claude/settings.json"}
	for _, name := range []string{"one", "two", "three"} {
		mustWrite(t, ".agnostic-ai/rules/"+name+".md", "---\nname: "+name+"\n---\nRule.\n")
		paths = append(paths, ".claude/rules/"+name+".md")
	}
	for _, name := range []string{"one", "two"} {
		mustWrite(t, ".agnostic-ai/agents/"+name+".md", "---\nname: "+name+"\ndescription: Agent.\n---\nReview.\n")
		mustWrite(t, ".agnostic-ai/skills/"+name+"/SKILL.md", "---\nname: "+name+"\ndescription: Skill.\n---\nRead reference.txt.\n")
		mustWrite(t, ".agnostic-ai/skills/"+name+"/reference.txt", "Reference.\n")
		paths = append(paths, ".claude/agents/"+name+".md", ".claude/skills/"+name+"/SKILL.md", ".claude/skills/"+name+"/reference.txt")
	}
	run(source, binary, "sync", "-q")
	contents := map[string]string{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		contents[path] = string(data)
	}
	add := []string{"add", "--", "agnostic-ai.yaml", ".agnostic-ai/AGNOSTIC_AI.md", ".agnostic-ai/rules", ".agnostic-ai/agents", ".agnostic-ai/skills", ".agnostic-ai/hooks"}
	git(source, append(add, paths...)...)
	git(source, "commit", "-qm", "tracked outputs")
	hooked := filepath.Join(t.TempDir(), "clone with hooks")
	plain := filepath.Join(t.TempDir(), "clone without hooks")
	git(source, "clone", "-q", source, hooked)
	git(source, "clone", "-q", source, plain)
	run(hooked, binary, "install-hook", "--post-checkout")
	mustWrite(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\ngitignore:\n  enabled: true\n")
	run(source, binary, "sync", "--untrack")
	if tracked := git(source, append([]string{"ls-files", "--"}, paths...)...); strings.TrimSpace(tracked) != "" {
		t.Fatalf("migration still tracks outputs: %s", tracked)
	}
	git(source, "add", "--", "agnostic-ai.yaml", ".gitignore")
	git(source, "commit", "-qm", "ignore generated outputs")
	git(plain, "pull", "--ff-only", "-q")
	for _, path := range paths {
		if _, err := os.Stat(filepath.Join(plain, path)); !os.IsNotExist(err) {
			t.Errorf("control pull did not delete %s: %v", path, err)
		}
	}
	git(hooked, "pull", "--ff-only", "-q")
	for path, want := range contents {
		data, err := os.ReadFile(filepath.Join(hooked, path))
		if err != nil || string(data) != want {
			t.Errorf("pull did not restore %s: bytes=%q err=%v", path, data, err)
		}
	}
}
