package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A spec file comes from the project or from an installed pack, so its
// frontmatter is untrusted input: any bytes must parse or fail, never
// panic, and parse the same way twice.
func FuzzSplitFrontmatter(f *testing.F) {
	for _, seed := range []string{
		"---\nname: r1\n---\nbody",
		"---\n---\n",
		"---\nname: [unclosed\n---\n",
		"no frontmatter",
		"---\na: &x [*x]\n---\n",
		"---\r\nname: r\r\n---\r\nbody",
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		meta, keys, _, body, err := splitFrontmatter(data)
		meta2, keys2, _, body2, err2 := splitFrontmatter(data)
		if (err == nil) != (err2 == nil) || body != body2 || len(meta) != len(meta2) || strings.Join(keys, ",") != strings.Join(keys2, ",") {
			t.Fatalf("splitFrontmatter is not deterministic for %q", data)
		}
	})
}

// A review's `@path` lines read files from the project root. Whatever a
// spec holds, an include must never read a file outside that root.
func FuzzResolveIncludesStaysInsideRoot(f *testing.F) {
	for _, seed := range []string{
		"@docs/a.md",
		"@../secret.txt",
		"@/etc/passwd",
		"@docs/../../secret.txt",
		"```\n@../secret.txt\n```",
		"  @./docs/a.md  ",
		"@docs\\..\\..\\secret.txt",
	} {
		f.Add(seed)
	}
	parent := f.TempDir()
	root := filepath.Join(parent, "project")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		f.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "docs", "a.md"), []byte("inside"), 0o644); err != nil {
		f.Fatal(err)
	}
	const secret = "OUTSIDE-THE-ROOT"
	if err := os.WriteFile(filepath.Join(parent, "secret.txt"), []byte(secret), 0o644); err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, body string) {
		if strings.Contains(body, secret) {
			return
		}
		out, err := resolveIncludes(body, root)
		if err == nil && strings.Contains(out, secret) {
			t.Fatalf("include read a file outside the project root: %q", body)
		}
	})
}
