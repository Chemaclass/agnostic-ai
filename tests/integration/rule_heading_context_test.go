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

func TestRuleHeadingContext_SyncNestsRuleSections(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	name := "agnostic-ai"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	build := exec.Command("go", "build", "-o", binary, "./cmd/agnostic-ai")
	build.Dir = filepath.Join(packageDir, "..", "..")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}
	dir := t.TempDir()
	for name, body := range map[string]string{
		"agnostic-ai.yaml":                    "version: 1\ntargets: [codex, gemini]\n",
		".agnostic-ai/AGNOSTIC_AI.md":         "# Project\n\nRoot guidance.\n",
		".agnostic-ai/rules/content.md":       "---\nname: content\n---\n### Doc versioning\n\nVersioning text.\n\n#### Version details\n\n~~~markdown\n### Code heading\n~~~\n\n1. Run tests\n---\n\n<div>\nLiteral HTML\n---\n# Literal heading\n</div>\n",
		".agnostic-ai/rules/working-style.md": "---\nname: working-style\n---\nStyle text.\n",
	} {
		must(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, name)), 0755))
		must(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0644))
	}
	for _, args := range [][]string{{"sync", "--gitignore=off"}, {"sync", "--check", "--gitignore=off"}} {
		cmd := exec.Command(binary, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	for _, path := range []string{"AGENTS.md", "GEMINI.md"} {
		assertContains(t, filepath.Join(dir, path), "### content\n", "#### Doc versioning\n", "##### Version details\n", "~~~markdown\n### Code heading\n~~~", "### working-style\n", "1. Run tests\n---\n", "<div>\nLiteral HTML\n---\n# Literal heading\n</div>\n")
		assertNoFileContains(t, filepath.Join(dir, path), "\n### Doc versioning\n")
	}
	testutil.AssertGoldenTree(t, dir, filepath.Join(packageDir, "fixtures", "rule-heading-context"), "agnostic-ai.yaml")
	path := filepath.Join(dir, ".agnostic-ai/rules/content.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	must(t, os.WriteFile(path, []byte(strings.Replace(string(data), "# Literal heading", "### Literal heading", 1)), 0644))
	cmd := exec.Command(binary, "sync", "--gitignore=off")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("HTML H3 sync: %v\n%s", err, out)
	}
	for _, fence := range []string{"~~~", "```"} {
		if fence == "```" {
			path := filepath.Join(dir, ".agnostic-ai/rules/content.md")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			must(t, os.WriteFile(path, []byte(strings.ReplaceAll(string(data), "~~~", "```")), 0644))
			cmd := exec.Command(binary, "sync", "--gitignore=off")
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("backtick sync: %v\n%s", err, out)
			}
		}
		before := map[string]string{}
		for _, name := range []string{"AGENTS.md", "GEMINI.md"} {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			before[name] = string(data)
		}
		for _, args := range [][]string{{"import", "codex", "gemini"}, {"sync", "--gitignore=off"}, {"sync", "--check", "--gitignore=off"}} {
			cmd := exec.Command(binary, args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s roundtrip %v: %v\n%s", fence, args, err, out)
			}
		}
		for name, original := range before {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != original {
				t.Errorf("%s %s fence roundtrip changed output:\n%s", name, fence, data)
			}
		}
		rules, err := os.ReadDir(filepath.Join(dir, ".agnostic-ai/rules"))
		if err != nil {
			t.Fatal(err)
		}
		if len(rules) != 2 {
			t.Errorf("%s fence imported phantom rules: %v", fence, rules)
		}
	}

}
