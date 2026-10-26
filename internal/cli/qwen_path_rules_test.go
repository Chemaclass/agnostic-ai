package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

func TestQwen_PathRulesUseNativeRulesDirectory(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/style.md"), "---\nname: style\n---\nKeep changes small.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/react.md"), "---\nname: react\ndescription: React conventions\nglobs: [\"src/**/*.tsx\"]\n---\nUse function components.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	root := readFile(t, filepath.Join(dir, "QWEN.md"))
	if !strings.Contains(root, "Keep changes small.") {
		t.Errorf("QWEN.md lost the unscoped rule:\n%s", root)
	}
	if strings.Contains(root, "Use function components.") {
		t.Errorf("QWEN.md inlines the path rule:\n%s", root)
	}
	native := readFile(t, filepath.Join(dir, ".qwen/rules/react.md"))
	if !strings.HasPrefix(native, "---\n") {
		t.Fatalf("Qwen reads frontmatter only at the start of the file:\n%s", native)
	}
	for _, want := range []string{"description: React conventions\n", "paths:\n  - src/**/*.tsx\n", "Use function components.\n"} {
		if !strings.Contains(native, want) {
			t.Errorf(".qwen/rules/react.md missing %q:\n%s", want, native)
		}
	}
	if strings.Contains(native, "globs") {
		t.Errorf("Qwen reads paths, not globs:\n%s", native)
	}
	if _, err := os.Stat(filepath.Join(dir, ".qwen/rules/style.md")); !os.IsNotExist(err) {
		t.Errorf("unscoped rule must stay in QWEN.md only: %v", err)
	}
	for _, args := range [][]string{{"sync", "--check"}, {"lint"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
}

func TestQwen_ImportRejectsRuleFileNamedLikeAnInlinedRule(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/react.md"), "---\nname: react\n---\nInline react.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if err := os.Remove(filepath.Join(dir, ".agnostic-ai/rules/react.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, ".qwen/rules/react.md"), "---\npaths: [\"src/**/*.tsx\"]\n---\n\nUse function components.\n")
	out, err := runCLI(t, "import", "qwen")
	if err == nil || !strings.Contains(err.Error()+out, "conflicts with") {
		t.Fatalf("import must refuse two rules named react; err=%v\n%s", err, out)
	}
}

func TestQwen_ImportNamesSkippedNestedRuleFiles(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\n")
	writeFile(t, filepath.Join(dir, ".qwen/rules/frontend/react.md"), "---\npaths: [\"src/**/*.tsx\"]\n---\n\nUse function components.\n")
	var logged bytes.Buffer
	prev := logOut
	logOut = &logged
	defer func() { logOut = prev }()
	out, err := runCLI(t, "import", "qwen")
	if err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	if !strings.Contains(logged.String(), ".qwen/rules/frontend/react.md") {
		t.Errorf("import must name the skipped nested file:\n%s", logged.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".agnostic-ai/rules/frontend/react.md")); !os.IsNotExist(err) {
		t.Errorf("a subfolder must not become a directory scope: %v", err)
	}
}

func TestQwen_RulesDirOverrideKeepsScopedInstructions(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\noutputs:\n  qwen:\n    rules-dir: custom/rules\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/api.md"), "---\nname: api\nscope: api\n---\nUse transactions.\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai/rules/react.md"), "---\nname: react\nglobs: [\"src/**/*.tsx\"]\n---\nUse function components.\n")
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if got := readFile(t, filepath.Join(dir, "api/QWEN.md")); !strings.Contains(got, "Use transactions.") {
		t.Errorf("api/QWEN.md:\n%s", got)
	}
	if got := readFile(t, filepath.Join(dir, "custom/rules/react.md")); !strings.Contains(got, "paths:\n  - src/**/*.tsx\n") {
		t.Errorf("custom/rules/react.md:\n%s", got)
	}
}

func TestQwen_ImportNativeRulesDirectory(t *testing.T) {
	dir := testutil.TempCwd(t)
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [qwen]\n")
	writeFile(t, filepath.Join(dir, ".qwen/rules/react.md"), "---\ndescription: React conventions\npaths:\n  - \"src/**/*.tsx\"\n---\n\nUse function components.\n")
	writeFile(t, filepath.Join(dir, ".qwen/rules/always.md"), "Always run the tests.\n")
	if out, err := runCLI(t, "import", "qwen"); err != nil {
		t.Fatalf("import: %v\n%s", err, out)
	}
	react, err := spec.ParseMarkdownBytes(spec.KindRule, []byte(readFile(t, filepath.Join(dir, ".agnostic-ai/rules/react.md"))))
	if err != nil {
		t.Fatal(err)
	}
	if got := spec.GlobList(react.Meta["globs"]); len(got) != 1 || got[0] != "src/**/*.tsx" {
		t.Errorf("react globs = %#v, want src/**/*.tsx", react.Meta["globs"])
	}
	if _, native := react.Meta["x-qwen"]; native {
		t.Errorf("paths must become portable globs: %#v", react.Meta)
	}
	always := readFile(t, filepath.Join(dir, ".agnostic-ai/rules/always.md"))
	if strings.Contains(always, "globs") || !strings.Contains(always, "Always run the tests.") {
		t.Errorf("always-on rule imported wrong:\n%s", always)
	}
	for _, args := range [][]string{{"sync"}, {"sync", "--check"}, {"lint"}} {
		if out, err := runCLI(t, args...); err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
	}
	if root := readFile(t, filepath.Join(dir, "QWEN.md")); !strings.Contains(root, "Always run the tests.") || strings.Contains(root, "Use function components.") {
		t.Errorf("QWEN.md after round trip:\n%s", root)
	}
	if native := readFile(t, filepath.Join(dir, ".qwen/rules/react.md")); !strings.Contains(native, "paths:\n  - src/**/*.tsx\n") || !strings.Contains(native, "Use function components.") {
		t.Errorf(".qwen/rules/react.md after round trip:\n%s", native)
	}
}
