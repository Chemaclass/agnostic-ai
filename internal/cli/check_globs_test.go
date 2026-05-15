package cli

import (
	"testing"
)

func TestCompileGlob_MatchesCommonShapes(t *testing.T) {
	cases := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"*.go", "main.go", true},
		{"*.go", "sub/main.go", false},
		{"**/*.go", "main.go", true},
		{"**/*.go", "a/b/main.go", true},
		{"src/**", "src/a/b/c.ts", true},
		{"src/**", "other/file.ts", false},
		{"backend/**/*.php", "backend/modules/x/Domain/y.php", true},
		{"backend/**/*.php", "frontend/x.php", false},
		{"a/*/b", "a/x/b", true},
		{"a/*/b", "a/x/y/b", false},
		{"a/**/b", "a/x/y/b", true},
		{"a/**/b", "a/b", true},
	}
	for _, c := range cases {
		re, err := compileGlob(c.pattern)
		if err != nil {
			t.Fatalf("compileGlob(%q): %v", c.pattern, err)
		}
		if got := re.MatchString(c.path); got != c.want {
			t.Errorf("compileGlob(%q).MatchString(%q) = %v, want %v\nregex: %s", c.pattern, c.path, got, c.want, re.String())
		}
	}
}

func TestSplitGlobPatterns(t *testing.T) {
	got := splitGlobPatterns(" src/**, internal/**, **/*.go ")
	want := []string{"src/**", "internal/**", "**/*.go"}
	if len(got) != len(want) {
		t.Fatalf("len(got)=%d want %d (%v)", len(got), len(want), got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("got[%d]=%q want %q", i, got[i], want[i])
		}
	}
}
