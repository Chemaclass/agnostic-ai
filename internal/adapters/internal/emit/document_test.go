package emit

import "testing"

func TestDocument_EmptyMetaReturnsBodyOnly(t *testing.T) {
	t.Parallel()
	got := Document(nil, "hello body\n", "claude")
	if got != "hello body\n" {
		t.Errorf("expected body-only, got %q", got)
	}
}

func TestDocument_EmptyMetaMapReturnsBodyOnly(t *testing.T) {
	t.Parallel()
	got := Document(map[string]any{}, "hello body\n", "claude")
	if got != "hello body\n" {
		t.Errorf("expected body-only, got %q", got)
	}
}

func TestDocument_PrependsFrontmatterWithSeparatingNewline(t *testing.T) {
	t.Parallel()
	got := Document(map[string]any{"name": "r1"}, "body line\n", "claude")
	want := "---\nname: r1\n---\n\nbody line\n"
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}

func TestDocument_ResolvesTargetSpecificMeta(t *testing.T) {
	t.Parallel()
	meta := map[string]any{
		"name":     "r1",
		"x-claude": map[string]any{"allowed-tools": []any{"Read"}},
		"x-cursor": map[string]any{"globs": "src/**"},
	}
	got := Document(meta, "body\n", "claude")
	if !contains(got, "allowed-tools") {
		t.Errorf("expected resolved claude key, got %q", got)
	}
	if contains(got, "globs") {
		t.Errorf("expected cursor key stripped, got %q", got)
	}
}
