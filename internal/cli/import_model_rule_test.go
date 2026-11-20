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

type modelRuleCase struct {
	kind, source, model string
	imports             [][]string
	edit                string
	relocated           bool
}

// One rule for every importer: a body-only native edit never changes the
// spec's model, a native model sync would not give that tool changes only
// that tool's entry, and a deleted native model stays deleted for that
// tool alone.
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
	specs := map[string]func(source string) string{
		"own entry": func(s string) string {
			if s == "claude" || s == "opencode" {
				return "{default: sonnet, opencode: gpt-5}"
			}
			return "{default: sonnet, opencode: gpt-5, " + s + ": m-src}"
		},
		"scalar":          func(string) string { return "sonnet" },
		"claude-only map": func(string) string { return "{default: sonnet, claude: opus}" },
		"tier":            func(string) string { return "fast" },
	}
	for _, f := range formats {
		for order, imports := range orders {
			for spec, model := range specs {
				for _, edit := range []string{"body", "model", "delete"} {
					c := modelRuleCase{f.kind, f.source, model(f.source), imports(f.source), edit, false}
					t.Run(f.kind+"/"+f.source+"/"+order+"/"+spec+"/"+edit, func(t *testing.T) { checkModelRule(t, c) })
					if order == "alone" {
						c.relocated = true
						t.Run(f.kind+"/"+f.source+"/relocated/"+spec+"/"+edit, func(t *testing.T) { checkModelRule(t, c) })
					}
				}
			}
		}
	}
}

func checkModelRule(t *testing.T, c modelRuleCase) {
	switch {
	case c.source == "claude" && len(c.imports[0]) > 1:
		t.Skip("the same source twice")
	case c.source == "trae" && c.edit == "delete":
		t.Skip("Trae shows no shared model, so a deleted native line keeps the entry")
	case (c.source == "goose" || c.source == "openhands") && len(c.imports[0]) > 1:
		t.Skip("import claude pins agents to claude when .agents/agents exists")
	}
	dir := testutil.TempCwd(t)
	silence(t)
	targets := []string{"claude", "opencode"}
	if !slices.Contains(targets, c.source) {
		targets = append(targets, c.source)
	}
	cfg := "version: 1\ntargets: [" + strings.Join(targets, ", ") + "]\nmodels:\n  fast: {default: sonnet}\n"
	specDir := filepath.Join(dir, ".agnostic-ai", c.kind)
	if c.relocated {
		cfg += "sources:\n  agents: specs/subagents\n  commands: specs/agents/commands\n"
		specDir = filepath.Join(dir, "specs", map[string]string{"agents": "subagents", "commands": "agents/commands"}[c.kind])
	}
	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), cfg)
	spec := filepath.Join(specDir, "check.md")
	writeFile(t, spec, "---\nname: check\ndescription: Checks things.\nmodel: "+c.model+"\n---\n\nBody marker.\n")
	execCLI(t, "sync")
	before := frontmatterOf(t, readFile(t, spec))

	natives := nativeFilesWith(t, dir, "Body marker.")
	othersBefore := map[string]string{}
	edited := false
	for _, path := range natives {
		content := strings.Replace(readFile(t, path), "Body marker.", "Body marker edited.", 1)
		if !ownedBy(dir, path, c.source) {
			othersBefore[path] = nativeModelLine.FindString(content)
		} else if c.edit != "body" {
			if !nativeModelLine.MatchString(content) {
				t.Skipf("%s writes no model here", c.source)
			}
			if c.edit == "model" {
				content = nativeModelLine.ReplaceAllString(content, "${1}m-new")
			} else {
				content = regexp.MustCompile(`(?m)^model\s*[:=].*\n`).ReplaceAllString(content, "")
			}
			edited = true
		}
		writeFile(t, path, content)
	}
	if c.edit != "body" && !edited {
		t.Skipf("%s writes no %s file", c.source, c.kind)
	}
	for _, sources := range c.imports {
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
	for path, line := range othersBefore {
		if got := nativeModelLine.FindString(readFile(t, path)); got != line {
			t.Errorf("%s model changed from %q to %q:\n%s", path, line, got, readFile(t, spec))
		}
	}
	for _, path := range natives {
		if !ownedBy(dir, path, c.source) {
			continue
		}
		got := nativeModelLine.FindString(readFile(t, path))
		if c.edit == "delete" && got != "" || c.edit == "model" && !strings.HasSuffix(got, "m-new") {
			t.Errorf("%s did not keep its %s edit, has %q:\n%s", path, c.edit, got, readFile(t, spec))
		}
	}
	switch c.edit {
	case "body":
		if !reflect.DeepEqual(after["model"], before["model"]) {
			t.Errorf("a body-only edit changed the model: %v -> %v", before["model"], after["model"])
		}
	case "model":
		entries := map[string]any{}
		switch m := before["model"].(type) {
		case map[string]any:
			for k, v := range m {
				entries[k] = v
			}
		case string:
			entries["default"] = m
		}
		for _, target := range importModelTargets(c.source) {
			entries[target] = "m-new"
		}
		if !reflect.DeepEqual(after["model"], map[string]any(entries)) {
			t.Errorf("model: got %v, want %v\n%s", after["model"], entries, readFile(t, spec))
		}
	}
	for key, value := range after {
		if strings.HasPrefix(key, "x-") && c.edit != "delete" && !reflect.DeepEqual(before[key], value) {
			t.Errorf("import changed %s:\n%s", key, readFile(t, spec))
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
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == ".agnostic-ai" || d.Name() == "specs" {
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

var nativeToolDirs = map[string]string{
	"claude": ".claude", "opencode": ".opencode", "junie": ".junie", "trae": ".trae",
	"qoder": ".qoder", "copilot": ".github", "cursor": ".cursor", "goose": ".agents",
	"openhands": ".agents", "codex": ".codex", "factory": ".factory", "qwen": ".qwen",
}

// ownedBy reports whether path sits in the folder source writes.
func ownedBy(dir, path, source string) bool {
	rel, _ := filepath.Rel(dir, path)
	return strings.SplitN(filepath.ToSlash(rel), "/", 2)[0] == nativeToolDirs[source]
}
