package cli

import (
	"strings"
	"testing"

	"github.com/chemaclass/agnostic-ai/internal/testutil"
)

// The first sync shows what each tool now reads; later ones do not (#1614).
func TestSync_FirstRunShowsWhatEachToolReads(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude, codex]\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review a PR.\n---\nReview.\n")
	log := captureLog(t)

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	for _, want := range []string{"claude now reads", "codex now reads", ".agents/skills/", "review"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("first sync output is missing %q:\n%s", want, log.String())
		}
	}

	log.Reset()
	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("second sync: %v\n%s", err, out)
	}
	if strings.Contains(log.String(), "now reads") {
		t.Errorf("a later sync repeated the summary:\n%s", log.String())
	}
}

// use for a tool already in targets prints nothing of its own, so a first
// sync still lists what each tool reads.
func TestUse_AlreadyInUseStillShowsTheFirstSyncList(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	log := captureLog(t)

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}
	if !strings.Contains(log.String(), "codex now reads") {
		t.Errorf("first sync under use lost its list:\n%s", log.String())
	}
}

func TestSync_FirstRunListIsQuietUnderQuiet(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex]\n")
	log := captureLog(t)

	if out, err := runCLI(t, "sync", "--quiet"); err != nil {
		t.Fatalf("sync --quiet: %v\n%s", err, out)
	}
	if strings.Contains(log.String(), "now reads") {
		t.Errorf("--quiet printed the first-sync list:\n%s", log.String())
	}
}

// A first sync under use lists every target, not only the added one.
func TestUse_FirstSyncListsEveryTarget(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [claude]\n")
	log := captureLog(t)

	if out, err := runCLI(t, "use", "codex"); err != nil {
		t.Fatalf("use codex: %v\n%s", err, out)
	}
	for _, want := range []string{"claude now reads", "codex now reads"} {
		if n := strings.Count(log.String(), want); n != 1 {
			t.Errorf("%q printed %d times, want once:\n%s", want, n, log.String())
		}
	}
}

// A target sync could not load writes nothing, so the list leaves it out.
func TestSync_FirstRunListLeavesOutATargetThatDidNotEmit(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [codex, acme-tool]\n")
	log := captureLog(t)

	_, _ = runCLI(t, "sync")

	if !strings.Contains(log.String(), "codex now reads") || strings.Contains(log.String(), "acme-tool now reads") {
		t.Errorf("list should name codex and not acme-tool:\n%s", log.String())
	}
}

// A kind the target cannot load is not listed as read.
func TestSync_FirstRunListLeavesOutUnsupportedKinds(t *testing.T) {
	testutil.Chdir(t, t.TempDir())
	isolateGit(t)
	mustWriteFile(t, "agnostic-ai.yaml", "version: 1\ntargets: [jules]\n")
	mustWriteFile(t, ".agnostic-ai/rules/style.md", "---\nname: style\ndescription: Style.\n---\nKeep it short.\n")
	mustWriteFile(t, ".agnostic-ai/skills/review/SKILL.md", "---\nname: review\ndescription: Review a PR.\n---\nReview.\n")
	log := captureLog(t)

	if out, err := runCLI(t, "sync"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if !strings.Contains(log.String(), "jules now reads") || strings.Contains(log.String(), "1 skill") {
		t.Errorf("list should name jules without the unsupported skill:\n%s", log.String())
	}
}
