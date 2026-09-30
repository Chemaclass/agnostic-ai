package spec

import (
	"regexp"
	"strings"
	"testing"
)

func protectEntry(path string, protected any) Entry {
	return Entry{Kind: KindSettings, Name: path, Path: path, Meta: map[string]any{"protected": protected}}
}

func TestProtectedPaths_ReadsEachSettingsSpecAsOneGroup(t *testing.T) {
	groups, err := ProtectedPaths([]Entry{
		protectEntry("settings/ci.yaml", map[string]any{
			"paths":    []any{".github/**", "./composer.lock", "/migrations/"},
			"decision": "deny",
			"reason":   "CI changes only on purpose.",
		}),
		{Kind: KindSettings, Path: "settings/model.yaml", Meta: map[string]any{"model": "m"}},
		protectEntry("settings/lock.yaml", map[string]any{"paths": []any{"go.sum"}}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	first := groups[0]
	if first.Source != "settings/ci.yaml" || first.Decision != ProtectDeny || first.Reason != "CI changes only on purpose." {
		t.Errorf("first group = %+v", first)
	}
	if got := strings.Join(first.Paths, ","); got != ".github/**,composer.lock,migrations/**" {
		t.Errorf("paths = %s", got)
	}
	if groups[1].Decision != ProtectAsk {
		t.Errorf("default decision = %q, want ask", groups[1].Decision)
	}
}

func TestProtectedPaths_RejectsShapesTheTargetsCannotAgreeOn(t *testing.T) {
	cases := map[string]any{
		"not a mapping":    []any{".github/**"},
		"no paths":         map[string]any{"decision": "ask"},
		"empty path":       map[string]any{"paths": []any{""}},
		"bad decision":     map[string]any{"paths": []any{"a"}, "decision": "allow"},
		"parent directory": map[string]any{"paths": []any{"../secrets"}},
		"home path":        map[string]any{"paths": []any{"~/x"}},
		"absolute path":    map[string]any{"paths": []any{"//etc/passwd"}},
		"negation":         map[string]any{"paths": []any{"!keep.txt"}},
		"character class":  map[string]any{"paths": []any{"file[0-9].txt"}},
		"brace":            map[string]any{"paths": []any{"*.{js,ts}"}},
		"backslash":        map[string]any{"paths": []any{`a\b`}},
		"parenthesis":      map[string]any{"paths": []any{"a(1).txt"}},
		"unknown key":      map[string]any{"paths": []any{"a"}, "mode": "deny"},
		"non-string path":  map[string]any{"paths": []any{3}},
		"non-string why":   map[string]any{"paths": []any{"a"}, "reason": 3},
	}
	for name, protected := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ProtectedPaths([]Entry{protectEntry("settings/p.yaml", protected)})
			if err == nil {
				t.Fatal("want an error")
			}
			if !strings.Contains(err.Error(), "settings/p.yaml") {
				t.Errorf("error %q does not name the spec", err)
			}
		})
	}
}

func TestProtectPatternRegexp_MatchesProjectRootAnchoredGlobs(t *testing.T) {
	cases := []struct {
		pattern string
		match   []string
		miss    []string
	}{
		{".github/**", []string{".github/workflows/ci.yml", ".github/CODEOWNERS"}, []string{".github", "sub/.github/x", ".githubx/a"}},
		{"composer.lock", []string{"composer.lock"}, []string{"sub/composer.lock", "composer.lock.bak"}},
		{"**/composer.lock", []string{"composer.lock", "a/b/composer.lock"}, []string{"composer.locks"}},
		{"src/*.go", []string{"src/main.go"}, []string{"src/a/main.go", "src/main.gox"}},
		{"docs/**/*.md", []string{"docs/a.md", "docs/x/y/a.md"}, []string{"docs/a.txt"}},
		{"file?.txt", []string{"file1.txt"}, []string{"file12.txt", "file/.txt"}},
		{"a+b.txt", []string{"a+b.txt"}, []string{"aab.txt"}},
		{"dir with space/*", []string{"dir with space/f"}, []string{"dir/f"}},
		{"**", []string{"a", "a/b"}, nil},
	}
	for _, tc := range cases {
		re := regexp.MustCompile(ProtectPatternRegexp(tc.pattern))
		for _, p := range tc.match {
			if !re.MatchString(p) {
				t.Errorf("%s should match %s (regexp %s)", tc.pattern, p, re)
			}
		}
		for _, p := range tc.miss {
			if re.MatchString(p) {
				t.Errorf("%s should not match %s (regexp %s)", tc.pattern, p, re)
			}
		}
	}
}

func TestProtectGroup_MatchCoversFilesInsideAProtectedDirectory(t *testing.T) {
	g := ProtectGroup{Paths: []string{"vendor", "go.sum"}}
	for _, p := range []string{"vendor", "vendor/a/b.go", "go.sum", "VENDOR/a.go", "Go.Sum"} {
		if _, ok := g.Match(p); !ok {
			t.Errorf("%s should be protected", p)
		}
	}
	for _, p := range []string{"vendors/a", "sub/go.sum"} {
		if _, ok := g.Match(p); ok {
			t.Errorf("%s should not be protected", p)
		}
	}
}
