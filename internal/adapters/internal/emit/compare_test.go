package emit

import (
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fullCompare is the status-quo oracle: an unconditional full read followed
// by a byte compare, exactly what the capture-compare path does today
// (internal/cli/check.go). CompareToDisk must return an identical verdict for
// every input; only its I/O may differ. It is also the "status-quo" subject
// in BenchmarkCompareToDisk.
func fullCompare(path, content string) (DiskState, error) {
	disk, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DiskMissing, nil
		}
		return DiskStale, fmt.Errorf("read %s: %w", path, err)
	}
	if string(disk) != content {
		return DiskStale, nil
	}
	return DiskInSync, nil
}

// The fast path must never diverge from a full read. For thousands of random
// (on-disk, candidate) pairs, deliberately weighted toward same-size but
// different content (the one case a naive size check would get wrong), the
// size-precheck verdict has to equal the full-compare verdict.
func TestCompareToDisk_MatchesFullCompareOracle(t *testing.T) {
	t.Parallel()
	r := rand.New(rand.NewPCG(0x9e3779b9, 0x7f4a7c15))
	dir := t.TempDir()
	const iters = 3000
	for i := 0; i < iters; i++ {
		disk := randBytes(r, 64)
		cand := mutate(r, disk)
		// One file in five is left absent so the missing verdict is exercised.
		present := r.IntN(5) != 0
		path := filepath.Join(dir, fmt.Sprintf("f-%05d", i))
		if present {
			if err := os.WriteFile(path, disk, filePerm); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
		}

		gotState, gotErr := CompareToDisk(path, string(cand))
		wantState, wantErr := fullCompare(path, string(cand))
		if (gotErr == nil) != (wantErr == nil) {
			t.Fatalf("iter %d: error mismatch: fast=%v full=%v", i, gotErr, wantErr)
		}
		if gotState != wantState {
			t.Fatalf("iter %d: verdict diverged: fast=%s full=%s (disk=%dB cand=%dB present=%v)",
				i, gotState, wantState, len(disk), len(cand), present)
		}
	}
}

// The marquee correctness case, stated flatly so intent is unmistakable:
// content that is the same length as the file on disk but different byte for
// byte must still be caught as stale. Size may never mask drift.
func TestCompareToDisk_SameSizeDifferentContentIsStale(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "same-size")
	if err := os.WriteFile(path, []byte("hello world"), filePerm); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	// "hello world" and "hello WORLD" are both 11 bytes: a size collision.
	got, err := CompareToDisk(path, "hello WORLD")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != DiskStale {
		t.Fatalf("size-equal different content must be %s, got %s", DiskStale, got)
	}
}

func TestCompareToDisk_Verdicts(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	tests := []struct {
		name    string
		disk    string // on-disk content; write skipped when absent is true
		absent  bool
		content string // candidate content
		want    DiskState
	}{
		{name: "in-sync", disk: "same", content: "same", want: DiskInSync},
		{name: "missing", absent: true, content: "anything", want: DiskMissing},
		{name: "stale-larger", disk: "short", content: "much longer body", want: DiskStale},
		{name: "stale-smaller", disk: "much longer body", content: "short", want: DiskStale},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name)
			if !tt.absent {
				if err := os.WriteFile(path, []byte(tt.disk), filePerm); err != nil {
					t.Fatalf("write fixture: %v", err)
				}
			}
			got, err := CompareToDisk(path, tt.content)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
		})
	}
}

// randBytes returns a random byte slice of length in [0, maxLen].
func randBytes(r *rand.Rand, maxLen int) []byte {
	b := make([]byte, r.IntN(maxLen+1))
	for i := range b {
		b[i] = byte(r.IntN(256))
	}
	return b
}

// mutate derives a candidate from the on-disk bytes, spread across the four
// cases the compare must distinguish: identical, same-length-but-different
// (the size-collision stressor), a different length, and fully random.
func mutate(r *rand.Rand, disk []byte) []byte {
	switch r.IntN(4) {
	case 0: // identical
		return append([]byte(nil), disk...)
	case 1: // same length, at least one byte guaranteed different
		c := append([]byte(nil), disk...)
		if len(c) == 0 {
			return c // empty has no byte to flip; stays identical
		}
		c[r.IntN(len(c))] ^= byte(1 + r.IntN(255))
		return c
	case 2: // different length
		return append(append([]byte(nil), disk...), byte(r.IntN(256)))
	default: // fully random content and length
		return randBytes(r, 64)
	}
}

// benchStateSink defeats dead-code elimination of the benchmarked verdict.
var benchStateSink DiskState

// BenchmarkCompareToDisk is the decision artifact required by
// bench-before-perf-refactor.md. It puts two subjects side by side over one
// on-disk file per scenario, reporting μs/call so the crossover can be
// computed:
//
//   - status-quo: fullCompare, an unconditional full read then byte compare.
//     For a pure I/O short-circuit the naive baseline and the status-quo
//     coincide (both always read the whole body), so two subjects capture it.
//   - fast-path:  CompareToDisk, which stats first and reads only when the
//     size matches.
//
// Scenarios model where the two diverge:
//
//   - insync:         candidate == disk. Sizes match, so the fast path reads
//     anyway and pays an extra stat (its worst case).
//   - drift-samesize: same length, different bytes. Fast path still reads
//     (and must still catch the drift), paying the extra stat.
//   - drift-diffsize: sizes differ. Fast path returns on the stat alone and
//     never reads the body. This is where the win lives, growing with size.
//
// Sizes sweep small to large because the saved read scales with the body.
func BenchmarkCompareToDisk(b *testing.B) {
	sizes := []int{200, 4 << 10, 64 << 10} // 200 B, 4 KiB, 64 KiB
	scenarios := []struct {
		name string
		cand func(disk string) string
	}{
		{"insync", func(disk string) string { return disk }},
		{"drift-samesize", func(disk string) string {
			return disk[:len(disk)-1] + "Z" // same length, last byte differs
		}},
		{"drift-diffsize", func(disk string) string { return disk + "extra" }},
	}
	subjects := []struct {
		name string
		cmp  func(path, content string) (DiskState, error)
	}{
		{"status-quo", fullCompare},
		{"fast-path", CompareToDisk},
	}
	for _, sz := range sizes {
		disk := strings.Repeat("a", sz)
		for _, sc := range scenarios {
			cand := sc.cand(disk)
			for _, sub := range subjects {
				b.Run(fmt.Sprintf("%s/size=%dB/%s", sc.name, sz, sub.name), func(b *testing.B) {
					dir := b.TempDir()
					path := filepath.Join(dir, "artifact")
					if err := os.WriteFile(path, []byte(disk), filePerm); err != nil {
						b.Fatalf("write fixture: %v", err)
					}
					b.ReportAllocs()
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						st, err := sub.cmp(path, cand)
						if err != nil {
							b.Fatalf("compare: %v", err)
						}
						benchStateSink = st
					}
				})
			}
		}
	}
}
