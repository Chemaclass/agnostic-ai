package emit

import (
	"os"
	"path/filepath"
	"testing"
)

// BenchmarkRewriteHookRoot measures one hook command rewrite from a
// folder three levels below the Git root.
//
//	status-quo: the root is resolved before the command is read
//	proposed:   RewriteHookRoot, which resolves the root only when the command needs it
//	baseline:   the root resolved once, outside the loop
func BenchmarkRewriteHookRoot(b *testing.B) {
	repo := b.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		b.Fatal(err)
	}
	project := filepath.Join(repo, "a", "b", "c")
	if err := os.MkdirAll(project, 0o755); err != nil {
		b.Fatal(err)
	}
	b.Chdir(project)
	cases := []struct{ name, command, target string }{
		{"no-reference", "go test ./...", "codex"},
		{"native-variable", "\"$CLAUDE_PROJECT_DIR\"/scripts/check.sh", "gemini"},
		{"git-root", "\"$CLAUDE_PROJECT_DIR\"/scripts/check.sh", "codex"},
	}
	root := projectHookRoot()
	for _, c := range cases {
		want := RewriteHookProjectRoot(c.command, c.target, projectHookRoot(), nil)
		if got := RewriteHookRoot(c.command, c.target); got != want {
			b.Fatalf("%s: proposed %q, status-quo %q", c.name, got, want)
		}
		b.Run(c.name+"/status-quo", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				RewriteHookProjectRoot(c.command, c.target, projectHookRoot(), nil)
			}
		})
		b.Run(c.name+"/proposed", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				RewriteHookRoot(c.command, c.target)
			}
		})
		b.Run(c.name+"/baseline", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				RewriteHookProjectRoot(c.command, c.target, root, nil)
			}
		})
	}
}
