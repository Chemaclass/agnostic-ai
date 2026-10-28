package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/adapters"
	"github.com/chemaclass/agnostic-ai/internal/config"
	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// escapeFixture is a claude project in base/proj beside files in
// base/victim that a crafted ledger names.
type escapeFixture struct {
	base, proj, victim string
}

func newEscapeFixture(t *testing.T) escapeFixture {
	t.Helper()
	base := t.TempDir()
	f := escapeFixture{base: base, proj: filepath.Join(base, "proj"), victim: filepath.Join(base, "victim")}
	mustWriteFile(t, filepath.Join(f.proj, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\n")
	mustWriteFile(t, filepath.Join(f.proj, ".agnostic-ai", "rules", "r1.md"), "---\nname: r1\ndescription: Rule one.\n---\n\nBody.\n")
	testutil.Chdir(t, f.proj)
	return f
}

func writeCraftedLedger(t *testing.T, state syncStateFile) {
	t.Helper()
	state.Version = syncStateVersion
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, stateFilePath("."), string(data))
}

func captureLedgerWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	prev := ledgerWarnOut
	ledgerWarnOut = buf
	t.Cleanup(func() {
		ledgerWarnOut = prev
		warnedLedgerEntries.Clear()
	})
	return buf
}

func TestRunSyncOnce_LeavesFilesOutsideTheProjectThatTheLedgerNames(t *testing.T) {
	f := newEscapeFixture(t)
	const body = "generated\n"
	relVictim := filepath.Join("..", "victim", "sub", "CLAUDE.md")
	absVictim := filepath.Join(f.victim, "abs", "CLAUDE.md")
	mergedVictim := filepath.Join(f.victim, "x.json")
	mustWriteFile(t, filepath.Join(f.victim, "sub", "CLAUDE.md"), body)
	mustWriteFile(t, absVictim, body)
	mustWriteFile(t, mergedVictim, "{\"a\": 1}\n")
	relMerged := filepath.Join("..", "victim", "x.json")
	writeCraftedLedger(t, syncStateFile{
		Outputs:    []string{relVictim, absVictim, relMerged},
		OutputSums: map[string]string{relVictim: adapters.ContentSum(body), absVictim: adapters.ContentSum(body)},
		Merged: map[string]mergedOutput{relMerged: {
			Keys:    []adapters.MergedKey{{Path: []string{"a"}, Sum: adapters.ContentSum("1")}},
			Created: true,
		}},
	})
	silence(t)
	captureLog(t)
	warnings := captureLedgerWarnings(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	for _, p := range []string{filepath.Join(f.victim, "sub", "CLAUDE.md"), absVictim, mergedVictim} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("sync removed %s outside the project: %v", p, err)
		}
	}
	for _, entry := range []string{relVictim, absVictim, relMerged} {
		if !strings.Contains(warnings.String(), strconv.Quote(entry)) {
			t.Errorf("warning does not name %s:\n%s", entry, warnings.String())
		}
	}
	for _, p := range readStateFile(".").Outputs {
		if config.UnsafeProjectPath(p) {
			t.Errorf("rewritten ledger still lists %s", p)
		}
	}
}

func TestRunSyncOnce_LeavesALinkOutsideTheProjectThatTheLedgerNames(t *testing.T) {
	f := newEscapeFixture(t)
	mustWriteFile(t, filepath.Join(f.victim, "real", "data"), "keep\n")
	link := filepath.Join(f.victim, "skills", "foo")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.victim, "real"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeCraftedLedger(t, syncStateFile{Outputs: []string{filepath.Join("..", "victim", "skills", "foo")}})
	silence(t)
	captureLog(t)
	captureLedgerWarnings(t)

	if err := runSyncOnce(".", nil, false, false, "off", 1); err != nil {
		t.Fatal(err)
	}

	if fi, err := os.Lstat(link); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Errorf("sync changed the link outside the project: %v", err)
	}
}

func TestReadStateFile_DropsEntriesOutsideTheProject(t *testing.T) {
	root := testutil.TempCwd(t)
	abs := filepath.Join(t.TempDir(), "elsewhere.md")
	absInside := filepath.Join(root, "tools", "rules", "r1.md")
	escaping := filepath.Join("..", "elsewhere.md")
	gitHead := filepath.Join(".git", "HEAD")
	inside := filepath.Join("x", "..", "CLAUDE.md")
	writeCraftedLedger(t, syncStateFile{
		Outputs:    []string{escaping, abs, gitHead, inside, absInside},
		OutputSums: map[string]string{escaping: "s", abs: "s", gitHead: "s", inside: "s"},
		Orphans:    []string{escaping},
		Unledgered: []string{abs},
		Merged:     map[string]mergedOutput{escaping: {Created: true}},
		Backups:    map[string]string{abs + ".bak": "s"},
	})
	warnings := captureLedgerWarnings(t)

	state := readStateFile(".")

	if len(state.Outputs) != 2 || state.Outputs[0] != inside || state.Outputs[1] != absInside {
		t.Errorf("outputs = %v, want only %s and %s", state.Outputs, inside, absInside)
	}
	if len(state.OutputSums) != 1 || len(state.Orphans) != 0 || len(state.Unledgered) != 0 || len(state.Merged) != 0 || len(state.Backups) != 0 {
		t.Errorf("escaping entries kept: %+v", state)
	}
	if strict, err := readStateFileStrict("."); err != nil || len(strict.Outputs) != 2 {
		t.Errorf("strict read = %v, %v; want only %s", strict.Outputs, err, inside)
	}
	if n := strings.Count(warnings.String(), strconv.Quote(escaping)+"\n"); n != 1 {
		t.Errorf("want one warning naming %s, got %d:\n%s", escaping, n, warnings.String())
	}
}

func TestRunSyncOnce_RefusesAnAbsoluteOutputOutsideTheProject(t *testing.T) {
	f := newEscapeFixture(t)
	rules := filepath.Join(f.victim, "rules")
	mustWriteFile(t, filepath.Join(f.proj, "agnostic-ai.yaml"), "version: 1\ntargets: [claude]\noutputs:\n  claude:\n    rules-dir: "+strconv.Quote(filepath.ToSlash(rules))+"\n")
	silence(t)
	captureLog(t)

	err := runSyncOnce(".", nil, false, false, "off", 1)

	if err == nil || !strings.Contains(err.Error(), "outside the project") {
		t.Fatalf("sync error = %v, want a refusal to write outside the project", err)
	}
	if _, err := os.Stat(rules); !os.IsNotExist(err) {
		t.Errorf("sync wrote outside the project: %v", err)
	}
}
