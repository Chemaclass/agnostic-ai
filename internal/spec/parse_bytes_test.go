package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMarkdownBytes_RuleWithFrontmatter(t *testing.T) {
	t.Parallel()
	body := []byte("---\nname: my-rule\ndescription: short\nglobs: \"**/*\"\nalwaysApply: true\n---\n\nrule body line one.\n")
	e, err := ParseMarkdownBytes(KindRule, body)
	if err != nil {
		t.Fatal(err)
	}
	if e.Kind != KindRule {
		t.Errorf("kind: want rule, got %s", e.Kind)
	}
	if e.Name != "my-rule" {
		t.Errorf("name: want my-rule, got %q", e.Name)
	}
	if e.Description() != "short" {
		t.Errorf("description: want short, got %q", e.Description())
	}
	if e.Body != "rule body line one.\n" {
		t.Errorf("body: %q", e.Body)
	}
	want := []string{"name", "description", "globs", "alwaysApply"}
	if len(e.MetaKeys) != len(want) {
		t.Fatalf("MetaKeys len: want %d got %d (%v)", len(want), len(e.MetaKeys), e.MetaKeys)
	}
	for i, k := range want {
		if e.MetaKeys[i] != k {
			t.Errorf("MetaKeys[%d]: want %q got %q (full %v)", i, k, e.MetaKeys[i], e.MetaKeys)
		}
	}
}

func TestParseMarkdownBytes_NoFrontmatter(t *testing.T) {
	t.Parallel()
	e, err := ParseMarkdownBytes(KindRule, []byte("just body, no frontmatter\n"))
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "" {
		t.Errorf("expected empty name when frontmatter missing, got %q", e.Name)
	}
	if e.Body != "just body, no frontmatter\n" {
		t.Errorf("body roundtrip failed: %q", e.Body)
	}
}

func TestParseMarkdownBytes_CRLF(t *testing.T) {
	t.Parallel()
	body := []byte("---\r\nname: crlf\r\n---\r\nbody\r\n")
	e, err := ParseMarkdownBytes(KindRule, body)
	if err != nil {
		t.Fatal(err)
	}
	if e.Name != "crlf" {
		t.Errorf("CRLF frontmatter not parsed: %q", e.Name)
	}
}

func TestParseMarkdownBytes_MalformedYAMLErrors(t *testing.T) {
	t.Parallel()
	body := []byte("---\nname: ok\nbroken: [unterminated\n---\nbody\n")
	if _, err := ParseMarkdownBytes(KindRule, body); err == nil {
		t.Error("expected error for malformed YAML in frontmatter")
	}
}

func TestParseYAMLBytes_HookFields(t *testing.T) {
	t.Parallel()
	body := []byte("name: fmt-hook\nevent: PostToolUse\ncommand: \"echo\"\n")
	e, err := ParseYAMLBytes(KindHook, body)
	if err != nil {
		t.Fatal(err)
	}
	if e.Kind != KindHook {
		t.Errorf("kind: want hook, got %s", e.Kind)
	}
	if e.Name != "fmt-hook" {
		t.Errorf("name: want fmt-hook, got %q", e.Name)
	}
	if got := e.Meta["event"]; got != "PostToolUse" {
		t.Errorf("event field lost in parse: %v", got)
	}
}

func TestParseYAMLBytes_EmptyDocument(t *testing.T) {
	t.Parallel()
	e, err := ParseYAMLBytes(KindMCP, []byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if e.Meta == nil {
		t.Error("Meta must never be nil after parse, even for empty input")
	}
}

func TestParseYAMLBytes_NestedDuplicateKeyErrors(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		kind Kind
		data string
		want string
	}{
		"settings permissions": {
			kind: KindSettings,
			data: "permissions:\n  deny:\n    - \"Bash(git push:*)\"\n  allow:\n    - Read\n  deny:\n    - \"Read(./.env)\"\n",
			want: `mapping key "deny" already defined`,
		},
		"mcp env": {
			kind: KindMCP,
			data: "name: srv\ncommand: run\nenv:\n  TOKEN: a\n  TOKEN: b\n",
			want: `mapping key "TOKEN" already defined`,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseYAMLBytes(tc.kind, []byte(tc.data))
			if err == nil {
				t.Fatal("expected an error for a nested duplicate key")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not mention %q", err, tc.want)
			}
		})
	}
}

func TestParseMarkdownBytes_NestedDuplicateKeyErrors(t *testing.T) {
	t.Parallel()
	body := []byte("---\nname: ok\nmetadata:\n  a: 1\n  a: 2\n---\nbody\n")
	_, err := ParseMarkdownBytes(KindRule, body)
	if err == nil || !strings.Contains(err.Error(), `mapping key "a" already defined`) {
		t.Fatalf("want nested duplicate error, got %v", err)
	}
}

func TestParseYAMLData_NestedDuplicateKeyReportsPathAndLine(t *testing.T) {
	t.Parallel()
	_, err := parseYAMLData("settings/base.yaml", []byte("permissions:\n  deny: [a]\n  allow: [b]\n  deny: [c]\n"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "settings/base.yaml:4:") {
		t.Errorf("want path and line 4, got %q", err)
	}
}

func TestParseMarkdown_NestedDuplicateKeyReportsFileLine(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(path, []byte("---\nname: ok\nmetadata:\n  a: 1\n  a: 2\n---\nbody\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := parseMarkdown(path)
	if err == nil || !strings.Contains(err.Error(), "x.md:5:") {
		t.Fatalf("want line 5, got %v", err)
	}
}
