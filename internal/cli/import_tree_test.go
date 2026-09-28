package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

const importTreeTestConfig = "version: 1\nsources:\n  rules: .agnostic-ai/rules\n  agents: .agnostic-ai/agents\n  skills: .agnostic-ai/skills\n  hooks: .agnostic-ai/hooks\n  mcps: .agnostic-ai/mcps\n  commands: .agnostic-ai/commands\ntargets: [codex, cursor, windsurf, gemini]\n"

// importTreeProject builds a git repository holding the project's own
// config plus copies that are not the project's: a gitignored clone, a
// gitignored agent worktree without its .git, and a nested repository
// that is not ignored.
func importTreeProject(t *testing.T) string {
	t.Helper()
	dir, _ := gitRepo(t)
	testutil.Chdir(t, dir)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), importTreeTestConfig)
	writeFile(t, filepath.Join(dir, ".gitignore"), "/other-repo/\n/.claude/worktrees/\n")
	skill := func(rel string) {
		name := filepath.Base(rel)
		writeFile(t, filepath.Join(dir, rel, "SKILL.md"), "---\nname: "+name+"\ndescription: Test.\n---\n\nBody.\n")
	}
	skill(".cursor/skills/real")
	skill("services/api/.cursor/skills/scoped")
	writeFile(t, filepath.Join(dir, "services/api/AGENTS.md"), "# API\n\nUse integer minor units.\n")

	writeFile(t, filepath.Join(dir, "other-repo/AGENTS.md"), "# Other repo\n\nRules for another repository.\n")
	skill(".claude/worktrees/w1/.cursor/skills/leaked")
	writeFile(t, filepath.Join(dir, ".claude/worktrees/w1/services/api/AGENTS.md"), "# Worktree\n\nLeaked copy.\n")

	writeFile(t, filepath.Join(dir, "clone/.git"), "gitdir: /elsewhere\n")
	writeFile(t, filepath.Join(dir, "clone/AGENTS.md"), "# Clone\n\nNested repository.\n")
	skill("clone/.cursor/skills/cloned")
	writeFile(t, filepath.Join(dir, "clone/.devin/rules/cloned.md"), "Nested repository rule.\n")
	writeFile(t, filepath.Join(dir, "clone/GEMINI.md"), "# Clone\n\nNested repository.\n")
	writeFile(t, filepath.Join(dir, "other-repo/GEMINI.md"), "# Other\n\nIgnored.\n")
	writeFile(t, filepath.Join(dir, "other-repo/.devin/rules/other.md"), "Ignored rule.\n")
	return dir
}

// specFilesUnder lists every file below dir, slash-form and relative to it.
func specFilesUnder(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func assertNoForeignImport(t *testing.T, paths []string, contents map[string]string) {
	t.Helper()
	for _, p := range paths {
		for _, leak := range []string{"leaked", "cloned", ".claude/worktrees", "other"} {
			if strings.Contains(p, leak) {
				t.Errorf("imported %s from a directory that is not the project's", p)
			}
		}
	}
	for p, body := range contents {
		for _, leak := range []string{"other-repo", "Rules for another repository", "Nested repository", "Leaked copy", "Ignored"} {
			if strings.Contains(body, leak) {
				t.Errorf("%s holds %q from a directory that is not the project's", p, leak)
			}
		}
	}
}

func TestImport_SkipsGitignoredDirectoriesAndNestedRepositories(t *testing.T) {
	dir := importTreeProject(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "codex", "cursor", "windsurf", "gemini"})
	_ = captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("import: %v", err)
		}
	})

	src := filepath.Join(dir, ".agnostic-ai")
	paths := specFilesUnder(t, src)
	contents := map[string]string{}
	for _, p := range paths {
		data, err := os.ReadFile(filepath.Join(src, p))
		if err != nil {
			t.Fatal(err)
		}
		contents[p] = string(data)
	}
	assertNoForeignImport(t, paths, contents)
	for _, want := range []string{"skills/real/SKILL.md", "skills/services/api/scoped/SKILL.md"} {
		if _, ok := contents[want]; !ok {
			t.Errorf("missing %s, got %v", want, paths)
		}
	}
	found := false
	for _, body := range contents {
		if strings.Contains(body, "Use integer minor units.") {
			found = true
		}
	}
	if !found {
		t.Errorf("services/api/AGENTS.md was not imported, got %v", paths)
	}
}

func TestImport_DryRunListsNothingFromGitignoredDirectoriesOrNestedRepositories(t *testing.T) {
	importTreeProject(t)

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "codex", "cursor", "windsurf", "gemini", "--dry-run"})
	stdout := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("import --dry-run: %v", err)
		}
	})

	var paths []string
	for line := range strings.SplitSeq(stdout, "\n") {
		if p, ok := strings.CutPrefix(strings.TrimSpace(line), "would write "); ok {
			paths = append(paths, filepath.ToSlash(p))
		}
	}
	assertNoForeignImport(t, paths, nil)
	if !strings.Contains(stdout, ".agnostic-ai/skills/real/SKILL.md") {
		t.Errorf("dry-run lost the project's own skill:\n%s", stdout)
	}
}

// A project that ignores its generated output, as the managed .gitignore
// block does, still imports it when asked: at the root and in a scope.
func TestImport_ReadsGitignoredToolFoldersAtTheRootAndInScopes(t *testing.T) {
	dir, _ := gitRepo(t)
	testutil.Chdir(t, dir)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), importTreeTestConfig)
	writeFile(t, filepath.Join(dir, ".gitignore"),
		"/.cursor/\n/services/api/AGENTS.md\n/services/api/.cursor/skills/scoped/\n/services/api/.devin/rules/\n")
	skill := "---\nname: %s\ndescription: Test.\n---\n\nBody.\n"
	writeFile(t, filepath.Join(dir, ".cursor/skills/real/SKILL.md"), strings.ReplaceAll(skill, "%s", "real"))
	writeFile(t, filepath.Join(dir, "services/api/.cursor/skills/scoped/SKILL.md"), strings.ReplaceAll(skill, "%s", "scoped"))
	writeFile(t, filepath.Join(dir, "services/api/AGENTS.md"), "# API\n\nUse integer minor units.\n")
	writeFile(t, filepath.Join(dir, "services/api/.devin/rules/money.md"), "Round half to even.\n")
	writeFile(t, filepath.Join(dir, "services/api/main.go"), "package main\n")

	root := NewRootCmd("test")
	root.SetArgs([]string{"import", "cursor", "codex", "windsurf"})
	_ = captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("import: %v", err)
		}
	})
	src := filepath.Join(dir, ".agnostic-ai")
	for _, want := range []string{"skills/real/SKILL.md", "skills/services/api/scoped/SKILL.md", "rules/services/api/money.md"} {
		if _, err := os.Stat(filepath.Join(src, want)); err != nil {
			t.Errorf("gitignored generated output not imported: %v", err)
		}
	}
	found := false
	for _, p := range specFilesUnder(t, filepath.Join(src, "rules")) {
		data, _ := os.ReadFile(filepath.Join(src, "rules", p))
		found = found || strings.Contains(string(data), "Use integer minor units.")
	}
	if !found {
		t.Error("gitignored services/api/AGENTS.md not imported")
	}
}
