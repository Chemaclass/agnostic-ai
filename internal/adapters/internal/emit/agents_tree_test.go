package emit

import (
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/spec"
)

func TestRouteScope_SourceLayoutWinsOverGlobs(t *testing.T) {
	t.Parallel()
	r := spec.Entry{
		Scope: "backend",
		Meta:  map[string]any{"globs": "frontend/**"},
	}
	if got := RouteScope(r); got != "backend" {
		t.Errorf("got %q want backend", got)
	}
}

func TestRouteScope_GlobsPrefixWhenNoSourceScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		globs string
		want  string
	}{
		{"docs/api/**", "docs/api"},
		{"src/**/*.go", "src"},
		{"**/*", ""},
		{"*", ""},
		{"", ""},
		{"foo/bar/baz", "foo/bar/baz"},
	}
	for _, c := range cases {
		r := spec.Entry{Meta: map[string]any{"globs": c.globs}}
		if got := RouteScope(r); got != c.want {
			t.Errorf("RouteScope(globs=%q) = %q, want %q", c.globs, got, c.want)
		}
	}
}
