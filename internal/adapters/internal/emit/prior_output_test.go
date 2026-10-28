package emit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPriorOutput_DoesNotAdoptSeparateLinks(t *testing.T) {
	for _, kind := range []string{"hard link", "symbolic link", "case-only hard link"} {
		t.Run(kind, func(t *testing.T) {
			dir := t.TempDir()
			old, current := filepath.Join(dir, "first", "rule.md"), filepath.Join(dir, "second", "rule.md")
			if kind == "case-only hard link" {
				old, current = filepath.Join(dir, "Rule.md"), filepath.Join(dir, "rule.md")
			}
			for _, path := range []string{old, current} {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "symbolic link" {
				target := filepath.Join(dir, "target.md")
				if err := os.WriteFile(target, []byte("hand edit\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				for _, path := range []string{old, current} {
					if err := os.Symlink(target, path); err != nil {
						t.Skipf("symbolic links unavailable: %v", err)
					}
				}
			} else {
				if err := os.WriteFile(old, []byte("hand edit\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if kind == "case-only hard link" {
					if _, err := os.Lstat(current); err == nil {
						t.Skip("the temp filesystem is case-insensitive")
					}
				}
				if err := os.Link(old, current); err != nil {
					t.Fatal(err)
				}
			}
			sums := map[string]string{old: ContentSum("generated\n")}
			sess := NewSession()
			sess.BackUpEditsSince(sums)
			sess.KeepEditsSince(sums)
			if err := sess.resolvePriorOutput(current); err != nil {
				t.Fatal(err)
			}
			if got := sess.priorChecksum(current, sums); got != "" {
				t.Errorf("separate entry adopted prior checksum: %q", got)
			}
			if len(sums) != 1 {
				t.Errorf("shared prior checksums changed: %v", sums)
			}
		})
	}
}
