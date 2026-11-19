package cli

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestPathBelow(t *testing.T) {
	parent := t.TempDir()
	base := filepath.Join(parent, "project")
	cases := []struct {
		name, path, rel string
		inside          bool
	}{
		{"equal", base, ".", true},
		{"child", filepath.Join(base, "child"), "child", true},
		{"parent", parent, "..", false},
		{"sibling", filepath.Join(parent, "project-sibling"), filepath.Join("..", "project-sibling"), false},
		{"dot-dot prefix", filepath.Join(base, "..data"), "..data", true},
		{"relative path with absolute base", "child", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rel, inside := pathBelow(base, tc.path)
			if rel != tc.rel || inside != tc.inside {
				t.Errorf("pathBelow(%q, %q) = (%q, %v), want (%q, %v)", base, tc.path, rel, inside, tc.rel, tc.inside)
			}
		})
	}
	if runtime.GOOS == "windows" {
		if rel, inside := pathBelow("C:\\project", "D:\\project"); rel != "" || inside {
			t.Errorf("paths on different drives returned (%q, %v), want (\"\", false)", rel, inside)
		}
	}
}

func TestPathUnder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "hooks")
	outside := filepath.Join(t.TempDir(), "x.yaml")
	for _, tc := range []struct {
		name, input string
		inside      bool
	}{
		{"file name", "format.yaml", true},
		{"dot-dot prefix", "..format.yaml", true},
		{"nested", filepath.Join("a", "b.yaml"), true},
		{"parent", filepath.Join("..", "x.yaml"), false},
		{"climbs out", filepath.Join("a", "..", "..", "x.yaml"), false},
		{"absolute", outside, false},
		{"empty", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, inside := pathUnder(dir, tc.input)
			if inside != tc.inside {
				t.Fatalf("pathUnder(%q, %q) inside = %v, want %v", dir, tc.input, inside, tc.inside)
			}
			if inside && got != filepath.Join(dir, tc.input) {
				t.Errorf("pathUnder(%q, %q) = %q", dir, tc.input, got)
			}
		})
	}
}
