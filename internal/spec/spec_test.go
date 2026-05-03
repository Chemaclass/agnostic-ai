package spec

import "testing"

func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	meta, body := splitFrontmatter([]byte("hello world"))
	if len(meta) != 0 {
		t.Fatalf("expected empty meta, got %v", meta)
	}
	if body != "hello world" {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestSplitFrontmatter_WithFrontmatter(t *testing.T) {
	input := []byte("---\nname: foo\ndescription: bar\n---\nbody here\n")
	meta, body := splitFrontmatter(input)
	if meta["name"] != "foo" {
		t.Fatalf("expected name=foo, got %v", meta["name"])
	}
	if body != "body here\n" {
		t.Fatalf("body mismatch: %q", body)
	}
}

func TestFilter(t *testing.T) {
	entries := []Entry{
		{Kind: KindAgent, Name: "a"},
		{Kind: KindRule, Name: "r"},
		{Kind: KindAgent, Name: "b"},
	}
	got := Filter(entries, KindAgent)
	if len(got) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(got))
	}
}
