package spec

import (
	"os"
	"path/filepath"
	"testing"
)

func loadProjectRules(t *testing.T, root string) map[string]Entry {
	t.Helper()
	b, err := LoadLayered([]Layer{{Name: "project", Root: root, Sources: defaultsForTest().Sources, IncludeRoot: root}})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]Entry{}
	for _, r := range b.Rules {
		out[r.Name] = r
	}
	return out
}

// A rules/ subfolder groups rules. It scopes one only when it names a
// project directory and the frontmatter sets no scope (#1430).
func TestLoadLayered_RuleFolderScopesOnlyWithoutFrontmatterScope(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "rules", "modules", "grouped.md"), "---\nname: grouped\nscope: src/a\n---\nA\n")
	mustWrite(t, filepath.Join(root, "rules", "modules", "plain.md"), "---\nname: plain\n---\nP\n")
	mustWrite(t, filepath.Join(root, "rules", "backend", "auth.md"), "---\nname: auth\n---\nB\n")
	mustWrite(t, filepath.Join(root, "rules", "backend", "moved.md"), "---\nname: moved\nscope: src/b\n---\nM\n")
	mustWrite(t, filepath.Join(root, "rules", "backend", "native.md"), "---\nname: native\nx-claude:\n  scope: src/c\n---\nN\n")

	rules := loadProjectRules(t, root)
	for name, want := range map[string]string{
		"grouped": "src/a",
		"plain":   "",
		"auth":    "backend",
		"moved":   "src/b",
	} {
		r := rules[name]
		if got := r.EffectiveScope(); got != want {
			t.Errorf("%s: EffectiveScope() = %q, want %q", name, got, want)
		}
		if got, err := RuleScope(r); err != nil || got != want {
			t.Errorf("%s: RuleScope() = %q, %v; want %q", name, got, err, want)
		}
	}
	native := rules["native"]
	native.Meta = map[string]any{"scope": "src/c"}
	if got, _ := RuleScope(native); got != "src/c" {
		t.Errorf("native: a target-resolved scope must win over the folder, got %q", got)
	}
	if got := rules["moved"].Folder; got != "backend" {
		t.Errorf("moved: Folder = %q, want backend", got)
	}
	if got := rules["grouped"].Folder; got != "" {
		t.Errorf("grouped: a folder naming no project directory is only a group, Folder = %q", got)
	}
}

// A local override that sits in a scoping folder yields to a scope the
// shared spec sets in its frontmatter.
func TestLoadLayered_LocalFolderYieldsToASharedFrontmatterScope(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	local := filepath.Join(root, ".agnostic-ai", "local")
	if err := os.MkdirAll(filepath.Join(root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(root, "rules", "style.md"), "---\nname: style\nscope: src/a\n---\nShared.\n")
	mustWrite(t, filepath.Join(local, "rules", "backend", "style.md"), "---\nname: style\n---\n")
	src := defaultsForTest().Sources
	b, err := LoadLayered([]Layer{
		{Name: "project", Root: root, Sources: src, IncludeRoot: root},
		{Name: "project-user", Root: local, Sources: src, Extends: true, IncludeRoot: root},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := b.Rules[0].EffectiveScope(); got != "src/a" {
		t.Errorf("EffectiveScope() = %q, want src/a", got)
	}
}
