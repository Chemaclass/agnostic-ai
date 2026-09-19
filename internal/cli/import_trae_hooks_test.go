package cli

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// TestImportTraeHooks_RoundTripFixedPoint emits hook specs to
// `.trae/hooks.json`, wipes the source specs, imports the emitted file
// back, then re-emits. The second emit must byte-match the first.
//
// The bundle covers the three shapes the file can hold: a single
// command with a timeout, two commands sharing one matcher (which must
// collapse into one spec whose `command:` is a list, or the re-emit
// would split the group), and a `Stop` group carrying `loop_limit`.
func TestImportTraeHooks_RoundTripFixedPoint(t *testing.T) {
	dir := t.TempDir()
	testutil.Chdir(t, dir)
	silence(t)

	writeFile(t, filepath.Join(dir, "agnostic-ai.yaml"), "version: 1\ntargets: [trae]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "edited.yaml"),
		"name: edited\nevent: PostToolUse\nmatcher: Edit\ncommand: echo edited\ntimeout: 12\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "pair.yaml"),
		"name: pair\nevent: PreToolUse\nmatcher: RunCommand\ncommand: [\"echo a\", \"echo b\"]\n")
	writeFile(t, filepath.Join(dir, ".agnostic-ai", "hooks", "guard.yaml"),
		"name: guard\nevent: Stop\ncommand: python3 check_tests.py\nloop_limit: 3\n")

	execCLI(t, "sync", "-t", "trae")
	first := snapshotEmitted(t, dir)
	emitted, ok := first[".trae/hooks.json"]
	if !ok {
		t.Fatalf("first emit produced no hook file: %v", keys(first))
	}
	for _, want := range []string{`"loop_limit": 3`, `"timeout": 12`, `"echo a"`, `"echo b"`} {
		if !strings.Contains(emitted, want) {
			t.Fatalf("first emit missing %q:\n%s", want, emitted)
		}
	}

	if err := os.RemoveAll(filepath.Join(dir, ".agnostic-ai")); err != nil {
		t.Fatalf("wipe source specs: %v", err)
	}
	execCLI(t, "import", "trae")

	specs := readHookSpecs(t, filepath.Join(dir, ".agnostic-ai", "hooks"))
	if len(specs) != 3 {
		t.Fatalf("expected 3 hook specs, got %d: %v", len(specs), keys(specs))
	}
	joined := strings.Join(sortedValues(specs), "\n")
	for _, want := range []string{
		"event: PostToolUse", "matcher: Edit", "timeout: 12",
		"event: PreToolUse", "- echo a", "- echo b",
		"event: Stop", "loop_limit: 3", "target: trae",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("imported hook specs missing %q:\n%s", want, joined)
		}
	}

	execCLI(t, "sync", "-t", "trae")
	assertEmittedEqual(t, first, snapshotEmitted(t, dir))
}

// TestImportTraeHooks_AbsentFileImportsNothing guards the no-op path: a
// Trae project with no hook file must still import cleanly.
func TestImportTraeHooks_AbsentFileImportsNothing(t *testing.T) {
	dir := t.TempDir()
	n, err := importTraeHooks(dir, filepath.Join(dir, "hooks"))
	if err != nil {
		t.Fatalf("importTraeHooks: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 hooks from an absent file, got %d", n)
	}
}

// TestImportTraeHooks_ZeroLoopLimitStaysOff proves a `loop_limit` the
// vendor treats as unset does not reach the spec. Trae falls back to its
// default of 5 for an absent key and for a value "less than or equal to
// 0" alike, so writing `loop_limit: 0` would emit a key the next sync
// then drops, and the round-trip would never reach a fixed point.
func TestImportTraeHooks_ZeroLoopLimitStaysOff(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".trae", "hooks.json"),
		`{"version":1,"hooks":{"Stop":[{"matcher":"","loop_limit":0,"hooks":[{"type":"command","command":"echo done"}]}]}}`)

	dst := filepath.Join(dir, "hooks")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	n, err := importTraeHooks(dir, dst)
	if err != nil {
		t.Fatalf("importTraeHooks: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 hook, got %d", n)
	}
	for _, body := range readHookSpecs(t, dst) {
		if strings.Contains(body, "loop_limit") {
			t.Errorf("zero loop_limit reached the spec:\n%s", body)
		}
	}
}

// readHookSpecs returns every `*.yaml` under dir keyed by filename.
func readHookSpecs(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}
	out := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		out[e.Name()] = readFile(t, filepath.Join(dir, e.Name()))
	}
	return out
}

// sortedValues returns a map's values ordered by key, so assertions read
// the same on every run.
func sortedValues(m map[string]string) []string {
	names := keys(m)
	sort.Strings(names)
	out := make([]string, 0, len(names))
	for _, n := range names {
		out = append(out, m[n])
	}
	return out
}
