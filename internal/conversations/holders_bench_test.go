package conversations

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// benchRecentLocks is a week of threads: the ones the 7-day cutoff kept.
const benchRecentLocks = 20

// BenchmarkHolders compares the ways a scan asks which thread locks a
// process holds, for a folder of old locks plus a week of recent ones,
// with one old lock held open by the benchmark itself:
//
//   - cutoff: the locks written in the last week, named to lsof. It
//     misses the old held lock, which is why it was dropped.
//   - per-file: every lock named to lsof, 128 at a time.
//   - under-folder: one lsof run over the folder, then the check that
//     lsof could see every lock: one lstat per lock, one open per folder.
//
// Run it with: go test -run '^$' -bench Holders -benchtime 5x ./internal/conversations
func BenchmarkHolders(b *testing.B) {
	if runtime.GOOS == "windows" {
		b.Skip("lsof is not read on Windows")
	}
	if _, err := exec.LookPath("lsof"); err != nil {
		b.Skip("lsof is not installed")
	}
	for _, old := range []int{100, 1000, 5000} {
		dir := b.TempDir()
		var all, recent []string
		for n := range old + benchRecentLocks {
			path := filepath.Join(dir, fmt.Sprintf("%08d.lock", n))
			if err := os.WriteFile(path, nil, 0o644); err != nil {
				b.Fatal(err)
			}
			all = append(all, path)
			if n >= old {
				recent = append(recent, path)
			}
		}
		held, err := os.Open(all[0])
		if err != nil {
			b.Fatal(err)
		}
		b.Cleanup(func() { held.Close() })
		subjects := []struct {
			name     string
			read     func() (map[string]int, error)
			findsOld bool
		}{
			{"cutoff", func() (map[string]int, error) { return SystemProcesses{}.Holders(recent) }, false},
			{"per-file", func() (map[string]int, error) { return SystemProcesses{}.Holders(all) }, true},
			{"under-folder", func() (map[string]int, error) {
				held, err := SystemProcesses{}.HoldersUnder(dir)
				if unseen := outOfSight(dir, all, held); len(unseen) > 0 {
					b.Fatalf("every lock is in sight: %v", unseen)
				}
				return held, err
			}, true},
		}
		for _, subject := range subjects {
			b.Run(fmt.Sprintf("%s/%d-old", subject.name, old), func(b *testing.B) {
				for b.Loop() {
					found, err := subject.read()
					if err != nil {
						b.Fatal(err)
					}
					if _, ok := found[all[0]]; ok != subject.findsOld {
						b.Fatalf("%s finds the old held lock: %v, want %v", subject.name, ok, subject.findsOld)
					}
				}
			})
		}
	}
}
