package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

var nativeModelLine = regexp.MustCompile(`(?m)^(model\s*[:=]\s*"?)[^"\n]+`)

// One rule for every importer: a body-only native edit never changes the
// spec's model, and a native model sync would not give that tool changes
// only that tool's entry.
func TestImport_ModelFollowsOneRuleForEveryFormat(t *testing.T) {
	formats := []struct{ kind, source string }{
		{"commands", "claude"}, {"commands", "opencode"}, {"commands", "junie"},
		{"commands", "trae"}, {"commands", "qoder"},
		{"agents", "claude"}, {"agents", "copilot"}, {"agents", "cursor"},
		{"agents", "goose"}, {"agents", "openhands"}, {"agents", "codex"},
		{"agents", "opencode"}, {"agents", "factory"}, {"agents", "qwen"},
		{"agents", "trae"}, {"agents", "qoder"},
	}
	orders := map[string]func(source string) [][]string{
		"alone":        func(s string) [][]string { return [][]string{{s}} },
		"claude first": func(s string) [][]string { return [][]string{{"claude", s}} },
		"claude last":  func(s string) [][]string { return [][]string{{s, "claude"}} },
	}
	for _, f := range formats {
		for order, imports := range orders {
			for _, edit := range []string{"body", "model"} {
				t.Run(f.kind+"/"+f.source+"/"+order+"/"+edit, func(t *testing.T) {
					checkModelRule(t, f.kind, f.source, imports(f.source), edit)
				})
			}
		}
	}
}

func checkModelRule(t *testing.T, kind, source string, imports [][]string, edit string) {
	dir := testutil.TempCwd(t)
	silence(t)
	targets := []string{"claude", "opencode"}
	if !slices.Contains(targets, source) {
		targets = append(targets, source)
	}
	model := "{default: sonnet, opencode: gpt-5}"
	if source != "claude" && source != "opencode" {
		model = "{default: sonnet, opencode: gpt-5, " + source + ": m-src}"
	}
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: ["+strings.Join(targets, ", ")+"]\n")
	spec := filepath.Join(dir, ".agnostic-ai", kind, "check.md")
	writeFile(t, spec, "---\nname: check\ndescription: Checks things.\nmodel: "+model+"\n---\n\nBody marker.\n")
	execCLI(t, "sync")
	before := frontmatterOf(t, readFile(t, spec))

	natives := nativeFilesWith(t, dir, "Body marker.")
	edited := false
	for _, path := range natives {
		content := readFile(t, path)
		content = strings.Replace(content, "Body marker.", "Body marker edited.", 1)
		if edit == "model" && ownedBy(dir, path, source) {
			if !nativeModelLine.MatchString(content) {
				t.Skipf("%s %s hold no model", source, kind)
			}
			content = nativeModelLine.ReplaceAllString(content, "${1}m-new")
			edited = true
		}
		writeFile(t, path, content)
	}
	if edit == "model" && !edited {
		t.Skipf("%s writes no %s file", source, kind)
	}
	for _, sources := range imports {
		if out, err := runCLI(t, append([]string{"import"}, sources...)...); err != nil {
			t.Fatalf("import %v: %v\n%s", sources, err, out)
		}
	}
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s\n%s", err, out, readFile(t, spec))
	}
	if out, err := runCLI(t, "sync", "--check"); err != nil {
		t.Fatalf("sync --check: %v\n%s", err, out)
	}

	after := frontmatterOf(t, readFile(t, spec))
	want := before["model"]
	if edit == "model" {
		entries := map[string]any{}
		if m, ok := want.(map[string]any); ok {
			for k, v := range m {
				entries[k] = v
			}
		}
		for _, target := range importModelTargets(source) {
			entries[target] = "m-new"
		}
		want = entries
	}
	if !reflect.DeepEqual(after["model"], want) {
		t.Errorf("model: got %v, want %v\n%s", after["model"], want, readFile(t, spec))
	}
	for key := range after {
		if _, had := before[key]; !had && strings.HasPrefix(key, "x-") {
			t.Errorf("import added %s:\n%s", key, readFile(t, spec))
		}
	}
}

func frontmatterOf(t *testing.T, doc string) map[string]any {
	t.Helper()
	_, _, meta := claudeFrontmatter(doc)
	return meta
}

func nativeFilesWith(t *testing.T, dir, marker string) []string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.IsDir() && d.Name() == ".agnostic-ai" {
				return filepath.SkipDir
			}
			return nil
		}
		if data, err := os.ReadFile(path); err == nil && strings.Contains(string(data), marker) {
			out = append(out, path)
		}
		return nil
	})
	return out
}

// ownedBy reports whether path sits in a folder that source writes and
// claude does not.
func ownedBy(dir, path, source string) bool {
	rel, _ := filepath.Rel(dir, path)
	claudeFile := strings.HasPrefix(rel, ".claude"+string(filepath.Separator))
	return source == "claude" == claudeFile
}
