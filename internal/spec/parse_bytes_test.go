package spec

import "testing"

func TestParseMarkdownBytes_RuleWithFrontmatter(t *testing.T) {
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
	body := []byte("---\nname: ok\nbroken: [unterminated\n---\nbody\n")
	if _, err := ParseMarkdownBytes(KindRule, body); err == nil {
		t.Error("expected error for malformed YAML in frontmatter")
	}
}

func TestParseYAMLBytes_HookFields(t *testing.T) {
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
	e, err := ParseYAMLBytes(KindMCP, []byte(""))
	if err != nil {
		t.Fatal(err)
	}
	if e.Meta == nil {
		t.Error("Meta must never be nil after parse, even for empty input")
	}
}
